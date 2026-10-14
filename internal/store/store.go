// Package store is the PostgreSQL backend of ThothMemory Full.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*Store, error) {
	c, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return nil, e
	}
	c.MaxConns = 8
	c.MinConns = 0
	c.MaxConnIdleTime = 5 * time.Minute
	c.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	p, e := pgxpool.NewWithConfig(ctx, c)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	return &Store{p}, nil
}

func (s *Store) Info() memory.Info {
	return memory.Info{Product: "ThothMemory", Edition: "full", Version: memory.FullVersion, Database: "postgresql", Capabilities: map[string]bool{"english_stemming": true, "full_text_index": true, "substring_search": true, "dashboard_explorer": true, "export_import": true}}
}

func (s *Store) Ping(ctx context.Context) error { return s.Pool.Ping(ctx) }

// Call executes one tool through the shared domain service.
func (s *Store) Call(ctx context.Context, p memory.Principal, tool string, a memory.Input) (any, error) {
	return memory.Call(ctx, s, p, tool, a)
}

func (s *Store) Migrate(ctx context.Context) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(74298133)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	fs, e := migrations.Files.ReadDir(".")
	if e != nil {
		return e
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].Name() < fs[j].Name() })
	for _, f := range fs {
		if !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		b, _ := migrations.Files.ReadFile(f.Name())
		h := sha256.Sum256(b)
		sum := hex.EncodeToString(h[:])
		var old string
		e = tx.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE version=$1", f.Name()).Scan(&old)
		if e == nil {
			if old != sum {
				return errors.New("migration checksum mismatch")
			}
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, string(b)); e != nil {
			return fmt.Errorf("migration %s: %w", f.Name(), e)
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)", f.Name(), sum); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

// SchemaVersion returns the newest applied migration.
func (s *Store) SchemaVersion(ctx context.Context) (string, error) {
	var v string
	e := s.Pool.QueryRow(ctx, "SELECT COALESCE(max(version),'') FROM schema_migrations").Scan(&v)
	return v, e
}

type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func Rows(ctx context.Context, q Queryer, sql string, args ...any) ([]map[string]any, error) {
	r, e := q.Query(ctx, sql, args...)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	out := []map[string]any{}
	for r.Next() {
		var b []byte
		if e = r.Scan(&b); e != nil {
			return nil, e
		}
		var m map[string]any
		if e = json.Unmarshal(b, &m); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, r.Err()
}

func (s *Store) Register(ctx context.Context, p memory.Principal) error {
	_, e := s.Pool.Exec(ctx, "INSERT INTO principals(id) VALUES($1) ON CONFLICT DO NOTHING", p.ID)
	return e
}

// Status reports engine, schema, database size and row counts.
func (s *Store) Status(ctx context.Context) (memory.Row, error) {
	r, e := Rows(ctx, s.Pool, `SELECT jsonb_build_object('engine','PostgreSQL '||current_setting('server_version'),'database_bytes',pg_database_size(current_database()),
 'schema',(SELECT max(version) FROM schema_migrations),
 'counts',jsonb_build_object('projects',(SELECT count(*) FROM projects),'tasks',(SELECT count(*) FROM tasks),'sessions',(SELECT count(*) FROM sessions),'events',(SELECT count(*) FROM events),'records',(SELECT count(*) FROM records),'evidence',(SELECT count(*) FROM evidence),'attestations',(SELECT count(*) FROM attestations),'imports',(SELECT count(*) FROM imports)))`)
	if e != nil {
		return nil, e
	}
	return r[0], nil
}
