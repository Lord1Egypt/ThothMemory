package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/portable"
	"github.com/jackc/pgx/v5"
)

type reader struct{ pgx.Tx }

// ExportReader reads every table from one REPEATABLE READ snapshot.
func (s *Store) ExportReader(ctx context.Context) (portable.Reader, error) {
	t, e := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return nil, e
	}
	return reader{t}, nil
}

func scope(t portable.Table) string {
	if t.Name == "projects" {
		return "id=$1"
	}
	return "project_id=$1"
}

func (r reader) Select(ctx context.Context, t portable.Table, project string) ([]memory.Row, error) {
	return Rows(ctx, r.Tx, "SELECT to_jsonb(x) FROM "+pgx.Identifier{t.Name}.Sanitize()+" x WHERE "+scope(t), project)
}
func (r reader) Close(ctx context.Context) error { return r.Rollback(ctx) }

type writer struct{ pgx.Tx }

func (s *Store) ImportWriter(ctx context.Context) (portable.Writer, error) {
	t, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	// Serialize imports with each other; mutations lock the project row.
	if _, e = t.Exec(ctx, "SELECT pg_advisory_xact_lock(74298134)"); e != nil {
		t.Rollback(ctx)
		return nil, e
	}
	return writer{t}, nil
}

func where(t portable.Table, start int) string {
	parts := make([]string, len(t.Key))
	for i, k := range t.Key {
		parts[i] = fmt.Sprintf("%s=$%d", pgx.Identifier{k}.Sanitize(), start+i)
	}
	return strings.Join(parts, " AND ")
}

func pgValues(vals []any) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		if b, ok := v.([]byte); ok {
			out[i] = json.RawMessage(b)
		} else {
			out[i] = v
		}
	}
	return out
}

func (w writer) Get(ctx context.Context, t portable.Table, key []any) (memory.Row, bool, error) {
	r, e := Rows(ctx, w.Tx, "SELECT to_jsonb(x) FROM "+pgx.Identifier{t.Name}.Sanitize()+" x WHERE "+where(t, 1), key...)
	if e != nil || len(r) == 0 {
		return nil, false, e
	}
	return r[0], true, nil
}

func (w writer) Insert(ctx context.Context, t portable.Table, vals []any) error {
	cols, marks := make([]string, len(t.Columns)), make([]string, len(t.Columns))
	for i, c := range t.Columns {
		cols[i], marks[i] = pgx.Identifier{c}.Sanitize(), fmt.Sprintf("$%d", i+1)
	}
	_, e := w.Exec(ctx, "INSERT INTO "+pgx.Identifier{t.Name}.Sanitize()+"("+strings.Join(cols, ",")+") VALUES("+strings.Join(marks, ",")+")", pgValues(vals)...)
	return e
}

func (w writer) Update(ctx context.Context, t portable.Table, vals []any) error {
	if !t.Mutable {
		return errors.New("history is append-only")
	}
	sets := make([]string, len(t.Columns))
	key := []any{}
	for i, c := range t.Columns {
		sets[i] = fmt.Sprintf("%s=$%d", pgx.Identifier{c}.Sanitize(), i+1)
	}
	for _, k := range t.Key {
		for i, c := range t.Columns {
			if c == k {
				key = append(key, vals[i])
			}
		}
	}
	_, e := w.Exec(ctx, "UPDATE "+pgx.Identifier{t.Name}.Sanitize()+" SET "+strings.Join(sets, ",")+" WHERE "+where(t, len(t.Columns)+1), append(pgValues(vals), key...)...)
	return e
}

func (w writer) EnsurePrincipal(ctx context.Context, id string) error {
	_, e := w.Exec(ctx, "INSERT INTO principals(id) VALUES($1) ON CONFLICT DO NOTHING", id)
	return e
}
func (w writer) EnsureAgent(ctx context.Context, id string) error {
	_, e := w.Exec(ctx, "INSERT INTO agents(id) VALUES($1) ON CONFLICT DO NOTHING", id)
	return e
}
func (w writer) Imported(ctx context.Context, project, checksum string) (bool, error) {
	var ok bool
	e := w.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM imports WHERE project_id=$1 AND checksum=$2)", project, checksum).Scan(&ok)
	return ok, e
}
func (w writer) EntityEvents(ctx context.Context, project, column, id string) ([]memory.Row, error) {
	if column != "task_id" && column != "session_id" {
		return nil, errors.New("invalid entity column")
	}
	return Rows(ctx, w.Tx, "SELECT jsonb_build_object('kind',kind,'details',details) FROM events WHERE project_id=$1 AND "+column+"=$2", project, id)
}
func (w writer) Finish(ctx context.Context, project string, p portable.Provenance) error {
	if _, e := w.Exec(ctx, "UPDATE projects SET event_count=c.n,event_budget=GREATEST(event_budget,LEAST(c.n,10000000)) FROM (SELECT count(*) n FROM events WHERE project_id=$1) c WHERE id=$1", project); e != nil {
		return e
	}
	counts, _ := json.Marshal(p.Counts)
	_, e := w.Exec(ctx, "INSERT INTO imports(project_id,export_id,source_edition,source_version,format_version,checksum,counts,principal_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", project, p.ExportID, p.SourceEdition, p.SourceVersion, p.FormatVersion, p.Checksum, json.RawMessage(counts), p.Principal)
	return e
}
func (w writer) Commit(ctx context.Context) error   { return w.Tx.Commit(ctx) }
func (w writer) Rollback(ctx context.Context) error { return w.Tx.Rollback(ctx) }
