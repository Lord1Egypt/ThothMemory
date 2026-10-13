// Package sqlite is the SQLite backend of ThothMemory UltraLite: one local
// database file, no server, pure Go (no CGO). Queries are shared with Mini
// through package sqlstore.
package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/portable"
	"github.com/Lord1Egypt/ThothMemory/internal/sqlstore"
	sqlite3 "modernc.org/sqlite"
	lib "modernc.org/sqlite/lib"
)

//go:embed schema/*.sql
var schema embed.FS

type Store struct {
	*sqlstore.Store
	Path string
}

var dialect = sqlstore.Dialect{
	NameCollation:       "name COLLATE NOCASE",
	ProjectByRepository: "repository=?",
	Time:                func(t time.Time) any { return t.UTC().Format(sqlstore.StoredTime) },
	Wrap:                wrap,
	Duplicate: func(e error) bool {
		var se *sqlite3.Error
		return errors.As(e, &se) && (se.Code() == lib.SQLITE_CONSTRAINT_UNIQUE || se.Code() == lib.SQLITE_CONSTRAINT_PRIMARYKEY)
	},
	// Every transaction begins IMMEDIATE (see Open), taking the single write
	// lock up front: a writer never deadlocks upgrading a read lock.
	WriteTx:    nil,
	SnapshotTx: nil,
}

// wrap classifies SQLite errors: busy/locked are transient and retryable.
func wrap(e error) error {
	var se *sqlite3.Error
	if !errors.As(e, &se) {
		return e
	}
	code := se.Code()
	state := "HY000"
	switch code & 0xff {
	case lib.SQLITE_CONSTRAINT:
		state = "23000"
	case lib.SQLITE_BUSY, lib.SQLITE_LOCKED:
		return &memory.DatabaseError{State: "40001", Code: code, Retryable: true, Err: e}
	}
	return &memory.DatabaseError{State: state, Code: code, Err: e}
}

// Open opens (creating if needed) the database file. Every pooled connection
// enforces foreign keys, uses WAL so readers never block the writer, and
// fsyncs each commit (synchronous=FULL). SQLite waits at most 250 ms for the
// write lock per attempt; retry() repeats busy attempts within the caller's
// deadline, because SQLite's own busy wait cannot observe a cancelled context.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.ContainsAny(path, "?#") {
		return nil, errors.New("database path must not contain ? or #")
	}
	abs, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	dsn := abs + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(250)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=trusted_schema(0)&_txlock=immediate"
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		return nil, e
	}
	// Few connections: WAL serves concurrent readers; writers take turns.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(2 * time.Minute)
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, wrap(e)
	}
	return &Store{&sqlstore.Store{DB: db, D: dialect}, abs}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// maxWait bounds how long one operation waits for another writer: 10 s, or
// THOTH_SQLITE_BUSY_MS (milliseconds, 50..60000) for tests and slow disks.
var maxWait = func() time.Duration {
	if v, e := strconv.Atoi(os.Getenv("THOTH_SQLITE_BUSY_MS")); e == nil && v >= 50 && v <= 60000 {
		return time.Duration(v) * time.Millisecond
	}
	return 10 * time.Second
}()

// retry repeats fn while the database is busy, backing off from 5 ms to
// 100 ms, until it succeeds, fails otherwise, ctx ends or maxWait passes.
func retry(ctx context.Context, fn func() error) error {
	deadline := time.Now().Add(maxWait)
	delay := 5 * time.Millisecond
	for {
		e := fn()
		var de *memory.DatabaseError
		if e == nil || !errors.As(e, &de) || !de.Retryable || time.Now().After(deadline) {
			return e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 100*time.Millisecond {
			delay *= 2
		}
	}
}

func (s *Store) Begin(ctx context.Context) (memory.Tx, error) {
	var t memory.Tx
	e := retry(ctx, func() (err error) { t, err = s.Store.Begin(ctx); return })
	return t, e
}

func (s *Store) ImportWriter(ctx context.Context) (portable.Writer, error) {
	var w portable.Writer
	e := retry(ctx, func() (err error) { w, err = s.Store.ImportWriter(ctx); return })
	return w, e
}

func (s *Store) Register(ctx context.Context, p memory.Principal) error {
	return retry(ctx, func() error { return s.Store.Register(ctx, p) })
}

func (s *Store) CreateProject(ctx context.Context, name, repo string) (memory.Row, error) {
	var r memory.Row
	e := retry(ctx, func() (err error) { r, err = s.Store.CreateProject(ctx, name, repo); return })
	return r, e
}

func (s *Store) Info() memory.Info {
	return memory.Info{Product: "ThothMemory UltraLite", Edition: "ultralite", Version: memory.UltraLiteVersion, Database: "sqlite", Capabilities: map[string]bool{"english_stemming": false, "full_text_index": false, "substring_search": true, "dashboard_explorer": true, "export_import": true}}
}

func (s *Store) Call(ctx context.Context, p memory.Principal, tool string, a memory.Input) (any, error) {
	return memory.Call(ctx, s, p, tool, a)
}

// Migrate applies embedded schema files atomically: SQLite DDL is
// transactional, so an interrupted migration leaves no partial schema.
func (s *Store) Migrate(ctx context.Context) error {
	return retry(ctx, func() error { return s.migrate(ctx) })
}

// current reports, without taking the write lock, whether every embedded
// migration is already applied with a matching checksum. A mismatch is not
// "current": the transactional path below rejects it.
func (s *Store) current(ctx context.Context) bool {
	files, e := schema.ReadDir("schema")
	if e != nil {
		return false
	}
	for _, f := range files {
		b, _ := schema.ReadFile("schema/" + f.Name())
		h := sha256.Sum256(b)
		var sum string
		if s.DB.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=?", f.Name()).Scan(&sum) != nil || sum != hex.EncodeToString(h[:]) {
			return false
		}
	}
	return true
}

func (s *Store) migrate(ctx context.Context) error {
	if s.current(ctx) {
		return nil // opening a healthy database must not need the write lock
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return wrap(e)
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version TEXT NOT NULL PRIMARY KEY, checksum TEXT NOT NULL, applied_at TEXT NOT NULL) STRICT"); e != nil {
		return wrap(e)
	}
	files, e := schema.ReadDir("schema")
	if e != nil {
		return e
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	for _, f := range files {
		b, _ := schema.ReadFile("schema/" + f.Name())
		h := sha256.Sum256(b)
		sum := hex.EncodeToString(h[:])
		var old string
		e = tx.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=?", f.Name()).Scan(&old)
		switch {
		case e == nil && old != sum:
			return errors.New("migration checksum mismatch")
		case e == nil:
			continue
		case !errors.Is(e, sql.ErrNoRows):
			return wrap(e)
		}
		if _, e = tx.ExecContext(ctx, string(b)); e != nil {
			return fmt.Errorf("migration %s: %w", f.Name(), wrap(e))
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum,applied_at) VALUES(?,?,?)", f.Name(), sum, dialect.Time(time.Now())); e != nil {
			return wrap(e)
		}
	}
	return wrap(tx.Commit())
}

func (s *Store) SchemaVersion(ctx context.Context) (string, error) {
	var v sql.NullString
	e := s.DB.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&v)
	return v.String, wrap(e)
}

// Check runs SQLite's integrity and foreign-key checks.
func Check(ctx context.Context, db *sql.DB) error {
	var result string
	if e := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); e != nil {
		return wrap(e)
	}
	if result != "ok" {
		return errors.New("integrity check failed: " + result)
	}
	r, e := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if e != nil {
		return wrap(e)
	}
	defer r.Close()
	if r.Next() {
		return errors.New("foreign key check failed")
	}
	return wrap(r.Err())
}

// Backup writes a consistent snapshot with VACUUM INTO, which reads one
// transaction of the live database (WAL included) while writers continue.
// The copy is integrity-checked and gets a SHA-256 sidecar. It never
// overwrites an existing file.
func (s *Store) Backup(ctx context.Context, target string) (string, error) {
	abs, e := filepath.Abs(target)
	if e != nil {
		return "", e
	}
	if _, e = os.Stat(abs); !os.IsNotExist(e) {
		return "", errors.New("backup target exists or cannot be inspected")
	}
	if e = os.MkdirAll(filepath.Dir(abs), 0700); e != nil {
		return "", e
	}
	if _, e = s.DB.ExecContext(ctx, "VACUUM INTO ?", abs); e != nil {
		return "", wrap(e)
	}
	ok := false
	defer func() {
		if !ok {
			os.Remove(abs)
		}
	}()
	if e = os.Chmod(abs, 0600); e != nil {
		return "", e
	}
	if e = Verify(ctx, abs); e != nil {
		return "", e
	}
	sum, e := fileSum(abs)
	if e != nil {
		return "", e
	}
	ok = true
	return sum, nil
}

// Verify opens a database file read-only and checks integrity and schema.
func Verify(ctx context.Context, path string) error {
	if strings.ContainsAny(path, "?#") {
		return errors.New("database path must not contain ? or #")
	}
	db, e := sql.Open("sqlite", path+"?_pragma=query_only(1)&_pragma=foreign_keys(1)")
	if e != nil {
		return e
	}
	defer db.Close()
	if e = Check(ctx, db); e != nil {
		return e
	}
	var n int
	if e = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); e != nil || n == 0 {
		return errors.New("not a ThothMemory UltraLite database")
	}
	return nil
}

func fileSum(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FileSum returns the SHA-256 of a file.
func FileSum(path string) (string, error) { return fileSum(path) }

// Status reports engine, schema, file sizes and row counts.
func (s *Store) Status(ctx context.Context) (memory.Row, error) {
	out := memory.Row{"path": s.Path}
	var version, mode string
	var pages, size, free int64
	for _, q := range []struct {
		sql string
		dst any
	}{{"SELECT sqlite_version()", &version}, {"PRAGMA journal_mode", &mode}, {"PRAGMA page_count", &pages}, {"PRAGMA page_size", &size}, {"PRAGMA freelist_count", &free}} {
		if e := s.DB.QueryRowContext(ctx, q.sql).Scan(q.dst); e != nil {
			return nil, wrap(e)
		}
	}
	schemaVersion, e := s.SchemaVersion(ctx)
	if e != nil {
		return nil, e
	}
	counts, e := s.Counts(ctx)
	if e != nil {
		return nil, e
	}
	out["engine"], out["journal_mode"], out["schema"], out["counts"] = "SQLite "+version, mode, schemaVersion, counts
	out["database_bytes"], out["free_bytes"] = float64(pages*size), float64(free*size)
	if st, e := os.Stat(s.Path + "-wal"); e == nil {
		out["wal_bytes"] = float64(st.Size())
	}
	return out, nil
}
