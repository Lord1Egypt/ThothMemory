package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/portable"
)

// quote lists identifiers from the fixed portable table specification.
func quote(names []string) string { return strings.Join(names, ",") }

func scope(t portable.Table) string {
	if t.Name == "projects" {
		return "id=?"
	}
	return "project_id=?"
}

type reader struct {
	*sql.Tx
	s *Store
}

// ExportReader reads every table from one consistent InnoDB snapshot.
func (s *Store) ExportReader(ctx context.Context) (portable.Reader, error) {
	t, e := s.DB.BeginTx(ctx, s.D.SnapshotTx)
	if e != nil {
		return nil, s.D.Wrap(e)
	}
	return reader{t, s}, nil
}

func (r reader) Select(ctx context.Context, t portable.Table, project string) ([]memory.Row, error) {
	return r.s.rows(ctx, r.Tx, "SELECT "+quote(t.Columns)+" FROM "+t.Name+" WHERE "+scope(t), project)
}
func (r reader) Close(context.Context) error { return r.Rollback() }

type writer struct {
	*sql.Tx
	s *Store
}

func (s *Store) ImportWriter(ctx context.Context) (portable.Writer, error) {
	t, e := s.DB.BeginTx(ctx, s.D.WriteTx)
	if e != nil {
		return nil, s.D.Wrap(e)
	}
	return writer{t, s}, nil
}

func keyWhere(t portable.Table) string {
	parts := make([]string, len(t.Key))
	for i, k := range t.Key {
		parts[i] = k + "=?"
	}
	return strings.Join(parts, " AND ")
}

func (w writer) dbValues(vals []any) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		switch x := v.(type) {
		case []byte:
			out[i] = string(x)
		case time.Time:
			out[i] = w.s.D.Time(x)
		default:
			out[i] = v
		}
	}
	return out
}

func (w writer) Get(ctx context.Context, t portable.Table, key []any) (memory.Row, bool, error) {
	r, e := w.s.rows(ctx, w.Tx, "SELECT "+quote(t.Columns)+" FROM "+t.Name+" WHERE "+keyWhere(t)+w.s.D.ForUpdate, key...)
	if e != nil || len(r) == 0 {
		return nil, false, e
	}
	return r[0], true, nil
}

// search derives the stored search text exactly as live writes do.
func search(t portable.Table, vals []any) (string, bool) {
	v := func(c string) string {
		for i, col := range t.Columns {
			if col == c {
				s, _ := vals[i].(string)
				return s
			}
		}
		return ""
	}
	switch t.Name {
	case "tasks":
		return memory.SearchText(v("title")), true
	case "records":
		return memory.SearchText(v("title"), v("body"), v("git_revision")), true
	case "evidence":
		return memory.SearchText(v("locator"), v("summary"), v("git_revision"), v("artifact_digest")), true
	case "events":
		return memory.SearchText(v("kind"), v("body"), v("git_revision")), true
	}
	return "", false
}

func (w writer) Insert(ctx context.Context, t portable.Table, vals []any) error {
	cols, args := append([]string{}, t.Columns...), w.dbValues(vals)
	if st, ok := search(t, vals); ok {
		cols, args = append(cols, "search_text"), append(args, st)
	}
	_, e := w.ExecContext(ctx, "INSERT INTO "+t.Name+"("+quote(cols)+") VALUES("+in(len(cols))+")", args...)
	return w.s.D.Wrap(e)
}

func (w writer) Update(ctx context.Context, t portable.Table, vals []any) error {
	if !t.Mutable {
		return errors.New("history is append-only")
	}
	sets := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		sets[i] = c + "=?"
	}
	args := w.dbValues(vals)
	for _, k := range t.Key {
		for i, c := range t.Columns {
			if c == k {
				args = append(args, vals[i])
			}
		}
	}
	_, e := w.ExecContext(ctx, "UPDATE "+t.Name+" SET "+strings.Join(sets, ",")+" WHERE "+keyWhere(t), args...)
	return w.s.D.Wrap(e)
}

func (w writer) EnsurePrincipal(ctx context.Context, id string) error {
	_, e := w.ExecContext(ctx, "INSERT INTO principals(id,created_at) VALUES(?,?)", id, w.s.now())
	return w.s.ignoreDuplicate(e)
}
func (w writer) EnsureAgent(ctx context.Context, id string) error {
	_, e := w.ExecContext(ctx, "INSERT INTO agents(id) VALUES(?)", id)
	return w.s.ignoreDuplicate(e)
}
func (w writer) Imported(ctx context.Context, project, checksum string) (bool, error) {
	var ok bool
	e := w.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM imports WHERE project_id=? AND checksum=?)", project, checksum).Scan(&ok)
	return ok, w.s.D.Wrap(e)
}
func (w writer) EntityEvents(ctx context.Context, project, column, id string) ([]memory.Row, error) {
	if column != "task_id" && column != "session_id" {
		return nil, errors.New("invalid entity column")
	}
	return w.s.rows(ctx, w.Tx, "SELECT kind,details FROM events WHERE project_id=? AND "+column+"=?", project, id)
}
func (w writer) Finish(ctx context.Context, project string, p portable.Provenance) error {
	if _, e := w.ExecContext(ctx, "UPDATE projects SET event_count=(SELECT COUNT(*) FROM events WHERE project_id=?) WHERE id=?", project, project); e != nil {
		return w.s.D.Wrap(e)
	}
	if _, e := w.ExecContext(ctx, "UPDATE projects SET event_budget=CASE WHEN event_count>event_budget THEN CASE WHEN event_count>10000000 THEN 10000000 ELSE event_count END ELSE event_budget END WHERE id=?", project); e != nil {
		return w.s.D.Wrap(e)
	}
	counts, _ := json.Marshal(p.Counts)
	_, e := w.ExecContext(ctx, "INSERT INTO imports(id,project_id,export_id,source_edition,source_version,format_version,checksum,counts,principal_id,imported_at) VALUES(?,?,?,?,?,?,?,?,?,?)", memory.NewID(), project, p.ExportID, p.SourceEdition, p.SourceVersion, p.FormatVersion, p.Checksum, string(counts), p.Principal, w.s.now())
	return w.s.D.Wrap(e)
}
func (w writer) Commit(context.Context) error   { return w.s.D.Wrap(w.Tx.Commit()) }
func (w writer) Rollback(context.Context) error { return w.Tx.Rollback() }
