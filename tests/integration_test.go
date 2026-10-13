package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Lord1Egypt/ThothMemory/internal/gateway"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type authTransport struct{ token string }

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(r)
}
func setup(t *testing.T) (*store.Store, *httptest.Server, memory.Principal) {
	t.Helper()
	dsn := os.Getenv("THOTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("THOTH_TEST_DATABASE_URL unset; PostgreSQL integration NOT executed")
	}
	ctx := context.Background()
	s, e := store.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Pool.Close)
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	p := memory.Principal{ID: "test-agent", Token: strings.Repeat("a", 64), Write: true, Projects: []string{"*"}}
	if e = s.Register(ctx, p); e != nil {
		t.Fatal(e)
	}
	reader := memory.Principal{ID: "test-reader", Token: strings.Repeat("b", 64), Projects: []string{"*"}}
	if e = s.Register(ctx, reader); e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(gateway.Handler(s, []memory.Principal{p, reader}))
	t.Cleanup(srv.Close)
	return s, srv, p
}
func client(t *testing.T, srv *httptest.Server, token, agent string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := mcp.NewClient(&mcp.Implementation{Name: agent, Version: "test"}, nil)
	cs, e := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: authTransport{token}}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}
func invoke(t *testing.T, c *mcp.ClientSession, name string, a map[string]any, fail bool) any {
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
			t.Fatalf("%s unexpectedly succeeded: %+v", name, r)
		}
		return nil
	}
	if r.IsError {
		t.Fatalf("%s failed: %+v", name, r.Content)
	}
	var out struct {
		Data any `json:"data"`
	}
	if e = json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &out); e != nil {
		t.Fatal(e)
	}
	return out.Data
}
func id(v any) string { return v.(map[string]any)["id"].(string) }
func TestRealMCPThreeAgentHandoff(t *testing.T) {
	s, srv, p := setup(t)
	head := strings.Repeat("a", 40)
	ctx := context.Background()
	claude := client(t, srv, p.Token, "claude-code")
	codex := client(t, srv, p.Token, "codex")
	open := client(t, srv, p.Token, "opencode")
	tools, e := claude.ListTools(ctx, nil)
	if e != nil || len(tools.Tools) != 23 {
		t.Fatalf("discovery: %v, count %d", e, len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.Name == "memory_attest_evidence" {
			t.Fatal("owner tool exposed")
		}
	}
	project := id(invoke(t, claude, "create_project", map[string]any{"name": "Test A", "repository": "git@github.com:thoth-tests/" + fmt.Sprint(time.Now().UnixNano()) + ".git"}, false))
	task := id(invoke(t, claude, "create_task", map[string]any{"project_id": project, "operation_id": "create-task-001", "title": "Implement durable cross-agent context", "git_revision": head}, false))
	sess := id(invoke(t, claude, "start_session", map[string]any{"project_id": project, "operation_id": "start-claude-001", "agent": "claude-code", "git_revision": head, "branch": "main"}, false))
	invoke(t, claude, "record_decision", map[string]any{"project_id": project, "operation_id": "decision-001", "title": "Use PostgreSQL full-text search", "body": "Indexed search preserves provenance.", "git_revision": head, "session_id": sess, "agent": "claude-code"}, false)
	invoke(t, claude, "append_event", map[string]any{"project_id": project, "operation_id": "progress-event-001", "body": "Checkpoint recorded before handoff", "task_id": task}, false)
	evidence := id(invoke(t, claude, "record_test_result", map[string]any{"project_id": project, "task_id": task, "operation_id": "test-result-001", "locator": "ci://test/123", "git_revision": head, "outcome": "pass", "body": "Reported test output; not independently attested."}, false))
	invoke(t, claude, "finish_session", map[string]any{"project_id": project, "session_id": sess, "operation_id": "finish-claude-001", "expected_revision": 1, "git_revision": head, "body": "Implemented checkpoint. Review test evidence, then resume task.", "agent": "claude-code"}, false)
	brief := invoke(t, codex, "get_project_context", map[string]any{"project_id": project, "git_revision": head}, false).(map[string]any)
	if len(brief["handoff"].([]any)) != 1 {
		t.Fatal("handoff missing")
	}
	invoke(t, codex, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "codex-verified-001", "expected_revision": 1, "state": "verified", "git_revision": head}, true)
	invoke(t, codex, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "codex-tested-001", "expected_revision": 1, "state": "tested", "git_revision": head}, false)
	invoke(t, open, "get_task", map[string]any{"project_id": project, "task_id": task}, false)
	invoke(t, open, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "open-stale-001", "expected_revision": 1, "state": "implemented"}, true)
	owner := memory.Principal{ID: "test-owner", Owner: true, Write: true, Projects: []string{"*"}}
	if e = s.Register(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Call(ctx, owner, "attest_evidence", memory.Input{ProjectID: project, OperationID: "owner-attest-001", EvidenceID: evidence, Body: "Test owner reviewed evidence"}); e != nil {
		t.Fatal(e)
	}
	invoke(t, open, "update_task", map[string]any{"project_id": project, "task_id": task, "operation_id": "open-wrong-rev-001", "expected_revision": 2, "state": "verified", "git_revision": strings.Repeat("b", 40)}, true)
	args := map[string]any{"project_id": project, "task_id": task, "operation_id": "open-verified-001", "expected_revision": 2, "state": "verified", "git_revision": head}
	invoke(t, open, "update_task", args, false)
	invoke(t, open, "update_task", args, false)
	args["state"] = "blocked"
	invoke(t, open, "update_task", args, true)
	updated := invoke(t, claude, "get_task", map[string]any{"project_id": project, "task_id": task}, false).([]any)[0].(map[string]any)
	if updated["state"] != "verified" || updated["revision"] != float64(3) {
		t.Fatal(updated)
	}
	found := invoke(t, open, "search", map[string]any{"project_id": project, "query": "PostgreSQL provenance", "git_revision": strings.Repeat("b", 40)}, false).([]any)
	if len(found) < 1 || found[0].(map[string]any)["stale"] != true {
		t.Fatal(found)
	}
	invoke(t, open, "append_event", map[string]any{"project_id": project, "operation_id": "invalid-schema-001", "body": "x", "sql": "DROP TABLE tasks"}, true)
	invoke(t, open, "list_tasks", map[string]any{"project_id": project, "limit": 101}, true)
	invoke(t, open, "list_tasks", map[string]any{"project_id": project, "state": "invalid"}, true)
	for _, agent := range []string{"codex", "opencode"} {
		c := client(t, srv, p.Token, agent)
		sid := id(invoke(t, c, "start_session", map[string]any{"project_id": project, "operation_id": "handoff-start-" + agent, "agent": agent, "git_revision": head}, false))
		invoke(t, c, "finish_session", map[string]any{"project_id": project, "session_id": sid, "operation_id": "handoff-finish-" + agent, "expected_revision": 1, "git_revision": head, "agent": agent, "body": agent + " retrieved and updated shared context; next agent review verified evidence."}, false)
	}
	h := invoke(t, claude, "get_latest_handoff", map[string]any{"project_id": project}, false).([]any)
	if h[0].(map[string]any)["agent_id"] != "opencode" {
		t.Fatal(h)
	}
	// Reconnect independently after prior writes.
	fresh := client(t, srv, p.Token, "claude-reconnected")
	invoke(t, fresh, "get_project_context", map[string]any{"project_id": project, "git_revision": head}, false)
}
func TestConcurrencyIsolationRollbackAndHistory(t *testing.T) {
	s, srv, p := setup(t)
	ctx := context.Background()
	create := func(name string) string {
		v, e := s.Call(ctx, p, "create_project", memory.Input{Name: name, Repository: "https://github.com/tests/" + name + fmt.Sprint(time.Now().UnixNano())})
		if e != nil {
			t.Fatal(e)
		}
		return id(v)
	}
	project := create("race")
	other := create("other")
	v, e := s.Call(ctx, p, "create_task", memory.Input{ProjectID: project, OperationID: "race-create-001", Title: "Concurrent task"})
	if e != nil {
		t.Fatal(e)
	}
	task := id(v)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Call(ctx, p, "update_task", memory.Input{ProjectID: project, OperationID: fmt.Sprintf("race-update-%d", i), TaskID: task, ExpectedRevision: 1, State: "implemented"})
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
	if _, e = s.Call(ctx, p, "attach_evidence", memory.Input{ProjectID: other, TaskID: task, OperationID: "cross-project-001", SourceKind: "test", Locator: "log:test", GitRevision: strings.Repeat("a", 40), Outcome: "pass", Body: "cross project"}); e == nil {
		t.Fatal("cross-project evidence accepted")
	}
	var n int
	if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE project_id=$1 AND operation_id='cross-project-001'", other).Scan(&n); e != nil || n != 0 {
		t.Fatal("failed write was not rolled back", e, n)
	}
	for _, table := range []string{"events", "records", "evidence", "attestations", "operations"} {
		if _, e = s.Pool.Exec(ctx, "UPDATE "+table+" SET project_id=project_id"); e == nil && table == "events" {
			t.Fatal("history update accepted")
		}
	}
	restricted := p
	restricted.Projects = []string{other}
	if _, e = s.Call(ctx, restricted, "get_task", memory.Input{ProjectID: project, TaskID: task}); e == nil {
		t.Fatal("project authorization bypass")
	}
	if _, e = s.Call(ctx, restricted, "create_project", memory.Input{Name: "forbidden", Repository: "https://github.com/tests/forbidden"}); e == nil {
		t.Fatal("scoped credential created project")
	}
	reader := client(t, srv, strings.Repeat("b", 64), "reader")
	invoke(t, reader, "list_tasks", map[string]any{"project_id": project}, false)
	invoke(t, reader, "create_task", map[string]any{"project_id": project, "operation_id": "reader-write-001", "title": "no"}, true)
	// Cancelled transaction cannot create a durable operation.
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.Call(dead, p, "append_event", memory.Input{ProjectID: project, OperationID: "cancelled-001", Body: "should fail"}); e == nil {
		t.Fatal("cancelled write succeeded")
	}
}
func TestHTTPBoundary(t *testing.T) {
	_, srv, _ := setup(t)
	for _, tc := range []struct {
		name, path, host, origin, token string
		status                          int
	}{{"unauthenticated", "/mcp", "", "", "", 401}, {"host", "/healthz", "attacker.example", "", "", 403}, {"origin", "/healthz", "", "http://attacker.example", "", 403}, {"ready", "/healthz", "", "", "", 200}} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", srv.URL+tc.path, nil)
			if tc.host != "" {
				r.Host = tc.host
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			res, e := http.DefaultClient.Do(r)
			if e != nil {
				t.Fatal(e)
			}
			res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatalf("got %d want %d", res.StatusCode, tc.status)
			}
		})
	}
}
func BenchmarkMCPContext(b *testing.B) {
	dsn := os.Getenv("THOTH_TEST_DATABASE_URL")
	if dsn == "" {
		b.Skip("database unset")
	}
	ctx := context.Background()
	s, e := store.Open(ctx, dsn)
	if e != nil {
		b.Fatal(e)
	}
	defer s.Pool.Close()
	p := memory.Principal{ID: "bench", Write: true, Projects: []string{"*"}}
	_ = s.Register(ctx, p)
	v, e := s.Call(ctx, p, "create_project", memory.Input{Name: "bench", Repository: "https://github.com/tests/benchmark"})
	if e != nil {
		b.Fatal(e)
	}
	project := id(v)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e = s.Call(ctx, p, "get_project_context", memory.Input{ProjectID: project}); e != nil {
			b.Fatal(e)
		}
	}
}

func TestClaimOwnershipAndBudgetRollback(t *testing.T) {
	s, _, p := setup(t)
	ctx := context.Background()
	v, e := s.Call(ctx, p, "create_project", memory.Input{Name: "Claim fixture", Repository: "https://github.com/tests/claims-" + fmt.Sprint(time.Now().UnixNano())})
	if e != nil {
		t.Fatal(e)
	}
	project := id(v)
	v, e = s.Call(ctx, p, "create_task", memory.Input{ProjectID: project, OperationID: "claim-task-create", Title: "Claim task"})
	if e != nil {
		t.Fatal(e)
	}
	task := id(v)
	if _, e = s.Call(ctx, p, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-first-agent", ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	other := memory.Principal{ID: "claim-other", Write: true, Projects: []string{project}}
	if e = s.Register(ctx, other); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Call(ctx, other, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-second-agent", ExpectedRevision: 2}); e == nil {
		t.Fatal("other agent stole claim")
	}
	if _, e = s.Call(ctx, other, "release_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "release-other-agent", ExpectedRevision: 2}); e == nil {
		t.Fatal("other agent released claim")
	}
	if _, e = s.Call(ctx, p, "release_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "release-first-agent", ExpectedRevision: 2}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Call(ctx, other, "claim_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "claim-after-release", ExpectedRevision: 3}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE projects SET event_budget=100,event_count=100 WHERE id=$1", project); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Call(ctx, p, "append_event", memory.Input{ProjectID: project, OperationID: "budget-reject-001", Body: "over quota"}); e == nil {
		t.Fatal("event budget not enforced")
	}
	var n int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE project_id=$1 AND operation_id='budget-reject-001'", project).Scan(&n)
	if n != 0 {
		t.Fatal("budget failure left operation")
	}
}
