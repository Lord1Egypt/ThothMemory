package mariadb

import (
	"context"
	"errors"
	"strings"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
)

// Column lists mirror PostgreSQL to_jsonb(row) minus search columns, so both
// editions return the same keys for each entity.
const (
	projectCols     = "id,name,repository,created_at,event_count,event_budget"
	taskCols        = "id,project_id,title,state,revision,git_revision,claimed_by,updated_at,claim_expires_at"
	sessionCols     = "id,project_id,agent_id,principal_id,branch,git_revision,started_at,finished_at,revision"
	recordCols      = "id,project_id,task_id,session_id,kind,title,body,git_revision,principal_id,agent_id,evidence_id,supersedes,created_at"
	evidenceCols    = "id,project_id,task_id,source_kind,locator,git_revision,artifact_digest,outcome,summary,reported_by,created_at"
	eventCols       = "id,project_id,task_id,session_id,kind,body,principal_id,agent_id,git_revision,created_at"
	attestationCols = "id,project_id,evidence_id,principal_id,statement,created_at"
	importCols      = "id,project_id,export_id,source_edition,source_version,format_version,checksum,counts,principal_id,imported_at"
	// Names sort with Unicode collation, approximating PostgreSQL's locale order.
	byName = " ORDER BY name COLLATE utf8mb4_uca1400_ai_ci,id"
	// Evidence an owner could attest to unlock verified or released states.
	awaitingAttestation = "e.outcome='pass' AND e.source_kind IN ('test','ci','release') AND NOT EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id)"
)

// prefixed qualifies a column list with a table alias.
func prefixed(alias, cols string) string {
	return alias + "." + strings.ReplaceAll(cols, ",", ","+alias+".")
}

func projectFilter(all bool, ids []string) (string, []any) {
	if all {
		return "TRUE", nil
	}
	if len(ids) == 0 {
		return "FALSE", nil
	}
	return "id IN (" + in(len(ids)) + ")", anyArgs(ids)
}

func (s *Store) ListProjects(ctx context.Context, all bool, ids []string, limit, offset int) ([]memory.Row, error) {
	where, args := projectFilter(all, ids)
	return rows(ctx, s.DB, "SELECT "+projectCols+" FROM projects WHERE "+where+byName+" LIMIT ? OFFSET ?", append(args, limit, offset)...)
}

func (s *Store) CreateProject(ctx context.Context, name, repo string) (memory.Row, error) {
	// The repository key is the natural idempotency key; an existing project is returned unchanged.
	_, e := s.DB.ExecContext(ctx, "INSERT INTO projects(id,name,repository) VALUES(?,?,?)", newID(), name, repo)
	if e = ignoreDuplicate(e); e != nil {
		return nil, e
	}
	return one(ctx, s.DB, "SELECT "+projectCols+" FROM projects WHERE repository_key=UNHEX(SHA2(?,256))", repo)
}

func (s *Store) Tasks(ctx context.Context, project, task, state string, limit, offset int) ([]memory.Row, error) {
	return rows(ctx, s.DB, "SELECT "+taskCols+" FROM tasks WHERE project_id=? AND (?='' OR id=?) AND (?='' OR state=?) ORDER BY updated_at DESC,id LIMIT ? OFFSET ?", project, task, task, state, state, limit, offset)
}

func (s *Store) Activity(ctx context.Context, project, task string, limit, offset int) ([]memory.Row, error) {
	return s.events(ctx, project, task, "", limit, offset)
}

func (s *Store) events(ctx context.Context, project, task, kind string, limit, offset int) ([]memory.Row, error) {
	return rows(ctx, s.DB, "SELECT "+eventCols+" FROM events WHERE project_id=? AND (?='' OR task_id=?) AND (?='' OR kind=?) ORDER BY created_at DESC,id LIMIT ? OFFSET ?", project, task, task, kind, kind, limit, offset)
}

func (s *Store) LatestHandoff(ctx context.Context, project string) ([]memory.Row, error) {
	return rows(ctx, s.DB, "SELECT "+recordCols+" FROM records WHERE project_id=? AND kind='handoff' ORDER BY created_at DESC,id LIMIT 1", project)
}

// Search matches the NFC/lowercase literal phrase, or every query term as a
// substring, across records, events, evidence and tasks of one project. Each
// branch uses its project index; text matching is a bounded per-project scan.
func (s *Store) Search(ctx context.Context, project string, q memory.SearchQuery) ([]memory.Row, error) {
	union := "SELECT " + recordCols + ",search_text AS st FROM records WHERE project_id=?" +
		" UNION ALL SELECT id,project_id,task_id,session_id,'event',kind,body,git_revision,principal_id,agent_id,NULL,NULL,created_at,search_text FROM events WHERE project_id=?" +
		" UNION ALL SELECT id,project_id,task_id,NULL,'evidence',locator,summary,git_revision,reported_by,'',id,NULL,created_at,search_text FROM evidence WHERE project_id=?" +
		" UNION ALL SELECT id,project_id,id,NULL,'task',title,state,git_revision,COALESCE(claimed_by,''),'',NULL,NULL,updated_at,CONCAT(search_text,' ',git_revision) FROM tasks WHERE project_id=?"
	args := []any{q.CurrentRevision, q.CurrentRevision}
	rank := "0"
	where := []string{"TRUE"}
	var matchArgs []any
	if phrase, terms := searchTerms(q.Text); phrase != "" {
		// Deterministic: every match scores 1, the whole phrase adds 1, and the
		// phrase in a record or task title adds 1 (an event's title is its tool name).
		rank = "1+(u.st LIKE ?)+(u.kind<>'event' AND LOWER(u.title) LIKE ?)"
		args = append(args, likePattern(phrase), likePattern(phrase))
		all := make([]string, len(terms))
		for i, t := range terms {
			all[i] = "u.st LIKE ?"
			matchArgs = append(matchArgs, likePattern(t))
		}
		where = append(where, "(u.st LIKE ? OR ("+strings.Join(all, " AND ")+"))")
		matchArgs = append([]any{likePattern(phrase)}, matchArgs...)
	}
	args = append(args, project, project, project, project)
	args = append(args, matchArgs...)
	for _, f := range []struct{ col, v string }{{"task_id", q.TaskID}, {"agent_id", q.Agent}, {"evidence_id", q.EvidenceID}, {"id", q.ExactID}} {
		if f.v != "" {
			where = append(where, "u."+f.col+"=?")
			args = append(args, f.v)
		}
	}
	if q.After != nil {
		where = append(where, "u.created_at>=?")
		args = append(args, q.After.UTC())
	}
	if q.Before != nil {
		where = append(where, "u.created_at<=?")
		args = append(args, q.Before.UTC())
	}
	args = append(args, q.Limit, q.Offset)
	out, e := rows(ctx, s.DB, "SELECT "+prefixed("u", recordCols)+",EXISTS(SELECT 1 FROM records n WHERE n.supersedes=u.id) AS superseded,CASE WHEN ?='' THEN NULL ELSE u.git_revision<>? END AS stale,"+rank+" AS `rank` FROM ("+union+") u WHERE "+strings.Join(where, " AND ")+" ORDER BY `rank` DESC,u.created_at DESC,u.id LIMIT ? OFFSET ?", args...)
	for _, r := range out {
		r["provenance"] = memory.Row{"principal": r["principal_id"], "agent": r["agent_id"], "evidence_id": r["evidence_id"], "trust": "reported"}
	}
	return out, e
}

func (s *Store) Context(ctx context.Context, project string) (map[string][]memory.Row, error) {
	queries := map[string]string{
		"project":              "SELECT " + projectCols + " FROM projects WHERE id=?",
		"tasks":                "SELECT " + taskCols + " FROM tasks WHERE project_id=? ORDER BY (state IN ('active','blocked')) DESC,updated_at DESC,id LIMIT 12",
		"sessions":             "SELECT " + sessionCols + ",CASE WHEN finished_at IS NULL THEN 'incomplete' ELSE 'finished' END AS status FROM sessions WHERE project_id=? ORDER BY started_at DESC,id LIMIT 4",
		"checkpoints":          "SELECT " + recordCols + " FROM records WHERE project_id=? AND session_id IS NOT NULL AND kind='observation' ORDER BY created_at DESC,id LIMIT 4",
		"handoff":              "SELECT " + recordCols + " FROM records WHERE project_id=? AND kind='handoff' ORDER BY created_at DESC,id LIMIT 1",
		"decisions_and_issues": "SELECT " + prefixed("r", recordCols) + " FROM records r WHERE r.project_id=? AND r.kind IN ('decision','issue') AND NOT EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id) ORDER BY r.created_at DESC,r.id LIMIT 8",
		"evidence":             "SELECT " + prefixed("e", evidenceCols) + ",EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id) AS owner_attested FROM evidence e WHERE e.project_id=? ORDER BY e.created_at DESC,e.id LIMIT 8",
	}
	out := map[string][]memory.Row{}
	for _, name := range memory.ContextSections {
		v, e := rows(ctx, s.DB, queries[name], project)
		if e != nil {
			return nil, e
		}
		out[name] = v
	}
	return out, nil
}

// Overview selects per-project facts in one query, then loads the referenced
// latest rows in four batched lookups instead of per-project round trips.
func (s *Store) Overview(ctx context.Context, all bool, ids []string) ([]memory.Row, error) {
	where, args := projectFilter(all, ids)
	latest := func(table, cond, order string) string {
		return "(SELECT x.id FROM " + table + " x WHERE x.project_id=p.id" + cond + " ORDER BY x." + order + " DESC,x.id DESC LIMIT 1)"
	}
	out, e := rows(ctx, s.DB, "SELECT "+prefixed("p", projectCols)+
		",(SELECT MAX(x.created_at) FROM events x WHERE x.project_id=p.id) AS last_activity_at"+
		","+latest("events", " AND x.git_revision<>''", "created_at")+" AS observed_ref"+
		","+latest("sessions", "", "started_at")+" AS session_ref"+
		","+latest("records", " AND x.kind='handoff'", "created_at")+" AS handoff_ref"+
		","+latest("records", " AND x.kind='observation' AND x.session_id IS NOT NULL", "created_at")+" AS checkpoint_ref"+
		","+latest("evidence", "", "created_at")+" AS evidence_ref"+
		",(SELECT COUNT(*) FROM sessions x WHERE x.project_id=p.id AND x.finished_at IS NULL) AS incomplete_sessions"+
		",(SELECT COUNT(*) FROM records r WHERE r.project_id=p.id AND r.kind='issue' AND NOT EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id)) AS open_issues"+
		",(SELECT COUNT(*) FROM records r WHERE r.project_id=p.id AND r.kind='decision' AND NOT EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id)) AS current_decisions"+
		",(SELECT COUNT(*) FROM evidence x WHERE x.project_id=p.id) AS evidence_total"+
		",(SELECT COUNT(*) FROM attestations x WHERE x.project_id=p.id) AS evidence_attested"+
		",(SELECT COUNT(*) FROM evidence e WHERE e.project_id=p.id AND "+awaitingAttestation+") AS evidence_awaiting_attestation"+
		" FROM projects p WHERE "+strings.ReplaceAll(where, "id IN", "p.id IN")+" ORDER BY p.name COLLATE utf8mb4_uca1400_ai_ci,p.id LIMIT 200", args...)
	if e != nil || len(out) == 0 {
		return out, e
	}
	refs := map[string][]string{}
	projects := make([]string, len(out))
	for i, r := range out {
		projects[i] = r["id"].(string)
		for _, k := range []string{"observed_ref", "session_ref", "handoff_ref", "checkpoint_ref", "evidence_ref"} {
			if id, ok := r[k].(string); ok {
				refs[k] = append(refs[k], id)
			}
		}
	}
	byID := map[string]memory.Row{}
	lookups := map[string]string{
		"observed_ref":   "SELECT id,git_revision,created_at AS observed_at,agent_id,principal_id,'event' AS source,id AS source_id FROM events WHERE id IN (%s)",
		"session_ref":    "SELECT id,agent_id,principal_id,branch,git_revision,started_at,finished_at,CASE WHEN finished_at IS NULL THEN 'incomplete' ELSE 'finished' END AS status FROM sessions WHERE id IN (%s)",
		"handoff_ref":    "SELECT id,created_at,agent_id,principal_id,git_revision,LEFT(body,280) AS excerpt FROM records WHERE id IN (%s)",
		"checkpoint_ref": "SELECT id,created_at,agent_id,session_id,git_revision,LEFT(body,280) AS excerpt FROM records WHERE id IN (%s)",
		"evidence_ref":   "SELECT e.id,e.created_at,e.git_revision,e.source_kind,e.outcome,EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id) AS owner_attested FROM evidence e WHERE e.id IN (%s)",
	}
	for k, list := range refs {
		found, e := rows(ctx, s.DB, strings.Replace(lookups[k], "%s", in(len(list)), 1), anyArgs(list)...)
		if e != nil {
			return nil, e
		}
		for _, r := range found {
			byID[k+r["id"].(string)] = r
			if k == "observed_ref" {
				delete(r, "id") // PostgreSQL reports the event as source_id only
			}
		}
	}
	counts, e := rows(ctx, s.DB, "SELECT project_id,state,COUNT(*) AS n FROM tasks WHERE project_id IN ("+in(len(projects))+") GROUP BY project_id,state", anyArgs(projects)...)
	if e != nil {
		return nil, e
	}
	taskCounts := map[string]memory.Row{}
	for _, c := range counts {
		p := c["project_id"].(string)
		if taskCounts[p] == nil {
			taskCounts[p] = memory.Row{}
		}
		taskCounts[p][c["state"].(string)] = c["n"]
	}
	names := map[string]string{"observed_ref": "last_observed", "session_ref": "last_session", "handoff_ref": "latest_handoff", "checkpoint_ref": "latest_checkpoint", "evidence_ref": "latest_evidence"}
	for _, r := range out {
		for ref, name := range names {
			id, _ := r[ref].(string)
			if found, ok := byID[ref+id]; ok {
				r[name] = found
			} else {
				r[name] = nil
			}
			delete(r, ref)
		}
		if tc := taskCounts[r["id"].(string)]; tc != nil {
			r["task_counts"] = tc
		} else {
			r["task_counts"] = memory.Row{}
		}
	}
	return out, nil
}

func (s *Store) Explore(ctx context.Context, project string, q memory.ExploreQuery) ([]memory.Row, error) {
	switch q.View {
	case "sessions":
		return rows(ctx, s.DB, "SELECT "+prefixed("s", sessionCols)+",CASE WHEN s.finished_at IS NULL THEN 'incomplete' ELSE 'finished' END AS status"+
			",(SELECT COUNT(*) FROM records r WHERE r.project_id=s.project_id AND r.session_id=s.id) AS checkpoints"+
			",(SELECT MAX(r.created_at) FROM records r WHERE r.project_id=s.project_id AND r.session_id=s.id) AS last_checkpoint_at"+
			" FROM sessions s WHERE s.project_id=? AND (?='' OR (?='incomplete')=(s.finished_at IS NULL)) ORDER BY s.started_at DESC,s.id LIMIT ? OFFSET ?", project, q.Kind, q.Kind, q.Limit, q.Offset)
	case "records":
		return rows(ctx, s.DB, "SELECT "+prefixed("r", recordCols)+",(SELECT n.id FROM records n WHERE n.supersedes=r.id) AS superseded_by,EXISTS(SELECT 1 FROM records n WHERE n.supersedes=r.id) AS superseded"+
			" FROM records r WHERE r.project_id=? AND (?='' OR r.kind=?) AND (?='' OR r.task_id=?) ORDER BY r.created_at DESC,r.id LIMIT ? OFFSET ?", project, q.Kind, q.Kind, q.TaskID, q.TaskID, q.Limit, q.Offset)
	case "evidence":
		out, e := rows(ctx, s.DB, "SELECT "+prefixed("e", evidenceCols)+",a.id IS NOT NULL AS owner_attested,a.id AS a_id,a.principal_id AS a_principal_id,a.statement AS a_statement,a.created_at AS a_created_at"+
			" FROM evidence e LEFT JOIN attestations a ON a.evidence_id=e.id WHERE e.project_id=? AND (?='' OR e.task_id=?)"+
			" AND (?='' OR (?='attested' AND a.id IS NOT NULL) OR (?='awaiting_attestation' AND "+awaitingAttestation+")) ORDER BY e.created_at DESC,e.id LIMIT ? OFFSET ?", project, q.TaskID, q.TaskID, q.Kind, q.Kind, q.Kind, q.Limit, q.Offset)
		for _, r := range out {
			if r["a_id"] != nil {
				r["attestation"] = memory.Row{"id": r["a_id"], "principal_id": r["a_principal_id"], "statement": r["a_statement"], "created_at": r["a_created_at"]}
			} else {
				r["attestation"] = nil
			}
			for _, k := range []string{"a_id", "a_principal_id", "a_statement", "a_created_at"} {
				delete(r, k)
			}
		}
		return out, e
	case "events":
		return s.events(ctx, project, q.TaskID, q.Kind, q.Limit, q.Offset)
	case "imports":
		return rows(ctx, s.DB, "SELECT "+importCols+" FROM imports WHERE project_id=? ORDER BY imported_at DESC,id LIMIT ? OFFSET ?", project, q.Limit, q.Offset)
	case "task":
		sections := []struct{ name, sql string }{
			{"task", "SELECT " + taskCols + " FROM tasks WHERE project_id=? AND id=?"},
			{"dependency", "SELECT t.id,t.title,t.state,t.revision,t.git_revision FROM task_dependencies d JOIN tasks t ON t.project_id=d.project_id AND t.id=d.depends_on WHERE d.project_id=? AND d.task_id=? ORDER BY t.title,t.id LIMIT 50"},
			{"dependent", "SELECT t.id,t.title,t.state,t.revision,t.git_revision FROM task_dependencies d JOIN tasks t ON t.project_id=d.project_id AND t.id=d.task_id WHERE d.project_id=? AND d.depends_on=? ORDER BY t.title,t.id LIMIT 50"},
			{"evidence", "SELECT " + prefixed("e", evidenceCols) + ",EXISTS(SELECT 1 FROM attestations x WHERE x.evidence_id=e.id) AS owner_attested FROM evidence e WHERE e.project_id=? AND e.task_id=? ORDER BY e.created_at DESC,e.id LIMIT 50"},
			{"transition", "SELECT id,kind,body,principal_id,agent_id,git_revision,created_at,details FROM events WHERE project_id=? AND task_id=? ORDER BY created_at DESC,id LIMIT 100"},
		}
		out := []memory.Row{}
		for _, sec := range sections {
			v, e := rows(ctx, s.DB, sec.sql, project, q.TaskID)
			if e != nil {
				return nil, e
			}
			for _, r := range v {
				r["section"] = sec.name
			}
			out = append(out, v...)
		}
		return out, nil
	}
	return nil, errors.New("unknown view")
}
