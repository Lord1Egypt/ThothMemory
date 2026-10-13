package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
)

type tx struct {
	*sql.Tx
	s   *Store
	now time.Time
}

// Begin starts a mutation. One timestamp per transaction gives every row the
// same time, as PostgreSQL's now() does.
func (s *Store) Begin(ctx context.Context) (memory.Tx, error) {
	t, e := s.DB.BeginTx(ctx, s.D.WriteTx)
	if e != nil {
		return nil, s.D.Wrap(e)
	}
	return tx{t, s, time.Now()}, nil
}

func (t tx) Commit(context.Context) error   { return t.s.D.Wrap(t.Tx.Commit()) }
func (t tx) Rollback(context.Context) error { return t.s.D.Wrap(t.Tx.Rollback()) }

func (t tx) ts() any { return t.s.D.Time(t.now) }

func (t tx) exec(ctx context.Context, query string, args ...any) (int64, error) {
	r, e := t.ExecContext(ctx, query, args...)
	if e != nil {
		return 0, t.s.D.Wrap(e)
	}
	return r.RowsAffected()
}

func (t tx) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var ok bool
	e := t.QueryRowContext(ctx, "SELECT EXISTS("+query+")", args...).Scan(&ok)
	return ok, t.s.D.Wrap(e)
}

// guarded runs a conditional UPDATE and returns the row, or ErrConflict when
// the guard (revision, claim, open session) matched nothing. Both engines
// report matched rows here (MariaDB with ClientFoundRows).
func (t tx) guarded(ctx context.Context, update, reload string, args []any, id string) (memory.Row, error) {
	n, e := t.exec(ctx, update, args...)
	if e != nil {
		return nil, e
	}
	if n != 1 {
		return nil, memory.ErrConflict
	}
	return t.s.one(ctx, t.Tx, reload, id)
}

// LockOperation locks the project row. PostgreSQL serializes a project's
// mutations through its event-budget row update; holding the same row lock
// from the start gives identical ordering. SQLite writers already hold the
// database write lock, so the read takes no further lock there.
func (t tx) LockOperation(ctx context.Context, project, _, _ string) error {
	r, e := t.QueryContext(ctx, "SELECT id FROM projects WHERE id=?"+t.s.D.ForUpdate, project)
	if e != nil {
		return t.s.D.Wrap(e)
	}
	return t.s.D.Wrap(r.Close())
}

func (t tx) Operation(ctx context.Context, project, principal, operation string) (string, []byte, bool, error) {
	var hash string
	var cached []byte
	e := t.QueryRowContext(ctx, "SELECT payload_hash,response FROM operations WHERE project_id=? AND principal_id=? AND operation_id=?", project, principal, operation).Scan(&hash, &cached)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	return hash, cached, e == nil, t.s.D.Wrap(e)
}

func (t tx) ReserveEvent(ctx context.Context, project string) (bool, error) {
	n, e := t.exec(ctx, "UPDATE projects SET event_count=event_count+1 WHERE id=? AND event_count<event_budget", project)
	return n == 1, e
}

func (t tx) SessionOpen(ctx context.Context, project, session, principal string) (bool, error) {
	return t.exists(ctx, "SELECT 1 FROM sessions WHERE project_id=? AND id=? AND principal_id=? AND finished_at IS NULL", project, session, principal)
}

func (t tx) EnsureAgent(ctx context.Context, agent string) error {
	_, e := t.ExecContext(ctx, "INSERT INTO agents(id) VALUES(?)", agent)
	return t.s.ignoreDuplicate(e)
}

func (t tx) InsertTask(ctx context.Context, project, title, revision string) (memory.Row, error) {
	id := memory.NewID()
	if _, e := t.exec(ctx, "INSERT INTO tasks(id,project_id,title,git_revision,updated_at,search_text) VALUES(?,?,?,?,?,?)", id, project, title, revision, t.ts(), memory.SearchText(title)); e != nil {
		return nil, e
	}
	return t.s.one(ctx, t.Tx, "SELECT "+memory.TaskColumns+" FROM tasks WHERE id=?", id)
}

func (t tx) InsertDependency(ctx context.Context, project, task, dependsOn string) error {
	_, e := t.exec(ctx, "INSERT INTO task_dependencies(project_id,task_id,depends_on) VALUES(?,?,?)", project, task, dependsOn)
	return e
}

func (t tx) InsertSession(ctx context.Context, project, agent, principal, branch, revision string) (memory.Row, error) {
	id := memory.NewID()
	if _, e := t.exec(ctx, "INSERT INTO sessions(id,project_id,agent_id,principal_id,branch,git_revision,started_at) VALUES(?,?,?,?,?,?,?)", id, project, agent, principal, branch, revision, t.ts()); e != nil {
		return nil, e
	}
	return t.s.one(ctx, t.Tx, "SELECT "+memory.SessionColumns+" FROM sessions WHERE id=?", id)
}

func (t tx) AdvanceSession(ctx context.Context, project, session string, expected int, revision, branch string, finish bool) (memory.Row, error) {
	var finished any
	if finish {
		finished = t.ts()
	}
	return t.guarded(ctx, "UPDATE sessions SET git_revision=?,branch=CASE WHEN ?='' THEN branch ELSE ? END,revision=revision+1,finished_at=? WHERE project_id=? AND id=? AND revision=? AND finished_at IS NULL",
		"SELECT "+memory.SessionColumns+" FROM sessions WHERE id=?", []any{revision, branch, branch, finished, project, session, expected}, session)
}

func (t tx) ReleaseTask(ctx context.Context, project, task string, expected int, principal string) (memory.Row, error) {
	return t.guarded(ctx, "UPDATE tasks SET claimed_by=NULL,claim_expires_at=NULL,revision=revision+1,updated_at=? WHERE project_id=? AND id=? AND revision=? AND claimed_by=?",
		"SELECT "+memory.TaskColumns+" FROM tasks WHERE id=?", []any{t.ts(), project, task, expected, principal}, task)
}

func (t tx) EvidenceSatisfies(ctx context.Context, project, task, revision string, r memory.Requirement) (bool, error) {
	args := append([]any{project, task, revision}, anyArgs(r.Kinds)...)
	args = append(args, r.Attested)
	return t.exists(ctx, "SELECT 1 FROM evidence e WHERE e.project_id=? AND e.task_id=? AND e.git_revision=? AND e.outcome='pass' AND e.source_kind IN ("+in(len(r.Kinds))+") AND (NOT ? OR EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id))", args...)
}

func (t tx) DependenciesOpen(ctx context.Context, project, task string) (bool, error) {
	return t.exists(ctx, "SELECT 1 FROM task_dependencies d JOIN tasks t ON t.id=d.depends_on WHERE d.project_id=? AND d.task_id=? AND t.state NOT IN ('verified','released')", project, task)
}

// UpdateTask mirrors the PostgreSQL statement. MariaDB evaluates SET clauses
// left to right; no assignment here reads a column assigned before it.
func (t tx) UpdateTask(ctx context.Context, project, task string, expected int, state, revision string, claim bool, principal string) (memory.Row, error) {
	now := t.ts()
	expires := t.s.D.Time(t.now.Add(time.Duration(memory.ClaimMinutes) * time.Minute))
	return t.guarded(ctx, "UPDATE tasks SET state=?,git_revision=CASE WHEN ?='' THEN git_revision ELSE ? END,"+
		"claimed_by=CASE WHEN ?='planned' THEN NULL WHEN ? THEN ? ELSE claimed_by END,"+
		"claim_expires_at=CASE WHEN ?='planned' THEN NULL WHEN ? THEN ? ELSE claim_expires_at END,"+
		"revision=revision+1,updated_at=? WHERE project_id=? AND id=? AND revision=? AND (claimed_by IS NULL OR claimed_by=? OR (? AND claim_expires_at<?))",
		"SELECT "+memory.TaskColumns+" FROM tasks WHERE id=?",
		[]any{state, revision, revision, state, claim, principal, state, claim, expires, now, project, task, expected, principal, claim, now}, task)
}

func (t tx) RecordIs(ctx context.Context, project, id, kind string) (bool, error) {
	return t.exists(ctx, "SELECT 1 FROM records WHERE project_id=? AND id=? AND kind=?", project, id, kind)
}

func (t tx) FindRecord(ctx context.Context, project, kind, title, body string) (memory.Row, bool, error) {
	r, e := t.s.rows(ctx, t.Tx, "SELECT "+memory.RecordColumns+" FROM records WHERE project_id=? AND kind=? AND title=? AND body=? ORDER BY created_at DESC,id LIMIT 1", project, kind, title, body)
	if e != nil || len(r) == 0 {
		return nil, false, e
	}
	return r[0], true, nil
}

func (t tx) InsertRecord(ctx context.Context, r memory.RecordInput) (memory.Row, error) {
	id := memory.NewID()
	if _, e := t.exec(ctx, "INSERT INTO records(id,project_id,task_id,session_id,kind,title,body,git_revision,principal_id,agent_id,evidence_id,supersedes,created_at,search_text) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
		id, r.Project, nullable(r.Task), nullable(r.Session), r.Kind, r.Title, r.Body, r.Revision, r.Principal, r.Agent, nullable(r.Evidence), nullable(r.Supersedes), t.ts(), memory.SearchText(r.Title, r.Body, r.Revision)); e != nil {
		return nil, e
	}
	return t.s.one(ctx, t.Tx, "SELECT "+memory.RecordColumns+" FROM records WHERE id=?", id)
}

func (t tx) InsertEvidence(ctx context.Context, v memory.EvidenceInput) (memory.Row, error) {
	id := memory.NewID()
	if _, e := t.exec(ctx, "INSERT INTO evidence(id,project_id,task_id,source_kind,locator,git_revision,artifact_digest,outcome,summary,reported_by,created_at,search_text) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
		id, v.Project, nullable(v.Task), v.SourceKind, v.Locator, v.Revision, v.Digest, v.Outcome, v.Summary, v.ReportedBy, t.ts(), memory.SearchText(v.Locator, v.Summary, v.Revision, v.Digest)); e != nil {
		return nil, e
	}
	return t.s.one(ctx, t.Tx, "SELECT "+memory.EvidenceColumns+" FROM evidence WHERE id=?", id)
}

func (t tx) InsertAttestation(ctx context.Context, project, evidence, principal, statement string) (memory.Row, error) {
	id := memory.NewID()
	if _, e := t.exec(ctx, "INSERT INTO attestations(id,project_id,evidence_id,principal_id,statement,created_at) VALUES(?,?,?,?,?,?)", id, project, evidence, principal, statement, t.ts()); e != nil {
		return nil, e
	}
	return t.s.one(ctx, t.Tx, "SELECT "+memory.AttestationColumns+" FROM attestations WHERE id=?", id)
}

func (t tx) InsertEvent(ctx context.Context, v memory.EventInput) error {
	_, e := t.exec(ctx, "INSERT INTO events(id,project_id,task_id,session_id,kind,body,principal_id,agent_id,git_revision,created_at,details,search_text) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
		memory.NewID(), v.Project, nullable(v.Task), nullable(v.Session), v.Kind, v.Body, v.Principal, v.Agent, v.Revision, t.ts(), string(v.Details), memory.SearchText(v.Kind, v.Body, v.Revision))
	return e
}

func (t tx) InsertOperation(ctx context.Context, project, operation, principal, hash string, response []byte) error {
	_, e := t.exec(ctx, "INSERT INTO operations(project_id,operation_id,principal_id,payload_hash,response,created_at) VALUES(?,?,?,?,?,?)", project, operation, principal, hash, string(response), t.ts())
	return e
}
