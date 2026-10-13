package mariadb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
)

type tx struct{ *sql.Tx }

func (s *Store) Begin(ctx context.Context) (memory.Tx, error) {
	t, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return nil, wrap(e)
	}
	return tx{t}, nil
}

func (t tx) Commit(context.Context) error   { return wrap(t.Tx.Commit()) }
func (t tx) Rollback(context.Context) error { return wrap(t.Tx.Rollback()) }

func (t tx) exec(ctx context.Context, query string, args ...any) (int64, error) {
	r, e := t.ExecContext(ctx, query, args...)
	if e != nil {
		return 0, wrap(e)
	}
	return r.RowsAffected()
}

func (t tx) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var ok bool
	e := t.QueryRowContext(ctx, "SELECT EXISTS("+query+")", args...).Scan(&ok)
	return ok, wrap(e)
}

// guarded runs a conditional UPDATE and returns the row, or ErrConflict when
// the guard (revision, claim, open session) matched nothing. ClientFoundRows
// makes RowsAffected count matched rows.
func (t tx) guarded(ctx context.Context, update, reload string, args []any, id string) (memory.Row, error) {
	n, e := t.exec(ctx, update, args...)
	if e != nil {
		return nil, e
	}
	if n != 1 {
		return nil, memory.ErrConflict
	}
	return one(ctx, t.Tx, reload, id)
}

// LockOperation locks the project row. PostgreSQL serializes a project's
// mutations through its event-budget row update; holding the same row lock
// from the start gives identical ordering without session-scoped GET_LOCK.
func (t tx) LockOperation(ctx context.Context, project, _, _ string) error {
	r, e := t.QueryContext(ctx, "SELECT id FROM projects WHERE id=? FOR UPDATE", project)
	if e != nil {
		return wrap(e)
	}
	return wrap(r.Close())
}

func (t tx) Operation(ctx context.Context, project, principal, operation string) (string, []byte, bool, error) {
	var hash string
	var cached []byte
	e := t.QueryRowContext(ctx, "SELECT payload_hash,response FROM operations WHERE project_id=? AND principal_id=? AND operation_id=?", project, principal, operation).Scan(&hash, &cached)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	return hash, cached, e == nil, wrap(e)
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
	return ignoreDuplicate(e)
}

func (t tx) InsertTask(ctx context.Context, project, title, revision string) (memory.Row, error) {
	id := newID()
	if _, e := t.exec(ctx, "INSERT INTO tasks(id,project_id,title,git_revision,search_text) VALUES(?,?,?,?,?)", id, project, title, revision, searchText(title)); e != nil {
		return nil, e
	}
	return one(ctx, t.Tx, "SELECT "+taskCols+" FROM tasks WHERE id=?", id)
}

func (t tx) InsertDependency(ctx context.Context, project, task, dependsOn string) error {
	_, e := t.exec(ctx, "INSERT INTO task_dependencies(project_id,task_id,depends_on) VALUES(?,?,?)", project, task, dependsOn)
	return e
}

func (t tx) InsertSession(ctx context.Context, project, agent, principal, branch, revision string) (memory.Row, error) {
	id := newID()
	if _, e := t.exec(ctx, "INSERT INTO sessions(id,project_id,agent_id,principal_id,branch,git_revision) VALUES(?,?,?,?,?,?)", id, project, agent, principal, branch, revision); e != nil {
		return nil, e
	}
	return one(ctx, t.Tx, "SELECT "+sessionCols+" FROM sessions WHERE id=?", id)
}

func (t tx) AdvanceSession(ctx context.Context, project, session string, expected int, revision, branch string, finish bool) (memory.Row, error) {
	return t.guarded(ctx, "UPDATE sessions SET git_revision=?,branch=IF(?='',branch,?),revision=revision+1,finished_at=IF(?,UTC_TIMESTAMP(6),NULL) WHERE project_id=? AND id=? AND revision=? AND finished_at IS NULL",
		"SELECT "+sessionCols+" FROM sessions WHERE id=?", []any{revision, branch, branch, finish, project, session, expected}, session)
}

func (t tx) ReleaseTask(ctx context.Context, project, task string, expected int, principal string) (memory.Row, error) {
	return t.guarded(ctx, "UPDATE tasks SET claimed_by=NULL,claim_expires_at=NULL,revision=revision+1,updated_at=UTC_TIMESTAMP(6) WHERE project_id=? AND id=? AND revision=? AND claimed_by=?",
		"SELECT "+taskCols+" FROM tasks WHERE id=?", []any{project, task, expected, principal}, task)
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
	return t.guarded(ctx, "UPDATE tasks SET state=?,git_revision=IF(?='',git_revision,?),"+
		"claimed_by=CASE WHEN ?='planned' THEN NULL WHEN ? THEN ? ELSE claimed_by END,"+
		"claim_expires_at=CASE WHEN ?='planned' THEN NULL WHEN ? THEN UTC_TIMESTAMP(6)+INTERVAL 30 MINUTE ELSE claim_expires_at END,"+
		"revision=revision+1,updated_at=UTC_TIMESTAMP(6) WHERE project_id=? AND id=? AND revision=? AND (claimed_by IS NULL OR claimed_by=? OR (? AND claim_expires_at<UTC_TIMESTAMP(6)))",
		"SELECT "+taskCols+" FROM tasks WHERE id=?",
		[]any{state, revision, revision, state, claim, principal, state, claim, project, task, expected, principal, claim}, task)
}

func (t tx) RecordIs(ctx context.Context, project, id, kind string) (bool, error) {
	return t.exists(ctx, "SELECT 1 FROM records WHERE project_id=? AND id=? AND kind=?", project, id, kind)
}

func (t tx) InsertRecord(ctx context.Context, r memory.RecordInput) (memory.Row, error) {
	id := newID()
	if _, e := t.exec(ctx, "INSERT INTO records(id,project_id,task_id,session_id,kind,title,body,git_revision,principal_id,agent_id,evidence_id,supersedes,search_text) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)",
		id, r.Project, nullable(r.Task), nullable(r.Session), r.Kind, r.Title, r.Body, r.Revision, r.Principal, r.Agent, nullable(r.Evidence), nullable(r.Supersedes), searchText(r.Title, r.Body, r.Revision)); e != nil {
		return nil, e
	}
	return one(ctx, t.Tx, "SELECT "+recordCols+" FROM records WHERE id=?", id)
}

func (t tx) InsertEvidence(ctx context.Context, v memory.EvidenceInput) (memory.Row, error) {
	id := newID()
	if _, e := t.exec(ctx, "INSERT INTO evidence(id,project_id,task_id,source_kind,locator,git_revision,artifact_digest,outcome,summary,reported_by,search_text) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
		id, v.Project, nullable(v.Task), v.SourceKind, v.Locator, v.Revision, v.Digest, v.Outcome, v.Summary, v.ReportedBy, searchText(v.Locator, v.Summary, v.Revision, v.Digest)); e != nil {
		return nil, e
	}
	return one(ctx, t.Tx, "SELECT "+evidenceCols+" FROM evidence WHERE id=?", id)
}

func (t tx) InsertAttestation(ctx context.Context, project, evidence, principal, statement string) (memory.Row, error) {
	id := newID()
	if _, e := t.exec(ctx, "INSERT INTO attestations(id,project_id,evidence_id,principal_id,statement) VALUES(?,?,?,?,?)", id, project, evidence, principal, statement); e != nil {
		return nil, e
	}
	return one(ctx, t.Tx, "SELECT "+attestationCols+" FROM attestations WHERE id=?", id)
}

func (t tx) InsertEvent(ctx context.Context, v memory.EventInput) error {
	_, e := t.exec(ctx, "INSERT INTO events(id,project_id,task_id,session_id,kind,body,principal_id,agent_id,git_revision,details,search_text) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
		newID(), v.Project, nullable(v.Task), nullable(v.Session), v.Kind, v.Body, v.Principal, v.Agent, v.Revision, string(v.Details), searchText(v.Kind, v.Body, v.Revision))
	return e
}

func (t tx) InsertOperation(ctx context.Context, project, operation, principal, hash string, response []byte) error {
	_, e := t.exec(ctx, "INSERT INTO operations(project_id,operation_id,principal_id,payload_hash,response) VALUES(?,?,?,?,?)", project, operation, principal, hash, string(response))
	return e
}
