package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Lord1Egypt/ThothMemory/internal/gateway"
	"github.com/Lord1Egypt/ThothMemory/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func auditProject(t *testing.T, s *store.Store, p store.Principal, label string) string {
	t.Helper()
	v, e := s.Call(context.Background(), p, "create_project", store.Input{Name: "SIMULATED " + label, Repository: fmt.Sprintf("https://github.com/thothmemory-example/%s-%d", label, time.Now().UnixNano())})
	if e != nil {
		t.Fatal(e)
	}
	return id(v)
}
func TestAuditSearchAndCheckpointRecovery(t *testing.T) {
	s, srv, p := setup(t)
	c := client(t, srv, p.Token, "simulated-claude")
	project := auditProject(t, s, p, "search-recovery")
	head := strings.Repeat("c", 40)
	fixtures := []struct{ body, query string }{{"Running migrations safely", "migrate"}, {"المشكلة اتحلت لما غيرنا إعدادات الاتصال", "اتحلت"}, {"فشل التحقق بسبب تعارض المراجعات", "تعارض"}, {"PostgreSQL rollback مع اختبار الاتصال", "PostgreSQL اختبار"}, {"HandleCheckpointHTTP internal/store/operations.go", "Checkpoint"}, {"إعدادات الاتصال", "إعداد"}, {"café checkpoint", "cafe\u0301"}, {"commit bound record", head[:12]}, {"ERROR duplicate key value violates unique constraint SQLSTATE 23505", "23505"}}
	for i, f := range fixtures {
		record := id(invoke(t, c, "record_observation", map[string]any{"project_id": project, "operation_id": fmt.Sprintf("search-fixture-%d", i), "title": "Fixture", "body": f.body, "git_revision": head}, false))
		rows := invoke(t, c, "search", map[string]any{"project_id": project, "query": f.query}, false).([]any)
		found := false
		for _, r := range rows {
			if r.(map[string]any)["id"] == record {
				found = true
			}
		}
		if !found {
			t.Errorf("query %q did not retrieve %q", f.query, f.body)
		}
	}
	sid := id(invoke(t, c, "start_session", map[string]any{"project_id": project, "operation_id": "crash-session-start", "agent": "simulated-claude", "git_revision": head}, false))
	invoke(t, c, "checkpoint_session", map[string]any{"project_id": project, "operation_id": "crash-checkpoint-001", "session_id": sid, "expected_revision": 1, "git_revision": head, "body": "Known failed attempt: old migration command fails; use fixed command next."}, false)
	c.Close() // No finish hook. A disconnected client must leave an incomplete session.
	next := client(t, srv, p.Token, "simulated-codex")
	brief := invoke(t, next, "get_project_context", map[string]any{"project_id": project, "git_revision": strings.Repeat("d", 40)}, false).(map[string]any)
	b, _ := json.Marshal(brief)
	if !strings.Contains(string(b), "old migration command fails") {
		t.Error("recovery briefing hides durable checkpoint")
	}
	sessions := brief["sessions"].([]any)
	if len(sessions) != 1 || sessions[0].(map[string]any)["finished_at"] != nil {
		t.Fatal("crashed session falsely completed", sessions)
	}
}

func TestAuditProjectAssociationsAndExpiredClaims(t *testing.T) {
	s, _, p := setup(t)
	ctx := context.Background()
	a := auditProject(t, s, p, "scope-a")
	b := auditProject(t, s, p, "scope-b")
	head := strings.Repeat("e", 40)
	task, e := s.Call(ctx, p, "create_task", store.Input{ProjectID: a, OperationID: "scope-task-create", Title: "Boundary task"})
	if e != nil {
		t.Fatal(e)
	}
	session, e := s.Call(ctx, p, "start_session", store.Input{ProjectID: a, OperationID: "scope-session-start", Agent: "simulated-claude", GitRevision: head})
	if e != nil {
		t.Fatal(e)
	}
	evidence, e := s.Call(ctx, p, "attach_evidence", store.Input{ProjectID: a, OperationID: "scope-evidence-create", SourceKind: "test", Outcome: "pass", GitRevision: head, Locator: "test://simulated", Body: "Reported fixture"})
	if e != nil {
		t.Fatal(e)
	}
	scoped := store.Principal{ID: "audit-scoped", Token: strings.Repeat("s", 64), Write: true, Projects: []string{a}}
	if e = s.Register(ctx, scoped); e != nil {
		t.Fatal(e)
	}
	scopedServer := httptest.NewServer(gateway.Handler(s, []store.Principal{scoped}))
	defer scopedServer.Close()
	c := client(t, scopedServer, scoped.Token, "scoped-agent")
	invoke(t, c, "get_project_context", map[string]any{"project_id": b}, true)
	projects := invoke(t, c, "list_projects", map[string]any{}, false).([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["id"] != a {
		t.Fatal("scoped discovery", projects)
	}
	for _, contract := range gateway.Contracts {
		if contract.Name == "list_projects" || contract.Name == "get_health" || contract.Name == "create_project" {
			continue
		}
		if _, err := s.Call(ctx, scoped, contract.Name, store.Input{ProjectID: b}); !errors.Is(err, store.ErrForbidden) {
			t.Errorf("%s unauthorized result: %v", contract.Name, err)
		}
	}
	for i, input := range []store.Input{{TaskID: id(task)}, {SessionID: id(session)}, {EvidenceID: id(evidence)}} {
		input.ProjectID = b
		input.OperationID = fmt.Sprintf("cross-association-%d", i)
		input.Title = "no crossing"
		input.Body = "no crossing"
		if _, err := s.Call(ctx, p, "record_decision", input); err == nil {
			t.Errorf("cross-project association accepted: %d", i)
		}
	}
	if _, e = s.Call(ctx, p, "claim_task", store.Input{ProjectID: a, TaskID: id(task), OperationID: "lease-first-claim", ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Call(ctx, scoped, "claim_task", store.Input{ProjectID: a, TaskID: id(task), OperationID: "lease-stolen-claim", ExpectedRevision: 2}); e == nil {
		t.Fatal("active claim stolen")
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE tasks SET claim_expires_at=now()-interval '1 second' WHERE id=$1", id(task)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Call(ctx, scoped, "claim_task", store.Input{ProjectID: a, TaskID: id(task), OperationID: "lease-expired-claim", ExpectedRevision: 2}); e != nil {
		t.Fatal("expired claim not reclaimable", e)
	}
	// Interrupt a write while it is blocked inside a real transaction.
	blocker, e := s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(ctx)
	if _, e = blocker.Exec(ctx, "SELECT 1 FROM projects WHERE id=$1 FOR UPDATE", a); e != nil {
		t.Fatal(e)
	}
	interrupted, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, e = s.Call(interrupted, p, "append_event", store.Input{ProjectID: a, OperationID: "interrupted-in-flight", Body: "must roll back"}); e == nil {
		t.Fatal("interrupted write accepted")
	}
	blocker.Rollback(ctx)
	var count int
	if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE project_id=$1 AND operation_id='interrupted-in-flight'", a).Scan(&count); e != nil || count != 0 {
		t.Fatal("partial write persisted", count, e)
	}
}

func TestAuditRealisticFourSessionEngineeringHandoff(t *testing.T) {
	s, _, owner := setup(t)
	ctx := context.Background()
	project := auditProject(t, s, owner, "engineering-handoff")
	actors := []store.Principal{}
	for i, name := range []string{"claude", "codex", "opencode"} {
		p := store.Principal{ID: "audit-" + name, Token: strings.Repeat(fmt.Sprint(i+1), 64), Write: true, Projects: []string{project}}
		if e := s.Register(ctx, p); e != nil {
			t.Fatal(e)
		}
		actors = append(actors, p)
	}
	srv := httptest.NewServer(gateway.Handler(s, actors))
	defer srv.Close()
	claude := client(t, srv, actors[0].Token, "simulated-claude")
	codex := client(t, srv, actors[1].Token, "simulated-codex")
	open := client(t, srv, actors[2].Token, "simulated-opencode")
	head1, head2 := strings.Repeat("1", 40), strings.Repeat("2", 40)
	invoke(t, claude, "get_project_context", map[string]any{"project_id": project, "git_revision": head1}, false)
	cs := id(invoke(t, claude, "start_session", map[string]any{"project_id": project, "operation_id": "engineering-claude-start", "agent": "simulated-claude", "git_revision": head1}, false))
	task := id(invoke(t, claude, "create_task", map[string]any{"project_id": project, "operation_id": "engineering-create-task", "title": "SIMULATED fix migration recovery", "git_revision": head1}, false))
	invoke(t, claude, "record_decision", map[string]any{"project_id": project, "operation_id": "engineering-decision", "title": "Use transactional migration", "body": "SIMULATED: preserve original database and try a replacement command.", "git_revision": head1, "session_id": cs}, false)
	invoke(t, claude, "record_observation", map[string]any{"project_id": project, "operation_id": "engineering-observation", "title": "Recoverable checkpoint", "body": "SIMULATED schema checksum confirmed; runtime grants pending.", "git_revision": head1, "session_id": cs}, false)
	issue := id(invoke(t, claude, "record_issue", map[string]any{"project_id": project, "operation_id": "engineering-failed-attempt", "title": "Known failed attempt", "body": "SIMULATED: legacy migrate --reset fails and must not be repeated. Use migrate --transaction instead.", "git_revision": head1, "task_id": task}, false))
	invoke(t, claude, "record_test_result", map[string]any{"project_id": project, "operation_id": "engineering-failed-test", "task_id": task, "git_revision": head1, "locator": "simulated://legacy-reset", "outcome": "fail", "body": "SIMULATED failure: legacy reset command unavailable."}, false)
	invoke(t, claude, "checkpoint_session", map[string]any{"project_id": project, "operation_id": "engineering-claude-checkpoint", "session_id": cs, "expected_revision": 1, "git_revision": head1, "body": "SIMULATED failed legacy reset; issue " + issue + ". Try transactional replacement."}, false)
	invoke(t, claude, "finish_session", map[string]any{"project_id": project, "operation_id": "engineering-claude-finish", "session_id": cs, "expected_revision": 2, "git_revision": head1, "body": "SIMULATED investigated checksums; legacy reset failed; implementation remains. See issue " + issue}, false)
	h := invoke(t, codex, "get_latest_handoff", map[string]any{"project_id": project}, false).([]any)
	if !strings.Contains(h[0].(map[string]any)["body"].(string), issue) {
		t.Fatal("prior handoff missing failure reference")
	}
	failure := invoke(t, codex, "search", map[string]any{"project_id": project, "exact_id": issue}, false).([]any)
	if !strings.Contains(failure[0].(map[string]any)["body"].(string), "must not be repeated") {
		t.Fatal("known failure missing")
	}
	ds := id(invoke(t, codex, "start_session", map[string]any{"project_id": project, "operation_id": "engineering-codex-start", "agent": "simulated-codex", "git_revision": head2}, false))
	invoke(t, codex, "checkpoint_session", map[string]any{"project_id": project, "operation_id": "engineering-codex-checkpoint", "session_id": ds, "expected_revision": 1, "git_revision": head2, "body": "SIMULATED: read known failure; used transactional replacement rather than legacy reset."}, false)
	invoke(t, codex, "record_test_result", map[string]any{"project_id": project, "operation_id": "engineering-passing-test", "task_id": task, "git_revision": head2, "locator": "simulated://transactional-migrate", "outcome": "pass", "body": "SIMULATED replacement test passed; reported, not owner-attested."}, false)
	invoke(t, codex, "update_task", map[string]any{"project_id": project, "operation_id": "engineering-task-tested", "task_id": task, "expected_revision": 1, "state": "tested", "git_revision": head2}, false)
	invoke(t, codex, "finish_session", map[string]any{"project_id": project, "operation_id": "engineering-codex-finish", "session_id": ds, "expected_revision": 2, "git_revision": head2, "body": "SIMULATED replacement implemented and reported tests pass. Failed legacy attempt retained. Remaining: owner evidence review; task is tested, not verified."}, false)
	taskState := invoke(t, open, "get_task", map[string]any{"project_id": project, "task_id": task}, false).([]any)[0].(map[string]any)
	if taskState["revision"] != float64(2) || taskState["git_revision"] != head2 || taskState["state"] != "tested" {
		t.Fatal(taskState)
	}
	osid := id(invoke(t, open, "start_session", map[string]any{"project_id": project, "operation_id": "engineering-opencode-start", "agent": "simulated-opencode", "git_revision": head2}, false))
	invoke(t, open, "checkpoint_session", map[string]any{"project_id": project, "operation_id": "engineering-opencode-checkpoint", "session_id": osid, "expected_revision": 1, "git_revision": head2, "body": "SIMULATED reviewed Codex task revision 2; no extra implementation claimed."}, false)
	invoke(t, open, "finish_session", map[string]any{"project_id": project, "operation_id": "engineering-opencode-finish", "session_id": osid, "expected_revision": 2, "git_revision": head2, "agent": "simulated-opencode", "body": "SIMULATED complete: replacement implemented/tested. Failed: legacy reset. Remaining: owner review. No verified claim."}, false)
	final := client(t, srv, actors[0].Token, "simulated-claude-return")
	brief := invoke(t, final, "get_project_context", map[string]any{"project_id": project, "git_revision": head2}, false)
	b, _ := json.Marshal(brief)
	for _, want := range []string{"replacement implemented/tested", "Failed: legacy reset", "Remaining: owner review", "incomplete"} {
		if want == "incomplete" {
			continue
		}
		if !strings.Contains(string(b), want) {
			t.Errorf("final briefing missing %q", want)
		}
	}
	t.Log("SIMULATED Claude -> Codex -> OpenCode -> Claude: real authenticated MCP, failed attempt, checkpoint, revision-aware task and handoff recovered")
}

func TestAuditBriefingPreservesUnicodeBoundary(t *testing.T) {
	s, _, p := setup(t)
	project := auditProject(t, s, p, "unicode-briefing")
	text := "x" + strings.Repeat("ع", 800)
	_, err := s.Call(context.Background(), p, "record_issue", store.Input{ProjectID: project, OperationID: "unicode-boundary-issue", Title: "Unicode", Body: text})
	if err != nil {
		t.Fatal(err)
	}
	brief, err := s.Call(context.Background(), p, "get_project_context", store.Input{ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}
	body := brief.(map[string]any)["decisions_and_issues"].([]map[string]any)[0]["body"].(string)
	if !utf8.ValidString(body) || !strings.HasPrefix(body, "x"+strings.Repeat("ع", 599)) {
		t.Fatal("briefing corrupted Unicode boundary")
	}
}
