package store

import (
	"context"
	"errors"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/jackc/pgx/v5"
)

const searchCols = "-'search_vector'-'search_simple'-'search_text'"

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *Store) ListProjects(ctx context.Context, all bool, ids []string, limit, offset int) ([]memory.Row, error) {
	return Rows(ctx, s.Pool, "SELECT to_jsonb(p) FROM projects p WHERE $1 OR id::text=ANY($2::text[]) ORDER BY name,id LIMIT $3 OFFSET $4", all, ids, limit, offset)
}

func (s *Store) CreateProject(ctx context.Context, name, repo string) (memory.Row, error) {
	r, e := Rows(ctx, s.Pool, "INSERT INTO projects(name,repository) VALUES($1,$2) ON CONFLICT(repository) DO UPDATE SET repository=EXCLUDED.repository RETURNING to_jsonb(projects)", name, repo)
	if e != nil {
		return nil, e
	}
	return r[0], nil
}

func (s *Store) Tasks(ctx context.Context, project, task, state string, limit, offset int) ([]memory.Row, error) {
	return Rows(ctx, s.Pool, "SELECT (to_jsonb(t)"+searchCols+") FROM tasks t WHERE project_id=$1 AND ($2='' OR id::text=$2) AND ($3='' OR state=$3) ORDER BY updated_at DESC,id LIMIT $4 OFFSET $5", project, task, state, limit, offset)
}

func (s *Store) Activity(ctx context.Context, project, task string, limit, offset int) ([]memory.Row, error) {
	return s.events(ctx, project, task, "", limit, offset)
}

func (s *Store) events(ctx context.Context, project, task, kind string, limit, offset int) ([]memory.Row, error) {
	return Rows(ctx, s.Pool, "SELECT (to_jsonb(e)"+searchCols+"-'details') FROM events e WHERE project_id=$1 AND ($2='' OR task_id::text=$2) AND ($3='' OR kind=$3) ORDER BY created_at DESC,id LIMIT $4 OFFSET $5", project, task, kind, limit, offset)
}

func (s *Store) LatestHandoff(ctx context.Context, project string) ([]memory.Row, error) {
	return Rows(ctx, s.Pool, "SELECT (to_jsonb(r)"+searchCols+") FROM records r WHERE project_id=$1 AND kind='handoff' ORDER BY created_at DESC,id LIMIT 1", project)
}

func (s *Store) Search(ctx context.Context, project string, q memory.SearchQuery) ([]memory.Row, error) {
	// Evaluate normalized queries once even when PostgreSQL selects a generic
	// prepared plan; inlining recomputed text-search parsers for every row.
	return Rows(ctx, s.Pool, `WITH q AS MATERIALIZED (SELECT normalize($2::text) AS text,websearch_to_tsquery('english',normalize($2::text)) AS english,websearch_to_tsquery('simple',normalize($2::text)) AS simple)
 SELECT to_jsonb(r)`+searchCols+` || jsonb_build_object('superseded',EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id),'stale',CASE WHEN $9='' THEN NULL ELSE r.git_revision<>$9 END,'provenance',jsonb_build_object('principal',r.principal_id,'agent',r.agent_id,'evidence_id',r.evidence_id,'trust','reported'),'rank',ts_rank(r.search_vector,q.english)+ts_rank(r.search_simple,q.simple))
 FROM memory_search r CROSS JOIN q WHERE project_id=$1 AND (q.text='' OR r.search_vector@@q.english OR r.search_simple@@q.simple OR strpos(lower(r.search_text),lower(q.text))>0)
 AND ($3='' OR task_id::text=$3) AND ($4='' OR agent_id=$4) AND ($5='' OR evidence_id::text=$5) AND ($6::timestamptz IS NULL OR created_at>=$6) AND ($7::timestamptz IS NULL OR created_at<=$7) AND ($8='' OR id::text=$8)
 ORDER BY (ts_rank(r.search_vector,q.english)+ts_rank(r.search_simple,q.simple)) DESC,created_at DESC,id LIMIT $10 OFFSET $11`, project, q.Text, q.TaskID, q.Agent, q.EvidenceID, q.After, q.Before, q.ExactID, q.CurrentRevision, q.Limit, q.Offset)
}

var contextQueries = map[string]string{
	"project":              "SELECT to_jsonb(p) FROM projects p WHERE id=$1",
	"tasks":                "SELECT (to_jsonb(t)" + searchCols + ") FROM tasks t WHERE project_id=$1 ORDER BY (state IN ('active','blocked')) DESC,updated_at DESC,id LIMIT 12",
	"sessions":             "SELECT to_jsonb(s) || jsonb_build_object('status',CASE WHEN finished_at IS NULL THEN 'incomplete' ELSE 'finished' END) FROM sessions s WHERE project_id=$1 ORDER BY started_at DESC,id LIMIT 4",
	"checkpoints":          "SELECT (to_jsonb(r)" + searchCols + ") FROM records r WHERE project_id=$1 AND session_id IS NOT NULL AND kind='observation' ORDER BY created_at DESC,id LIMIT 4",
	"handoff":              "SELECT (to_jsonb(r)" + searchCols + ") FROM records r WHERE project_id=$1 AND kind='handoff' ORDER BY created_at DESC,id LIMIT 1",
	"decisions_and_issues": "SELECT (to_jsonb(r)" + searchCols + ") FROM records r WHERE project_id=$1 AND kind IN ('decision','issue') AND NOT EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id) ORDER BY created_at DESC,id LIMIT 8",
	"evidence":             "SELECT (to_jsonb(e)" + searchCols + ") || jsonb_build_object('owner_attested',EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id)) FROM evidence e WHERE project_id=$1 ORDER BY created_at DESC,id LIMIT 8",
}

func (s *Store) Context(ctx context.Context, project string) (map[string][]memory.Row, error) {
	out := map[string][]memory.Row{}
	for _, name := range memory.ContextSections {
		v, e := Rows(ctx, s.Pool, contextQueries[name], project)
		if e != nil {
			return nil, e
		}
		out[name] = v
	}
	return out, nil
}

// awaitingAttestation is the shared definition of evidence an owner could attest
// to unlock verified or released task states.
const awaitingAttestation = "e.outcome='pass' AND e.source_kind IN ('test','ci','release') AND NOT EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id)"

func (s *Store) Overview(ctx context.Context, all bool, ids []string) ([]memory.Row, error) {
	return Rows(ctx, s.Pool, `SELECT to_jsonb(p) || jsonb_build_object(
 'last_activity_at',(SELECT max(created_at) FROM events e WHERE e.project_id=p.id),
 'last_observed',(SELECT jsonb_build_object('git_revision',e.git_revision,'observed_at',e.created_at,'agent_id',e.agent_id,'principal_id',e.principal_id,'source','event','source_id',e.id) FROM events e WHERE e.project_id=p.id AND e.git_revision<>'' ORDER BY created_at DESC,id DESC LIMIT 1),
 'last_session',(SELECT jsonb_build_object('id',s.id,'agent_id',s.agent_id,'principal_id',s.principal_id,'branch',s.branch,'git_revision',s.git_revision,'started_at',s.started_at,'finished_at',s.finished_at,'status',CASE WHEN s.finished_at IS NULL THEN 'incomplete' ELSE 'finished' END) FROM sessions s WHERE s.project_id=p.id ORDER BY started_at DESC,id DESC LIMIT 1),
 'task_counts',(SELECT COALESCE(jsonb_object_agg(state,n),'{}'::jsonb) FROM (SELECT state,count(*) n FROM tasks t WHERE t.project_id=p.id GROUP BY state) c),
 'incomplete_sessions',(SELECT count(*) FROM sessions s WHERE s.project_id=p.id AND s.finished_at IS NULL),
 'open_issues',(SELECT count(*) FROM records r WHERE r.project_id=p.id AND r.kind='issue' AND NOT EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id)),
 'current_decisions',(SELECT count(*) FROM records r WHERE r.project_id=p.id AND r.kind='decision' AND NOT EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id)),
 'latest_handoff',(SELECT jsonb_build_object('id',r.id,'created_at',r.created_at,'agent_id',r.agent_id,'principal_id',r.principal_id,'git_revision',r.git_revision,'excerpt',left(r.body,280)) FROM records r WHERE r.project_id=p.id AND r.kind='handoff' ORDER BY created_at DESC,id DESC LIMIT 1),
 'latest_checkpoint',(SELECT jsonb_build_object('id',r.id,'created_at',r.created_at,'agent_id',r.agent_id,'session_id',r.session_id,'git_revision',r.git_revision,'excerpt',left(r.body,280)) FROM records r WHERE r.project_id=p.id AND r.kind='observation' AND r.session_id IS NOT NULL ORDER BY created_at DESC,id DESC LIMIT 1),
 'latest_evidence',(SELECT jsonb_build_object('id',e.id,'created_at',e.created_at,'git_revision',e.git_revision,'source_kind',e.source_kind,'outcome',e.outcome,'owner_attested',EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id)) FROM evidence e WHERE e.project_id=p.id ORDER BY created_at DESC,id DESC LIMIT 1),
 'evidence_total',(SELECT count(*) FROM evidence e WHERE e.project_id=p.id),
 'evidence_attested',(SELECT count(*) FROM attestations x WHERE x.project_id=p.id),
 'evidence_awaiting_attestation',(SELECT count(*) FROM evidence e WHERE e.project_id=p.id AND `+awaitingAttestation+`))
 FROM projects p WHERE $1 OR id::text=ANY($2::text[]) ORDER BY name,id LIMIT 200`, all, ids)
}

func (s *Store) Explore(ctx context.Context, project string, q memory.ExploreQuery) ([]memory.Row, error) {
	switch q.View {
	case "sessions":
		return Rows(ctx, s.Pool, `SELECT to_jsonb(s) || jsonb_build_object('status',CASE WHEN finished_at IS NULL THEN 'incomplete' ELSE 'finished' END,
 'checkpoints',(SELECT count(*) FROM records r WHERE r.project_id=s.project_id AND r.session_id=s.id),
 'last_checkpoint_at',(SELECT max(created_at) FROM records r WHERE r.project_id=s.project_id AND r.session_id=s.id))
 FROM sessions s WHERE project_id=$1 AND ($2='' OR ($2='incomplete')=(finished_at IS NULL)) ORDER BY started_at DESC,id LIMIT $3 OFFSET $4`, project, q.Kind, q.Limit, q.Offset)
	case "records":
		return Rows(ctx, s.Pool, `SELECT (to_jsonb(r)`+searchCols+`) || jsonb_build_object('superseded_by',(SELECT n.id FROM records n WHERE n.supersedes=r.id),'superseded',EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id))
 FROM records r WHERE project_id=$1 AND ($2='' OR kind=$2) AND ($3='' OR task_id::text=$3) ORDER BY created_at DESC,id LIMIT $4 OFFSET $5`, project, q.Kind, q.TaskID, q.Limit, q.Offset)
	case "evidence":
		return Rows(ctx, s.Pool, `SELECT (to_jsonb(e)`+searchCols+`) || jsonb_build_object('owner_attested',a.id IS NOT NULL,'attestation',CASE WHEN a.id IS NULL THEN NULL ELSE jsonb_build_object('id',a.id,'principal_id',a.principal_id,'statement',a.statement,'created_at',a.created_at) END)
 FROM evidence e LEFT JOIN attestations a ON a.evidence_id=e.id WHERE e.project_id=$1 AND ($2='' OR e.task_id::text=$2)
 AND ($3='' OR ($3='attested' AND a.id IS NOT NULL) OR ($3='awaiting_attestation' AND `+awaitingAttestation+`)) ORDER BY e.created_at DESC,e.id LIMIT $4 OFFSET $5`, project, q.TaskID, q.Kind, q.Limit, q.Offset)
	case "events":
		return s.events(ctx, project, q.TaskID, q.Kind, q.Limit, q.Offset)
	case "imports":
		return Rows(ctx, s.Pool, "SELECT to_jsonb(i) FROM imports i WHERE project_id=$1 ORDER BY imported_at DESC,id LIMIT $2 OFFSET $3", project, q.Limit, q.Offset)
	case "task":
		return Rows(ctx, s.Pool, `SELECT (to_jsonb(t)`+searchCols+`) || jsonb_build_object('section','task') FROM tasks t WHERE project_id=$1 AND id::text=$2
 UNION ALL (SELECT jsonb_build_object('section','dependency','id',t.id,'title',t.title,'state',t.state,'revision',t.revision,'git_revision',t.git_revision) FROM task_dependencies d JOIN tasks t ON t.project_id=d.project_id AND t.id=d.depends_on WHERE d.project_id=$1 AND d.task_id::text=$2 ORDER BY t.title,t.id LIMIT 50)
 UNION ALL (SELECT jsonb_build_object('section','dependent','id',t.id,'title',t.title,'state',t.state,'revision',t.revision,'git_revision',t.git_revision) FROM task_dependencies d JOIN tasks t ON t.project_id=d.project_id AND t.id=d.task_id WHERE d.project_id=$1 AND d.depends_on::text=$2 ORDER BY t.title,t.id LIMIT 50)
 UNION ALL (SELECT (to_jsonb(e)`+searchCols+`) || jsonb_build_object('section','evidence','owner_attested',EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id)) FROM evidence e WHERE e.project_id=$1 AND e.task_id::text=$2 ORDER BY e.created_at DESC,e.id LIMIT 50)
 UNION ALL (SELECT jsonb_build_object('section','transition','id',e.id,'kind',e.kind,'body',e.body,'principal_id',e.principal_id,'agent_id',e.agent_id,'git_revision',e.git_revision,'created_at',e.created_at,'details',e.details::text) FROM events e WHERE e.project_id=$1 AND e.task_id::text=$2 ORDER BY e.created_at DESC,e.id LIMIT 100)`, project, q.TaskID)
	}
	return nil, errors.New("unknown view")
}

type tx struct{ pgx.Tx }

func (s *Store) Begin(ctx context.Context) (memory.Tx, error) {
	t, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	return tx{t}, nil
}

func (t tx) one(ctx context.Context, sql string, args ...any) (memory.Row, error) {
	r, e := Rows(ctx, t.Tx, sql, args...)
	if e != nil {
		return nil, e
	}
	if len(r) != 1 {
		return nil, memory.ErrConflict
	}
	return r[0], nil
}

func (t tx) exists(ctx context.Context, sql string, args ...any) (bool, error) {
	var ok bool
	e := t.QueryRow(ctx, sql, args...).Scan(&ok)
	return ok, e
}

func (t tx) LockOperation(ctx context.Context, project, principal, operation string) error {
	_, e := t.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", project+":"+principal+":"+operation)
	return e
}

func (t tx) Operation(ctx context.Context, project, principal, operation string) (string, []byte, bool, error) {
	var hash string
	var cached []byte
	e := t.QueryRow(ctx, "SELECT payload_hash,response FROM operations WHERE project_id=$1 AND principal_id=$2 AND operation_id=$3", project, principal, operation).Scan(&hash, &cached)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", nil, false, nil
	}
	return hash, cached, e == nil, e
}

func (t tx) ReserveEvent(ctx context.Context, project string) (bool, error) {
	var reserved bool
	e := t.QueryRow(ctx, "UPDATE projects SET event_count=event_count+1 WHERE id=$1 AND event_count<event_budget RETURNING true", project).Scan(&reserved)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	return reserved, e
}

func (t tx) SessionOpen(ctx context.Context, project, session, principal string) (bool, error) {
	return t.exists(ctx, "SELECT EXISTS(SELECT 1 FROM sessions WHERE project_id=$1 AND id=$2 AND principal_id=$3 AND finished_at IS NULL)", project, session, principal)
}

func (t tx) EnsureAgent(ctx context.Context, agent string) error {
	_, e := t.Exec(ctx, "INSERT INTO agents(id) VALUES($1) ON CONFLICT DO NOTHING", agent)
	return e
}

func (t tx) InsertTask(ctx context.Context, project, title, revision string) (memory.Row, error) {
	return t.one(ctx, "INSERT INTO tasks(project_id,title,git_revision) VALUES($1,$2,$3) RETURNING (to_jsonb(tasks)"+searchCols+")", project, title, revision)
}

func (t tx) InsertDependency(ctx context.Context, project, task, dependsOn string) error {
	_, e := t.Exec(ctx, "INSERT INTO task_dependencies(project_id,task_id,depends_on) VALUES($1,$2,$3)", project, task, dependsOn)
	return e
}

func (t tx) InsertSession(ctx context.Context, project, agent, principal, branch, revision string) (memory.Row, error) {
	return t.one(ctx, "INSERT INTO sessions(project_id,agent_id,principal_id,branch,git_revision) VALUES($1,$2,$3,$4,$5) RETURNING to_jsonb(sessions)", project, agent, principal, branch, revision)
}

func (t tx) AdvanceSession(ctx context.Context, project, session string, expected int, revision, branch string, finish bool) (memory.Row, error) {
	return t.one(ctx, "UPDATE sessions SET git_revision=$4,branch=CASE WHEN $5='' THEN branch ELSE $5 END,revision=revision+1,finished_at=CASE WHEN $6 THEN now() ELSE NULL END WHERE project_id=$1 AND id=$2 AND revision=$3 AND finished_at IS NULL RETURNING to_jsonb(sessions)", project, session, expected, revision, branch, finish)
}

func (t tx) ReleaseTask(ctx context.Context, project, task string, expected int, principal string) (memory.Row, error) {
	return t.one(ctx, "UPDATE tasks SET claimed_by=NULL,claim_expires_at=NULL,revision=revision+1,updated_at=now() WHERE project_id=$1 AND id=$2 AND revision=$3 AND claimed_by=$4 RETURNING (to_jsonb(tasks)"+searchCols+")", project, task, expected, principal)
}

func (t tx) EvidenceSatisfies(ctx context.Context, project, task, revision string, r memory.Requirement) (bool, error) {
	return t.exists(ctx, "SELECT EXISTS(SELECT 1 FROM evidence e WHERE e.project_id=$1 AND e.task_id=$2 AND e.git_revision=$3 AND e.outcome='pass' AND e.source_kind=ANY($4) AND (NOT $5 OR EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id)))", project, task, revision, r.Kinds, r.Attested)
}

func (t tx) DependenciesOpen(ctx context.Context, project, task string) (bool, error) {
	return t.exists(ctx, "SELECT EXISTS(SELECT 1 FROM task_dependencies d JOIN tasks t ON t.id=d.depends_on WHERE d.project_id=$1 AND d.task_id=$2 AND t.state NOT IN ('verified','released'))", project, task)
}

func (t tx) UpdateTask(ctx context.Context, project, task string, expected int, state, revision string, claim bool, principal string) (memory.Row, error) {
	return t.one(ctx, "UPDATE tasks SET state=$4,git_revision=CASE WHEN $5='' THEN git_revision ELSE $5 END,claimed_by=CASE WHEN $4='planned' THEN NULL WHEN $6 THEN $7 ELSE claimed_by END,claim_expires_at=CASE WHEN $4='planned' THEN NULL WHEN $6 THEN now()+interval '30 minutes' ELSE claim_expires_at END,revision=revision+1,updated_at=now() WHERE project_id=$1 AND id=$2 AND revision=$3 AND (claimed_by IS NULL OR claimed_by=$7 OR ($6 AND claim_expires_at<now())) RETURNING (to_jsonb(tasks)"+searchCols+")", project, task, expected, state, revision, claim, principal)
}

func (t tx) RecordIs(ctx context.Context, project, id, kind string) (bool, error) {
	return t.exists(ctx, "SELECT EXISTS(SELECT 1 FROM records WHERE project_id=$1 AND id=$2 AND kind=$3)", project, id, kind)
}

func (t tx) InsertRecord(ctx context.Context, r memory.RecordInput) (memory.Row, error) {
	return t.one(ctx, "INSERT INTO records(project_id,task_id,session_id,kind,title,body,git_revision,principal_id,agent_id,evidence_id,supersedes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING (to_jsonb(records)"+searchCols+")", r.Project, nullable(r.Task), nullable(r.Session), r.Kind, r.Title, r.Body, r.Revision, r.Principal, r.Agent, nullable(r.Evidence), nullable(r.Supersedes))
}

func (t tx) InsertEvidence(ctx context.Context, e memory.EvidenceInput) (memory.Row, error) {
	return t.one(ctx, "INSERT INTO evidence(project_id,task_id,source_kind,locator,git_revision,artifact_digest,outcome,summary,reported_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING (to_jsonb(evidence)"+searchCols+")", e.Project, nullable(e.Task), e.SourceKind, e.Locator, e.Revision, e.Digest, e.Outcome, e.Summary, e.ReportedBy)
}

func (t tx) InsertAttestation(ctx context.Context, project, evidence, principal, statement string) (memory.Row, error) {
	return t.one(ctx, "INSERT INTO attestations(project_id,evidence_id,principal_id,statement) VALUES($1,$2,$3,$4) RETURNING to_jsonb(attestations)", project, evidence, principal, statement)
}

func (t tx) InsertEvent(ctx context.Context, e memory.EventInput) error {
	_, err := t.Exec(ctx, "INSERT INTO events(project_id,task_id,session_id,kind,body,principal_id,agent_id,git_revision,details) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", e.Project, nullable(e.Task), nullable(e.Session), e.Kind, e.Body, e.Principal, e.Agent, e.Revision, e.Details)
	return err
}

func (t tx) InsertOperation(ctx context.Context, project, operation, principal, hash string, response []byte) error {
	_, e := t.Exec(ctx, "INSERT INTO operations(project_id,operation_id,principal_id,payload_hash,response) VALUES($1,$2,$3,$4,$5)", project, operation, principal, hash, response)
	return e
}
