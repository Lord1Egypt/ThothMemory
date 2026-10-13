// Package contract is the behaviour every ThothMemory backend must share.
// Full (PostgreSQL) and Mini (MariaDB) run the same scenarios; backend-only
// manipulations that agents can never perform are supplied as Admin hooks.
package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Lord1Egypt/ThothMemory/internal/gateway"
	"github.com/Lord1Egypt/ThothMemory/internal/mcptools"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Admin performs out-of-band manipulations used only to set up adversarial
// conditions. They run with database owner rights.
type Admin interface {
	ExpireClaim(ctx context.Context, task string) error
	ExhaustBudget(ctx context.Context, project string) error
	// MutateHistory attempts an UPDATE and a DELETE on table rows of project.
	MutateHistory(ctx context.Context, table, project string) error
	// LockProject holds the project row lock until release is called.
	LockProject(ctx context.Context, project string) (release func(), err error)
	CountOperations(ctx context.Context, project, operation string) (int, error)
}

type Harness struct {
	Backend memory.Backend
	Admin   Admin
}

func (h Harness) call(t *testing.T, p memory.Principal, tool string, a memory.Input) any {
	t.Helper()
	v, e := memory.Call(context.Background(), h.Backend, p, tool, a)
	if e != nil {
		t.Fatalf("%s: %v", tool, e)
	}
	return v
}

func (h Harness) fails(t *testing.T, p memory.Principal, tool string, a memory.Input) error {
	t.Helper()
	_, e := memory.Call(context.Background(), h.Backend, p, tool, a)
	if e == nil {
		t.Fatalf("%s unexpectedly succeeded", tool)
	}
	return e
}

func ID(v any) string { return v.(map[string]any)["id"].(string) }

var (
	Agent  = memory.Principal{ID: "contract-agent", Token: strings.Repeat("a", 64), Write: true, Projects: []string{"*"}}
	Reader = memory.Principal{ID: "contract-reader", Token: strings.Repeat("b", 64), Projects: []string{"*"}}
	Owner  = memory.Principal{ID: "contract-owner", Owner: true, Write: true, Projects: []string{"*"}}
)

func (h Harness) project(t *testing.T, label string) string {
	t.Helper()
	return ID(h.call(t, Agent, "create_project", memory.Input{Name: "CONTRACT " + label, Repository: fmt.Sprintf("https://github.com/thoth-contract/%s-%d", label, time.Now().UnixNano())}))
}

type authTransport struct{ token string }

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(r)
}

func Client(t *testing.T, url, token, name string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := mcp.NewClient(&mcp.Implementation{Name: name, Version: "contract"}, nil)
	cs, e := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url + "/mcp", HTTPClient: &http.Client{Transport: authTransport{token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// Invoke calls an MCP tool. With fail set it requires a tool error.
func Invoke(t *testing.T, c *mcp.ClientSession, name string, a map[string]any, fail bool) any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, e := c.CallTool(ctx, &mcp.CallToolParams{Name: "memory_" + name, Arguments: a})
	if e != nil {
		if fail {
			return nil
		}
		t.Fatal(e)
	}
	if fail {
		if !r.IsError {
			t.Fatalf("%s unexpectedly succeeded", name)
		}
		return nil
	}
	if r.IsError {
		t.Fatalf("%s failed: %+v", name, r.Content[0])
	}
	var out struct {
		Data any `json:"data"`
	}
	if e = json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &out); e != nil {
		t.Fatal(e)
	}
	return out.Data
}

// Run executes every shared scenario against one backend.
func Run(t *testing.T, h Harness) {
	ctx := context.Background()
	for _, p := range []memory.Principal{Agent, Reader, Owner} {
		if e := h.Backend.Register(ctx, p); e != nil {
			t.Fatal(e)
		}
	}
	srv := httptest.NewServer(gateway.Handler(h.Backend, []memory.Principal{Agent, Reader}))
	t.Cleanup(srv.Close)
	for _, sc := range []struct {
		name string
		fn   func(*testing.T, Harness, string)
	}{
		{"MCPHandoffAndEvidenceGates", mcpHandoff},
		{"ConcurrencyIsolationAndHistory", concurrency},
		{"ClaimsBudgetAndInterruptedWrites", claimsAndBudget},
		{"SearchMultilingualAndIdentifiers", search},
		{"CheckpointRecoveryAndUnicode", recovery},
		{"DependenciesAndSupersession", dependencies},
		{"IdeasNotesDuplicatesAndSecrets", ideasAndSecrets},
		{"DashboardOverviewAndExplorer", dashboard},
		{"HTTPBoundary", httpBoundary},
	} {
		t.Run(sc.name, func(t *testing.T) { sc.fn(t, h, srv.URL) })
	}
}

func mcpHandoff(t *testing.T, h Harness, url string) {
	head := strings.Repeat("a", 40)
	claude := Client(t, url, Agent.Token, "claude-code")
	codex := Client(t, url, Agent.Token, "codex")
	open := Client(t, url, Agent.Token, "opencode")
	tools, e := claude.ListTools(context.Background(), nil)
	if e != nil || len(tools.Tools) != len(mcptools.Contracts) {
		t.Fatalf("discovery: %v, %d tools", e, len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.Name == "memory_attest_evidence" {
			t.Fatal("owner tool exposed over MCP")
		}
	}
	project := ID(Invoke(t, claude, "create_project", map[string]any{"name": "Contract handoff", "repository": fmt.Sprintf("git@github.com:thoth-contract/handoff-%d.git", time.Now().UnixNano())}, false))
	task := ID(Invoke(t, claude, "create_task", map[string]any{"project_id": project, "operation_id": "create-task-001", "title": "Implement durable cross-agent context", "git_revision": head}, false))
	sess := ID(Invoke(t, claude, "start_session", map[string]any{"project_id": project, "operation_id": "start-claude-001", "agent": "claude-code", "git_revision": head, "branch": "main"}, false))
	Invoke(t, claude, "record_decision", map[string]any{"project_id": project, "operation_id": "decision-001", "title": "Use full-text search", "body": "Indexed search preserves provenance.", "git_revision": head, "session_id": sess, "agent": "claude-code"}, false)
	evidence := ID(Invoke(t, claude, "record_test_result", map[string]any{"project_id": project, "task_id": task, "operation_id": "test-result-001", "locator": "ci://test/123", "git_revision": head, "outcome": "pass", "body": "Reported test output; not independently attested."}, false))
	Invoke(t, claude, "finish_session", map[string]any{"project_id": project, "session_id": sess, "operation_id": "finish-claude-001", "expected_revision": 1, "git_revision": head, "body": "Implemented checkpoint. Review test evidence, then resume task.", "agent": "claude-code"}, false)
	Invoke(t, claude, "checkpoint_session", map[string]any{"project_id": project, "session_id": sess, "operation_id": "after-finish-001", "expected_revision": 2, "git_revision": head, "body": "late"}, true)
	brief := Invoke(t, codex, "get_project_context", map[string]any{"project_id": project, "git_revision": head}, false).(map[string]any)
	if len(brief["handoff"].([]any)) != 1 || brief["content_trust"] == nil {
		t.Fatal("handoff or trust marker missing", brief)
	}
	Invoke(t, codex, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "codex-verified-001", "expected_revision": 1, "state": "verified", "git_revision": head}, true)
	Invoke(t, codex, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "codex-tested-001", "expected_revision": 1, "state": "tested", "git_revision": head}, false)
	Invoke(t, open, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "open-stale-001", "expected_revision": 1, "state": "implemented"}, true)
	h.call(t, Owner, "attest_evidence", memory.Input{ProjectID: project, OperationID: "owner-attest-001", EvidenceID: evidence, Body: "Contract owner reviewed evidence"})
	if e := h.fails(t, Agent, "attest_evidence", memory.Input{ProjectID: project, OperationID: "agent-attest-001", EvidenceID: evidence, Body: "self attest"}); !errors.Is(e, memory.ErrForbidden) {
		t.Fatal("agent attestation not forbidden:", e)
	}
	Invoke(t, open, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "open-wrong-rev-001", "expected_revision": 2, "state": "verified", "git_revision": strings.Repeat("b", 40)}, true)
	args := map[string]any{"project_id": project, "task_id": task, "operation_id": "open-verified-001", "expected_revision": 2, "state": "verified", "git_revision": head}
	first := Invoke(t, open, "update_task", args, false)
	replay := Invoke(t, open, "update_task", args, false)
	if fmt.Sprint(first) != fmt.Sprint(replay) {
		t.Fatal("idempotent replay returned a different response")
	}
	args["state"] = "blocked"
	Invoke(t, open, "update_task", args, true)
	upper := strings.ToUpper(task)
	updated := Invoke(t, claude, "get_task", map[string]any{"project_id": strings.ToUpper(project), "task_id": upper}, false).([]any)
	if len(updated) != 1 || updated[0].(map[string]any)["state"] != "verified" || updated[0].(map[string]any)["revision"] != float64(3) {
		t.Fatal("task state or case-insensitive lookup", updated)
	}
	found := Invoke(t, open, "search", map[string]any{"project_id": project, "query": "search provenance", "git_revision": strings.Repeat("b", 40)}, false).([]any)
	if len(found) < 1 || found[0].(map[string]any)["stale"] != true || found[0].(map[string]any)["provenance"].(map[string]any)["trust"] != "reported" {
		t.Fatal("search stale/provenance", found)
	}
	Invoke(t, open, "append_event", map[string]any{"project_id": project, "operation_id": "invalid-schema-001", "body": "x", "sql": "DROP TABLE tasks"}, true)
	Invoke(t, open, "list_tasks", map[string]any{"project_id": project, "limit": 101}, true)
	Invoke(t, open, "list_tasks", map[string]any{"project_id": project, "state": "invalid"}, true)
	for _, agent := range []string{"codex", "opencode"} {
		c := Client(t, url, Agent.Token, agent)
		sid := ID(Invoke(t, c, "start_session", map[string]any{"project_id": project, "operation_id": "handoff-start-" + agent, "agent": agent, "git_revision": head}, false))
		Invoke(t, c, "finish_session", map[string]any{"project_id": project, "session_id": sid, "operation_id": "handoff-finish-" + agent, "expected_revision": 1, "git_revision": head, "agent": agent, "body": agent + " retrieved and updated shared context."}, false)
	}
	latest := Invoke(t, claude, "get_latest_handoff", map[string]any{"project_id": project}, false).([]any)
	if latest[0].(map[string]any)["agent_id"] != "opencode" {
		t.Fatal("latest handoff", latest)
	}
	activity := Invoke(t, claude, "get_recent_activity", map[string]any{"project_id": project, "limit": 100}, false).([]any)
	if len(activity) < 10 {
		t.Fatal("activity timeline incomplete", len(activity))
	}
	reader := Client(t, url, Reader.Token, "reader")
	rt, _ := reader.ListTools(context.Background(), nil)
	for _, tool := range rt.Tools {
		if memory.IsWrite(strings.TrimPrefix(tool.Name, "memory_")) {
			t.Fatal("read credential discovered write tool", tool.Name)
		}
	}
}

func concurrency(t *testing.T, h Harness, url string) {
	ctx := context.Background()
	project, other := h.project(t, "race"), h.project(t, "other")
	task := ID(h.call(t, Agent, "create_task", memory.Input{ProjectID: project, OperationID: "race-create-001", Title: "Concurrent task"}))
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := memory.Call(ctx, h.Backend, Agent, "update_task", memory.Input{ProjectID: project, OperationID: fmt.Sprintf("race-update-%d", i), TaskID: task, ExpectedRevision: 1, State: "implemented"})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful conflicting writers = %d", success)
	}
	// Same operation, different payload: rejected; same payload concurrently: one result.
	h.call(t, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "idem-payload-001", Body: "first"})
	if e := h.fails(t, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "idem-payload-001", Body: "changed"}); !errors.Is(e, memory.ErrConflict) {
		t.Fatal("changed payload not a conflict:", e)
	}
	if _, e := memory.Call(ctx, h.Backend, Agent, "attach_evidence", memory.Input{ProjectID: other, TaskID: task, OperationID: "cross-project-001", SourceKind: "test", Locator: "log:test", GitRevision: strings.Repeat("a", 40), Outcome: "pass", Body: "cross project"}); e == nil {
		t.Fatal("cross-project evidence accepted")
	}
	if n, e := h.Admin.CountOperations(ctx, other, "cross-project-001"); e != nil || n != 0 {
		t.Fatal("failed write left an operation", n, e)
	}
	h.call(t, Agent, "record_observation", memory.Input{ProjectID: project, OperationID: "history-record-001", Title: "History", Body: "immutable"})
	ev := ID(h.call(t, Agent, "attach_evidence", memory.Input{ProjectID: project, OperationID: "history-evidence-001", SourceKind: "log", Locator: "log:x", GitRevision: strings.Repeat("c", 40), Outcome: "unknown", Body: "evidence"}))
	h.call(t, Owner, "attest_evidence", memory.Input{ProjectID: project, OperationID: "history-attest-001", EvidenceID: ev, Body: "owner"})
	for _, table := range []string{"events", "records", "evidence", "attestations", "operations"} {
		if e := h.Admin.MutateHistory(ctx, table, project); e == nil {
			t.Errorf("history table %s accepted UPDATE or DELETE", table)
		}
	}
	restricted := Agent
	restricted.ID, restricted.Projects = "contract-restricted", []string{other}
	if e := h.Backend.Register(ctx, restricted); e != nil {
		t.Fatal(e)
	}
	if e := h.fails(t, restricted, "get_task", memory.Input{ProjectID: project, TaskID: task}); !errors.Is(e, memory.ErrForbidden) {
		t.Fatal("project authorization bypass:", e)
	}
	h.fails(t, restricted, "create_project", memory.Input{Name: "forbidden", Repository: "https://github.com/thoth-contract/forbidden"})
	for _, c := range mcptools.Contracts {
		if c.Name == "list_projects" || c.Name == "get_health" || c.Name == "create_project" {
			continue
		}
		if _, e := memory.Call(ctx, h.Backend, restricted, c.Name, memory.Input{ProjectID: project}); !errors.Is(e, memory.ErrForbidden) {
			t.Errorf("%s: scoped credential not forbidden: %v", c.Name, e)
		}
	}
	for view := range memory.DashboardViews {
		if view == "overview" {
			continue
		}
		if _, e := memory.Call(ctx, h.Backend, restricted, view, memory.Input{ProjectID: project, TaskID: task}); !errors.Is(e, memory.ErrForbidden) {
			t.Errorf("%s: dashboard view not forbidden: %v", view, e)
		}
	}
	rows := h.call(t, restricted, "overview", memory.Input{}).([]memory.Row)
	for _, r := range rows {
		if r["id"] != other {
			t.Fatal("overview leaked an unauthorized project")
		}
	}
	if e := h.fails(t, Reader, "create_task", memory.Input{ProjectID: project, OperationID: "reader-write-001", Title: "no"}); !errors.Is(e, memory.ErrForbidden) {
		t.Fatal("read-only credential wrote:", e)
	}
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := memory.Call(dead, h.Backend, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "cancelled-001", Body: "should fail"}); e == nil {
		t.Fatal("cancelled write succeeded")
	}
	if n, _ := h.Admin.CountOperations(ctx, project, "cancelled-001"); n != 0 {
		t.Fatal("cancelled write persisted")
	}
}

func claimsAndBudget(t *testing.T, h Harness, url string) {
	ctx := context.Background()
	project := h.project(t, "claims")
	task := ID(h.call(t, Agent, "create_task", memory.Input{ProjectID: project, OperationID: "claim-task-create", Title: "Claim task"}))
	h.call(t, Agent, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-first-agent", ExpectedRevision: 1})
	other := memory.Principal{ID: "contract-claim-other", Write: true, Projects: []string{project}}
	if e := h.Backend.Register(ctx, other); e != nil {
		t.Fatal(e)
	}
	h.fails(t, other, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-second-agent", ExpectedRevision: 2})
	h.fails(t, other, "release_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "release-other-agent", ExpectedRevision: 2})
	h.call(t, Agent, "release_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "release-first-agent", ExpectedRevision: 2})
	h.call(t, other, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-after-release", ExpectedRevision: 3})
	h.fails(t, Agent, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-steal-active", ExpectedRevision: 4})
	if e := h.Admin.ExpireClaim(ctx, task); e != nil {
		t.Fatal(e)
	}
	v := h.call(t, Agent, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-after-expiry", ExpectedRevision: 4}).(memory.Row)
	if v["claimed_by"] != Agent.ID || v["claim_expires_at"] == nil {
		t.Fatal("expired claim not reclaimed", v)
	}
	// Interrupt a write blocked behind another transaction: nothing persists.
	release, e := h.Admin.LockProject(ctx, project)
	if e != nil {
		t.Fatal(e)
	}
	interrupted, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, e = memory.Call(interrupted, h.Backend, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "interrupted-in-flight", Body: "must roll back"})
	cancel()
	release()
	if e == nil {
		t.Fatal("interrupted write accepted")
	}
	if n, _ := h.Admin.CountOperations(ctx, project, "interrupted-in-flight"); n != 0 {
		t.Fatal("partial write persisted")
	}
	h.call(t, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "after-interruption", Body: "service still writable"})
	if e = h.Admin.ExhaustBudget(ctx, project); e != nil {
		t.Fatal(e)
	}
	h.fails(t, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "budget-reject-001", Body: "over quota"})
	if n, _ := h.Admin.CountOperations(ctx, project, "budget-reject-001"); n != 0 {
		t.Fatal("budget failure left an operation")
	}
	// A replay of a completed operation consumes no budget.
	h.call(t, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "after-interruption", Body: "service still writable"})
}

func search(t *testing.T, h Harness, url string) {
	c := Client(t, url, Agent.Token, "simulated-claude")
	project := h.project(t, "search")
	head := strings.Repeat("c", 40)
	fixtures := []struct{ body, query string }{{"Running migrations safely", "migrate"}, {"المشكلة اتحلت لما غيرنا إعدادات الاتصال", "اتحلت"}, {"فشل التحقق بسبب تعارض المراجعات", "تعارض"}, {"PostgreSQL rollback مع اختبار الاتصال", "PostgreSQL اختبار"}, {"HandleCheckpointHTTP internal/store/operations.go", "Checkpoint"}, {"إعدادات الاتصال", "إعداد"}, {"café checkpoint", "café"}, {"commit bound record", head[:12]}, {"ERROR duplicate key value violates unique constraint SQLSTATE 23505", "23505"}, {"literal 100%_done marker", "100%_done"}}
	ids := map[string]string{}
	for i, f := range fixtures {
		ids[f.body] = ID(Invoke(t, c, "record_observation", map[string]any{"project_id": project, "operation_id": fmt.Sprintf("search-fixture-%d", i), "title": "Fixture", "body": f.body, "git_revision": head}, false))
	}
	for _, f := range fixtures {
		rows := Invoke(t, c, "search", map[string]any{"project_id": project, "query": f.query}, false).([]any)
		found := false
		for _, r := range rows {
			if r.(map[string]any)["id"] == ids[f.body] {
				found = true
			}
		}
		if !found {
			t.Errorf("query %q did not retrieve %q", f.query, f.body)
		}
	}
	// The observation and its mirrored event both contain "100%"; nothing else may match.
	for _, r := range Invoke(t, c, "search", map[string]any{"project_id": project, "query": "100%"}, false).([]any) {
		if !strings.Contains(r.(map[string]any)["body"].(string), "100%") {
			t.Errorf("wildcard was not escaped: %q matched a literal percent", r.(map[string]any)["body"])
		}
	}
	if rows := Invoke(t, c, "search", map[string]any{"project_id": project, "query": "nonexistent-token-zz"}, false).([]any); len(rows) != 0 {
		t.Error("search matched unrelated content")
	}
	exact := Invoke(t, c, "search", map[string]any{"project_id": project, "exact_id": ids[fixtures[1].body]}, false).([]any)
	if len(exact) != 1 || exact[0].(map[string]any)["body"] != fixtures[1].body {
		t.Fatal("exact id search", exact)
	}
	after := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if rows := Invoke(t, c, "search", map[string]any{"project_id": project, "query": "Fixture", "after": after}, false).([]any); len(rows) != 0 {
		t.Error("time filter ignored")
	}
	paged := Invoke(t, c, "search", map[string]any{"project_id": project, "query": "Fixture", "limit": 3, "offset": 3}, false).([]any)
	if len(paged) != 3 {
		t.Error("pagination", len(paged))
	}
	kinds := map[string]bool{}
	for _, r := range Invoke(t, c, "search", map[string]any{"project_id": project, "limit": 100}, false).([]any) {
		kinds[r.(map[string]any)["kind"].(string)] = true
	}
	if !kinds["observation"] || !kinds["event"] {
		t.Error("search omitted records or events", kinds)
	}
}

func recovery(t *testing.T, h Harness, url string) {
	c := Client(t, url, Agent.Token, "simulated-claude")
	project := h.project(t, "recovery")
	head := strings.Repeat("c", 40)
	sid := ID(Invoke(t, c, "start_session", map[string]any{"project_id": project, "operation_id": "crash-session-start", "agent": "simulated-claude", "git_revision": head}, false))
	Invoke(t, c, "checkpoint_session", map[string]any{"project_id": project, "operation_id": "crash-checkpoint-001", "session_id": sid, "expected_revision": 1, "git_revision": head, "body": "Known failed attempt: old migration command fails; use fixed command next."}, false)
	c.Close()
	next := Client(t, url, Agent.Token, "simulated-codex")
	brief := Invoke(t, next, "get_project_context", map[string]any{"project_id": project, "git_revision": strings.Repeat("d", 40)}, false).(map[string]any)
	b, _ := json.Marshal(brief)
	if !strings.Contains(string(b), "old migration command fails") {
		t.Error("recovery briefing hides durable checkpoint")
	}
	sessions := brief["sessions"].([]any)
	if len(sessions) != 1 || sessions[0].(map[string]any)["finished_at"] != nil || sessions[0].(map[string]any)["status"] != "incomplete" {
		t.Fatal("crashed session falsely completed", sessions)
	}
	if brief["checkpoints"].([]any)[0].(map[string]any)["stale"] != true {
		t.Error("checkpoint at another revision not marked stale")
	}
	text := "x" + strings.Repeat("ع", 800)
	h.call(t, Agent, "record_issue", memory.Input{ProjectID: project, OperationID: "unicode-boundary-issue", Title: "Unicode", Body: text})
	ctxv := h.call(t, Agent, "get_project_context", memory.Input{ProjectID: project}).(map[string]any)
	body := ctxv["decisions_and_issues"].([]memory.Row)[0]["body"].(string)
	if !utf8.ValidString(body) || !strings.HasPrefix(body, "x"+strings.Repeat("ع", 599)) {
		t.Fatal("briefing corrupted Unicode boundary")
	}
}

func dependencies(t *testing.T, h Harness, url string) {
	project := h.project(t, "deps")
	head := strings.Repeat("d", 40)
	base := ID(h.call(t, Agent, "create_task", memory.Input{ProjectID: project, OperationID: "dep-base-create", Title: "Base"}))
	child := ID(h.call(t, Agent, "create_task", memory.Input{ProjectID: project, OperationID: "dep-child-create", Title: "Child", DependsOn: []string{base}}))
	h.fails(t, Agent, "update_task", memory.Input{ProjectID: project, OperationID: "dep-child-early", TaskID: child, ExpectedRevision: 1, State: "implemented"})
	h.call(t, Agent, "update_task", memory.Input{ProjectID: project, OperationID: "dep-child-blocked", TaskID: child, ExpectedRevision: 1, State: "blocked"})
	ev := ID(h.call(t, Agent, "record_test_result", memory.Input{ProjectID: project, OperationID: "dep-base-test", TaskID: base, Locator: "ci://base", GitRevision: head, Outcome: "pass", Body: "pass"}))
	h.call(t, Owner, "attest_evidence", memory.Input{ProjectID: project, OperationID: "dep-base-attest", EvidenceID: ev, Body: "reviewed"})
	h.call(t, Agent, "update_task", memory.Input{ProjectID: project, OperationID: "dep-base-verified", TaskID: base, ExpectedRevision: 1, State: "verified", GitRevision: head})
	h.call(t, Agent, "update_task", memory.Input{ProjectID: project, OperationID: "dep-child-now", TaskID: child, ExpectedRevision: 2, State: "implemented"})
	h.fails(t, Agent, "update_task", memory.Input{ProjectID: project, OperationID: "dep-released-no-evidence", TaskID: base, ExpectedRevision: 2, State: "released", GitRevision: head})
	h.fails(t, Agent, "update_task", memory.Input{ProjectID: project, OperationID: "dep-committed-no-rev", TaskID: child, ExpectedRevision: 3, State: "committed"})
	first := ID(h.call(t, Agent, "record_decision", memory.Input{ProjectID: project, OperationID: "decision-v1", Title: "Store", Body: "Use files"}))
	second := ID(h.call(t, Agent, "record_decision", memory.Input{ProjectID: project, OperationID: "decision-v2", Title: "Store", Body: "Use a database", Supersedes: first}))
	h.fails(t, Agent, "record_decision", memory.Input{ProjectID: project, OperationID: "decision-v2-fork", Title: "Store", Body: "Fork", Supersedes: first})
	h.fails(t, Agent, "record_issue", memory.Input{ProjectID: project, OperationID: "issue-supersedes-decision", Title: "x", Body: "x", Supersedes: second})
	current := h.call(t, Agent, "get_project_context", memory.Input{ProjectID: project}).(map[string]any)["decisions_and_issues"].([]memory.Row)
	if len(current) != 1 || current[0]["id"] != second {
		t.Fatal("superseded decision still current", current)
	}
	rows := h.call(t, Agent, "search", memory.Input{ProjectID: project, ExactID: first}).([]memory.Row)
	if len(rows) != 1 || rows[0]["superseded"] != true {
		t.Fatal("superseded record lost or unflagged", rows)
	}
}

func dashboard(t *testing.T, h Harness, url string) {
	project := h.project(t, "dashboard")
	head := strings.Repeat("e", 40)
	task := ID(h.call(t, Agent, "create_task", memory.Input{ProjectID: project, OperationID: "dash-task", Title: "Dashboard task", GitRevision: head}))
	h.call(t, Agent, "update_task", memory.Input{ProjectID: project, OperationID: "dash-block", TaskID: task, ExpectedRevision: 1, State: "blocked"})
	sid := ID(h.call(t, Agent, "start_session", memory.Input{ProjectID: project, OperationID: "dash-session", Agent: "codex", GitRevision: head, Branch: "feature/x"}))
	h.call(t, Agent, "checkpoint_session", memory.Input{ProjectID: project, OperationID: "dash-checkpoint", SessionID: sid, ExpectedRevision: 1, GitRevision: head, Body: "checkpoint text"})
	h.call(t, Agent, "record_test_result", memory.Input{ProjectID: project, OperationID: "dash-evidence", TaskID: task, Locator: "ci://dash", GitRevision: head, Outcome: "pass", Body: "passing"})
	h.call(t, Agent, "record_issue", memory.Input{ProjectID: project, OperationID: "dash-issue", Title: "Open issue", Body: "details"})
	var row memory.Row
	for _, r := range h.call(t, Agent, "overview", memory.Input{}).([]memory.Row) {
		if r["id"] == project {
			row = r
		}
	}
	if row == nil {
		t.Fatal("overview omitted project")
	}
	want := map[string]float64{"incomplete_sessions": 1, "open_issues": 1, "evidence_total": 1, "evidence_awaiting_attestation": 1, "evidence_attested": 0}
	for k, v := range want {
		if row[k] != v {
			t.Errorf("overview %s = %v, want %v", k, row[k], v)
		}
	}
	if row["task_counts"].(memory.Row)["blocked"] != float64(1) {
		t.Error("task counts", row["task_counts"])
	}
	if obs := row["last_observed"].(memory.Row); obs["git_revision"] != head || obs["source"] != "event" {
		t.Error("last observed revision", obs)
	}
	if ls := row["last_session"].(memory.Row); ls["branch"] != "feature/x" || ls["status"] != "incomplete" {
		t.Error("last session", ls)
	}
	if row["latest_checkpoint"] == nil || row["latest_evidence"] == nil || row["last_activity_at"] == nil {
		t.Error("overview latest records missing")
	}
	if a, _ := row["next_recommended_action"].(string); !strings.Contains(a, "unfinished session") {
		t.Error("next action", a)
	}
	for _, v := range []struct {
		tool, kind string
		min        int
	}{{"explore_sessions", "incomplete", 1}, {"explore_records", "issue", 1}, {"explore_evidence", "awaiting_attestation", 1}, {"explore_events", "", 5}, {"explore_imports", "", 0}} {
		rows := h.call(t, Agent, v.tool, memory.Input{ProjectID: project, Kind: v.kind}).([]memory.Row)
		if len(rows) < v.min {
			t.Errorf("%s(%s) = %d rows", v.tool, v.kind, len(rows))
		}
	}
	h.fails(t, Agent, "explore_records", memory.Input{ProjectID: project, Kind: "secret"})
	detail := h.call(t, Agent, "explore_task", memory.Input{ProjectID: project, TaskID: task}).([]memory.Row)
	sections := map[string]int{}
	for _, r := range detail {
		sections[r["section"].(string)]++
		if r["section"] == "transition" && r["kind"] == "update_task" && r["state"] != "blocked" {
			t.Error("transition state not decoded", r)
		}
	}
	if sections["task"] != 1 || sections["evidence"] != 1 || sections["transition"] < 2 {
		t.Error("task detail sections", sections)
	}
}

func httpBoundary(t *testing.T, h Harness, url string) {
	for _, tc := range []struct {
		name, path, host, origin, token string
		status                          int
	}{{"unauthenticated", "/mcp", "", "", "", 401}, {"host", "/healthz", "attacker.example", "", "", 403}, {"origin", "/healthz", "", "http://attacker.example", "", 403}, {"ready", "/healthz", "", "", "", 200}, {"dashboard-write", "/api/read?tool=append_event&project_id=x", "", "", Agent.Token, 403}, {"dashboard-unknown", "/api/read?tool=drop_everything", "", "", Agent.Token, 403}, {"dashboard-anonymous", "/api/read?tool=overview", "", "", "", 401}, {"dashboard-overview", "/api/read?tool=overview", "", "", Agent.Token, 200}, {"dashboard-bad-limit", "/api/read?tool=overview&limit=x", "", "", Agent.Token, 400}, {"info", "/api/info", "", "", "", 200}} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", url+tc.path, nil)
			if tc.host != "" {
				r.Host = tc.host
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			res, e := http.DefaultClient.Do(r)
			if e != nil {
				t.Fatal(e)
			}
			res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatalf("got %d want %d", res.StatusCode, tc.status)
			}
			if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
				t.Fatal("missing CSP")
			}
		})
	}
}

// Shape reduces a response to its sorted key structure so two backends can be
// compared without their generated identifiers and timestamps.
func Shape(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k+":"+Shape(x[k]))
		}
		sort.Strings(keys)
		return "{" + strings.Join(keys, ",") + "}"
	case []any:
		// The set of distinct element shapes, independent of ranking order.
		seen := map[string]bool{}
		for _, e := range x {
			seen[Shape(e)] = true
		}
		shapes := make([]string, 0, len(seen))
		for k := range seen {
			shapes = append(shapes, k)
		}
		sort.Strings(shapes)
		return "[" + strings.Join(shapes, "|") + "]"
	case nil:
		return "null"
	case string:
		return "s"
	case float64:
		return "n"
	case bool:
		return "b"
	}
	return fmt.Sprintf("%T", v)
}

// Shapes runs one deterministic scenario and returns the response shape of
// every read tool and dashboard view, for cross-edition comparison.
func Shapes(t *testing.T, h Harness) map[string]string {
	ctx := context.Background()
	for _, p := range []memory.Principal{Agent, Owner} {
		if e := h.Backend.Register(ctx, p); e != nil {
			t.Fatal(e)
		}
	}
	project := h.project(t, "shapes")
	head := strings.Repeat("f", 40)
	task := ID(h.call(t, Agent, "create_task", memory.Input{ProjectID: project, OperationID: "shape-task", Title: "Shape task", GitRevision: head}))
	sid := ID(h.call(t, Agent, "start_session", memory.Input{ProjectID: project, OperationID: "shape-session", Agent: "codex", GitRevision: head, Branch: "main"}))
	h.call(t, Agent, "checkpoint_session", memory.Input{ProjectID: project, OperationID: "shape-checkpoint", SessionID: sid, ExpectedRevision: 1, GitRevision: head, Body: "checkpoint"})
	ev := ID(h.call(t, Agent, "record_test_result", memory.Input{ProjectID: project, OperationID: "shape-evidence", TaskID: task, Locator: "ci://shape", GitRevision: head, Outcome: "pass", Body: "pass"}))
	h.call(t, Owner, "attest_evidence", memory.Input{ProjectID: project, OperationID: "shape-attest", EvidenceID: ev, Body: "ok"})
	h.call(t, Agent, "record_decision", memory.Input{ProjectID: project, OperationID: "shape-decision", Title: "Decision", Body: "body", TaskID: task, SessionID: sid, EvidenceID: ev})
	h.call(t, Agent, "finish_session", memory.Input{ProjectID: project, OperationID: "shape-finish", SessionID: sid, ExpectedRevision: 2, GitRevision: head, Body: "handoff"})
	out := map[string]string{}
	for _, tool := range []string{"list_projects", "get_task", "list_tasks", "get_recent_activity", "get_latest_handoff", "search", "get_project_context", "overview", "explore_sessions", "explore_records", "explore_evidence", "explore_events", "explore_task"} {
		a := memory.Input{ProjectID: project, TaskID: task, GitRevision: head}
		switch tool {
		case "list_projects", "overview":
			a = memory.Input{}
		case "get_recent_activity", "explore_events", "explore_sessions", "explore_records", "explore_evidence":
			a.TaskID = ""
		case "search":
			a = memory.Input{ProjectID: project, Query: "Decision", GitRevision: head}
		}
		v := h.call(t, Agent, tool, a)
		if tool == "list_projects" || tool == "overview" {
			for _, r := range v.([]memory.Row) {
				if r["id"] == project {
					v = []memory.Row{r}
				}
			}
		}
		if tool == "explore_task" {
			bySection := map[string]any{}
			for _, r := range v.([]memory.Row) {
				bySection[r["section"].(string)] = r
			}
			v = bySection
		}
		// Round-trip through JSON exactly as MCP clients receive it.
		b, _ := json.Marshal(v)
		var decoded any
		_ = json.Unmarshal(b, &decoded)
		out[tool] = Shape(decoded)
	}
	return out
}

func ideasAndSecrets(t *testing.T, h Harness, url string) {
	ctx := context.Background()
	project := h.project(t, "ideas")
	claude := Client(t, url, Agent.Token, "claude-code")
	// A small idea becomes durable memory, labelled as a proposal.
	idea := Invoke(t, claude, "record_idea", map[string]any{"project_id": project, "operation_id": "idea-pocketclaw-1", "title": "SQLite edition for PocketClaw", "body": "Maybe we can make a SQLite version for PocketClaw.", "agent": "claude-code"}, false).(map[string]any)
	if idea["kind"] != "idea" || idea["duplicate"] != nil {
		t.Fatal("idea not recorded as kind idea", idea)
	}
	// The same words again, even under a new operation ID, add no history.
	events := len(h.call(t, Agent, "get_recent_activity", memory.Input{ProjectID: project, Limit: 100}).([]memory.Row))
	again := Invoke(t, claude, "record_idea", map[string]any{"project_id": project, "operation_id": "idea-pocketclaw-2", "title": "SQLite edition for PocketClaw", "body": "Maybe we can make a SQLite version for PocketClaw."}, false).(map[string]any)
	if again["id"] != idea["id"] || again["duplicate"] != true {
		t.Fatal("duplicate idea created a new record", again)
	}
	if after := len(h.call(t, Agent, "get_recent_activity", memory.Input{ProjectID: project, Limit: 100}).([]memory.Row)); after != events {
		t.Fatal("duplicate idea added history", events, after)
	}
	// Replaying the duplicate's operation ID returns the same answer.
	replay := Invoke(t, claude, "record_idea", map[string]any{"project_id": project, "operation_id": "idea-pocketclaw-2", "title": "SQLite edition for PocketClaw", "body": "Maybe we can make a SQLite version for PocketClaw."}, false).(map[string]any)
	if replay["id"] != idea["id"] {
		t.Fatal("duplicate replay differs")
	}
	note := Invoke(t, claude, "record_note", map[string]any{"project_id": project, "operation_id": "note-arabic-1", "title": "User preference", "body": "المستخدم يفضل الواجهة العربية في لوحة التحكم"}, false).(map[string]any)
	// Ideas and notes cannot masquerade as evidence or be verified.
	Invoke(t, claude, "record_idea", map[string]any{"project_id": project, "operation_id": "idea-evidence-1", "title": "x", "body": "x", "evidence_id": "00000000-0000-4000-8000-000000000000"}, true)
	// A different agent finds it later by searching, and in the briefing.
	codex := Client(t, url, Agent.Token, "codex")
	found := Invoke(t, codex, "search", map[string]any{"project_id": project, "query": "PocketClaw"}, false).([]any)
	hit := false
	for _, r := range found {
		hit = hit || r.(map[string]any)["id"] == idea["id"]
	}
	if !hit {
		t.Fatal("idea not found by search", found)
	}
	brief := Invoke(t, codex, "get_project_context", map[string]any{"project_id": project}, false).(map[string]any)
	if len(brief["ideas_and_notes"].([]any)) != 2 {
		t.Fatal("briefing omits ideas and notes", brief["ideas_and_notes"])
	}
	// Adopting the idea records a decision that supersedes it; the idea is kept.
	decision := Invoke(t, codex, "record_decision", map[string]any{"project_id": project, "operation_id": "decide-pocketclaw", "title": "Build UltraLite", "body": "Adopt the SQLite idea as UltraLite.", "supersedes": idea["id"]}, false).(map[string]any)
	if decision["supersedes"] != idea["id"] {
		t.Fatal("decision did not supersede the idea", decision)
	}
	brief = Invoke(t, codex, "get_project_context", map[string]any{"project_id": project}, false).(map[string]any)
	if len(brief["ideas_and_notes"].([]any)) != 1 || brief["ideas_and_notes"].([]any)[0].(map[string]any)["id"] != note["id"] {
		t.Fatal("adopted idea still listed as open", brief["ideas_and_notes"])
	}
	old := Invoke(t, codex, "search", map[string]any{"project_id": project, "exact_id": idea["id"]}, false).([]any)
	if len(old) != 1 || old[0].(map[string]any)["superseded"] != true {
		t.Fatal("adopted idea lost or unflagged", old)
	}
	// A note cannot supersede a decision, nor a decision supersede a note.
	Invoke(t, codex, "record_note", map[string]any{"project_id": project, "operation_id": "note-supersedes-decision", "title": "x", "body": "y", "supersedes": decision["id"]}, true)
	Invoke(t, codex, "record_decision", map[string]any{"project_id": project, "operation_id": "decision-supersedes-note", "title": "x", "body": "y", "supersedes": note["id"]}, true)
	// Credential-looking content is refused and leaves nothing behind.
	for i, secret := range []string{
		"-----BEGIN OPENSSH " + "PRIVATE KEY-----\nabc",
		"my key is AKIA" + "IOSFODNN7EXAMPLE ok",
		"token ghp_" + strings.Repeat("a1B2", 10),
		"pass" + "word: " + strings.Repeat("hunter2", 4),
		"Authorization: Bearer " + strings.Repeat("Ab3", 12),
		"sk-ant-" + strings.Repeat("x9", 20),
	} {
		op := fmt.Sprintf("secret-attempt-%d", i)
		e := h.fails(t, Agent, "record_note", memory.Input{ProjectID: project, OperationID: op, Title: "note", Body: secret})
		if !errors.Is(e, memory.ErrSecret) {
			t.Errorf("secret %d not refused: %v", i, e)
		}
		if n, _ := h.Admin.CountOperations(ctx, project, op); n != 0 {
			t.Errorf("secret %d left an operation row", i)
		}
	}
	h.fails(t, Agent, "append_event", memory.Input{ProjectID: project, OperationID: "secret-event-1", Body: "AKIA" + "IOSFODNN7EXAMPLE"})
	// Ordinary technical prose is not mistaken for a secret.
	h.call(t, Agent, "record_note", memory.Input{ProjectID: project, OperationID: "benign-note-1", Title: "Config", Body: "token_file points at .secrets/agent_token; the commit is " + strings.Repeat("a", 40) + " and sha " + strings.Repeat("b", 64)})
	if v := h.call(t, Agent, "explore_records", memory.Input{ProjectID: project, Kind: "idea"}).([]memory.Row); len(v) != 1 {
		t.Fatal("explore by kind idea", len(v))
	}
}
