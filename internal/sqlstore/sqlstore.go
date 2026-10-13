// Package sqlstore persists the shared memory domain through database/sql
// for the embedded-SQL editions: Mini (MariaDB) and UltraLite (SQLite). The
// queries are written once in SQL both engines accept; a Dialect supplies the
// few differences. PostgreSQL keeps its own implementation in package store.
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
)

// Dialect captures what differs between the engines.
type Dialect struct {
	// ForUpdate is appended to locking reads (" FOR UPDATE"); SQLite's
	// writers already hold the database write lock and leave it empty.
	ForUpdate string
	// NameCollation orders project names case-insensitively.
	NameCollation string
	// ProjectByRepository selects a project row by canonical repository.
	ProjectByRepository string
	// Time converts a timestamp into the parameter the engine stores.
	Time func(time.Time) any
	// Wrap turns a driver error into memory.DatabaseError where applicable.
	Wrap func(error) error
	// Duplicate reports a unique or primary key violation.
	Duplicate func(error) bool
	// WriteTx and SnapshotTx are the options for mutations and exports.
	WriteTx, SnapshotTx *sql.TxOptions
}

type Store struct {
	DB *sql.DB
	D  Dialect
}

func (s *Store) Ping(ctx context.Context) error { return s.D.Wrap(s.DB.PingContext(ctx)) }

func (s *Store) now() any { return s.D.Time(time.Now()) }

func (s *Store) Register(ctx context.Context, p memory.Principal) error {
	_, e := s.DB.ExecContext(ctx, "INSERT INTO principals(id,created_at) VALUES(?,?)", p.ID, s.now())
	return s.ignoreDuplicate(e)
}

// ignoreDuplicate accepts an existing key. Runtime logins cannot UPDATE
// principals or agents, so "upsert" statements are not used; engines roll back
// only the failed statement, never the surrounding transaction.
func (s *Store) ignoreDuplicate(e error) error {
	if e != nil && s.D.Duplicate(e) {
		return nil
	}
	return s.D.Wrap(e)
}

// Column naming conventions shared by every query: these names decode as
// booleans or nested JSON; names ending in _at are timestamps.
var boolColumns = map[string]bool{"owner_attested": true, "superseded": true, "stale": true}
var jsonColumns = map[string]bool{"counts": true}
var numericTypes = map[string]bool{"TINYINT": true, "SMALLINT": true, "MEDIUMINT": true, "INT": true, "BIGINT": true, "DECIMAL": true, "DOUBLE": true, "FLOAT": true, "INTEGER": true}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// rows scans a result into JSON-shaped maps identical to the PostgreSQL
// backend: numbers are float64 and timestamps are PostgreSQL-formatted.
func (s *Store) rows(ctx context.Context, q queryer, query string, args ...any) ([]memory.Row, error) {
	r, e := q.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, s.D.Wrap(e)
	}
	defer r.Close()
	types, e := r.ColumnTypes()
	if e != nil {
		return nil, e
	}
	cols := make([]string, len(types))
	for i, t := range types {
		cols[i] = t.Name()
	}
	out := []memory.Row{}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for r.Next() {
		if e = r.Scan(ptrs...); e != nil {
			return nil, s.D.Wrap(e)
		}
		m := make(memory.Row, len(cols))
		for i, c := range cols {
			m[c] = convert(c, types[i].DatabaseTypeName(), vals[i])
		}
		out = append(out, m)
	}
	return out, s.D.Wrap(r.Err())
}

// StoredTime is the fixed-width UTC text form used where an engine stores
// timestamps as text; equal width keeps text order chronological.
const StoredTime = "2006-01-02T15:04:05.000000Z"

func convert(col, dbType string, v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		if numericTypes[strings.ToUpper(dbType)] {
			var f float64
			fmt.Sscan(string(x), &f)
			if boolColumns[col] {
				return f != 0
			}
			return f
		}
		return convert(col, dbType, string(x))
	case string:
		if jsonColumns[col] {
			var j any
			if json.Unmarshal([]byte(x), &j) == nil {
				return j
			}
		}
		if strings.HasSuffix(col, "_at") {
			if t, e := time.Parse(StoredTime, x); e == nil {
				return memory.Timestamp(t)
			}
		}
		return x
	case time.Time:
		return memory.Timestamp(x)
	case int64:
		if boolColumns[col] {
			return x != 0
		}
		return float64(x)
	case float32:
		return float64(x)
	}
	return v
}

func (s *Store) one(ctx context.Context, q queryer, query string, args ...any) (memory.Row, error) {
	r, e := s.rows(ctx, q, query, args...)
	if e != nil {
		return nil, e
	}
	if len(r) != 1 {
		return nil, memory.ErrConflict
	}
	return r[0], nil
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// in returns "?,?,..." for n values; callers guarantee n > 0.
func in(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func anyArgs(v []string) []any {
	out := make([]any, len(v))
	for i, s := range v {
		out[i] = s
	}
	return out
}

// prefixed qualifies a column list with a table alias.
func prefixed(alias, cols string) string {
	return alias + "." + strings.ReplaceAll(cols, ",", ","+alias+".")
}
