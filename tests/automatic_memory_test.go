package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"
)

// Evidence classes in this file, stated so a reader cannot mistake them:
//   - REAL: the real UltraLite executable, real SQLite file, real Git, and the
//     exact hook command strings the installers wrote, run through the shell
//     with the JSON payload shape clients send.
//   - SIMULATED CLIENTS: "claude", "codex" and "opencode" below are separate
//     MCP stdio sessions with their own scoped credentials; no model or
//     provider is involved. The genuine-client connection checks are in
//     TestRealClientsConnectToUltraLiteStdio.

type fixture struct {
	t              *testing.T
	home, repo, db string
}

func gitFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@example.invalid"}, {"config", "user.name", "Tester"}, {"remote", "add", "origin", "git@github.com:Example-Owner/Demo-App.git"}} {
		if out, e := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); e != nil {
			t.Fatal(string(out))
		}
	}
	commit(t, repo, "first.txt", "one")
	return repo
}

func commit(t *testing.T, repo, file, text string) string {
	t.Helper()
	if e := os.WriteFile(filepath.Join(repo, file), []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "change " + file}} {
		if out, e := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); e != nil {
			t.Fatal(string(out))
		}
	}
	out, _ := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	return strings.TrimSpace(string(out))
}

func (f fixture) ul(stdin string, env []string, args ...string) (string, error) {
	cmd := exec.Command(ultralite(f.t), append([]string{"--data", f.home}, args...)...)
	cmd.Dir = f.repo
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	e := cmd.Run()
	return out.String() + errb.String(), e
}

// hook runs the command string installed in a client's hook configuration,
// exactly as a client does: through the shell, in the checkout, payload on stdin.
func (f fixture) hook(file, event, payload string, env ...string) (string, error) {
	f.t.Helper()
	b, e := os.ReadFile(file)
	if e != nil {
		f.t.Fatal(e)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	if e = json.Unmarshal(b, &cfg); e != nil {
		f.t.Fatal(e)
	}
	groups := cfg.Hooks[event]
	if len(groups) == 0 || len(groups[0].Hooks) == 0 {
		f.t.Fatalf("no %s hook installed in %s", event, file)
	}
	cmd := exec.Command("sh", "-c", groups[len(groups)-1].Hooks[0].Command)
	cmd.Dir = f.repo
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(payload)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	e = cmd.Run()
	return out.String() + errb.String(), e
}

func (f fixture) agent(client string) *mcp.ClientSession {
	cs, _ := stdioAgent(f.t, f.home, client)
	f.t.Cleanup(func() { cs.Close() })
	return cs
}

func (f fixture) sql(query string, args ...any) int {
	f.t.Helper()
	db, e := sql.Open("sqlite", filepath.Join(f.home, "memory.db")+"?_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if e != nil {
		f.t.Fatal(e)
	}
	defer db.Close()
	var n int
	if e = db.QueryRow(query, args...).Scan(&n); e != nil {
		f.t.Fatal(e)
	}
	return n
}

func TestAutomaticMemoryEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hook command strings use POSIX sh; Windows relies on instructions and explicit writes")
	}
	f := fixture{t: t, home: filepath.Join(t.TempDir(), "memory"), repo: gitFixture(t)}
	head := func() string {
		out, _ := exec.Command("git", "-C", f.repo, "rev-parse", "HEAD").Output()
		return strings.TrimSpace(string(out))
	}

	// 1. Fresh installation and registration, with consent shown and preserved settings.
	if out, e := f.ul("", nil, "init"); e != nil {
		t.Fatal(out)
	}
	existing := `{"mcpServers":{"other":{"type":"stdio","command":"keep-me"}},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo user-hook"}]}]}}`
	if e := os.MkdirAll(filepath.Join(f.repo, ".claude"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(f.repo, ".mcp.json"), []byte(`{"mcpServers":{"other":{"type":"stdio","command":"keep-me"}}}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(f.repo, ".claude", "settings.json"), []byte(existing), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := f.ul("n\n", nil, "hooks", "claude", "--dir", f.repo); e == nil {
		t.Fatal("hooks installed without consent:", out)
	}
	if out, e := f.ul("", nil, "hooks", "claude", "--dir", f.repo, "--yes"); e != nil {
		t.Fatal(out)
	}
	claudeHooks := filepath.Join(f.repo, ".claude", "settings.json")
	b, _ := os.ReadFile(claudeHooks)
	if !strings.Contains(string(b), "echo user-hook") || !strings.Contains(string(b), "UserPromptSubmit") || !strings.Contains(string(b), "SessionEnd") {
		t.Fatalf("hooks not merged: %s", b)
	}
	if out, e := f.ul("", nil, "instructions", "--dir", f.repo, "--yes"); e != nil {
		t.Fatal(out)
	}
	if b, _ = os.ReadFile(filepath.Join(f.repo, "CLAUDE.md")); !strings.Contains(string(b), "@THOTHMEMORY_AGENT_PROTOCOL.md") {
		t.Fatal("CLAUDE.md does not reference the protocol")
	}
	if out, e := f.ul("", nil, "hooks", "codex", "--dir", f.repo, "--yes"); e != nil {
		t.Fatal(out)
	}
	codexHooks := filepath.Join(f.repo, ".codex", "hooks.json")
	if _, e := os.Stat(codexHooks); e != nil {
		t.Fatal("codex hooks missing")
	}

	// 2 & 3. Session start on a completely new project: it is registered once,
	// a durable session starts, and untrusted context is printed.
	out, e := f.hook(claudeHooks, "SessionStart", `{"hook_event_name":"SessionStart"}`)
	if e != nil || !strings.Contains(out, "UNTRUSTED DATA") || !strings.Contains(out, "started") {
		t.Fatal("session start:", e, out)
	}
	projects, _ := f.ul("", nil, "projects")
	if strings.Count(projects, "github.com/example-owner/demo-app") != 1 {
		t.Fatal("project not registered exactly once:", projects)
	}
	sessionLine := regexpFind(t, out, `Session ([0-9a-f-]{36}) \(revision (\d+)`)
	projectID := strings.Fields(projects)[0]
	if out, e = f.hook(claudeHooks, "SessionStart", `{}`); e != nil || !strings.Contains(out, "resumed") || !strings.Contains(out, sessionLine[1]) {
		t.Fatal("unfinished session not resumed:", e, out)
	}
	if strings.Count(func() string { p, _ := f.ul("", nil, "projects"); return p }(), "github.com/example-owner/demo-app") != 1 {
		t.Fatal("second start duplicated the project")
	}

	// 4-9. Agent writes over MCP stdio: note, idea, decision, failed work, passing work, checkpoint.
	claude := f.agent("claude")
	rev := head()
	note := invoke(t, claude, "record_note", map[string]any{"project_id": projectID, "operation_id": "e2e-note-1", "title": "Requirement", "body": "Dashboard must stay optional; agents talk MCP over stdio.", "session_id": sessionLine[1], "git_revision": rev, "agent": "claude-code"}, false).(map[string]any)
	idea := invoke(t, claude, "record_idea", map[string]any{"project_id": projectID, "operation_id": "e2e-idea-1", "title": "SQLite version for PocketClaw", "body": "Maybe we can make a SQLite version for PocketClaw.", "session_id": sessionLine[1], "agent": "claude-code"}, false).(map[string]any)
	if idea["kind"] != "idea" || note["kind"] != "note" {
		t.Fatal("kinds", idea, note)
	}
	invoke(t, claude, "record_decision", map[string]any{"project_id": projectID, "operation_id": "e2e-decision-1", "title": "stdio first", "body": "Use MCP stdio as the primary transport because it needs no daemon.", "git_revision": rev}, false)
	failing := id(invoke(t, claude, "create_task", map[string]any{"project_id": projectID, "operation_id": "e2e-task-fail", "title": "Port migration runner", "git_revision": rev}, false))
	invoke(t, claude, "record_issue", map[string]any{"project_id": projectID, "operation_id": "e2e-issue-1", "title": "Runner deadlocks", "body": "Attempt 1 failed: running DDL inside the writer loop deadlocks; cause is nested transactions.", "task_id": failing}, false)
	invoke(t, claude, "record_test_result", map[string]any{"project_id": projectID, "operation_id": "e2e-test-fail", "task_id": failing, "locator": "go test ./migrate", "git_revision": rev, "outcome": "fail", "body": "FAIL: deadlock after 10s"}, false)
	invoke(t, claude, "update_task", map[string]any{"project_id": projectID, "operation_id": "e2e-task-blocked", "task_id": failing, "expected_revision": 1, "state": "blocked"}, false)
	good := id(invoke(t, claude, "create_task", map[string]any{"project_id": projectID, "operation_id": "e2e-task-ok", "title": "Add backup command", "git_revision": rev}, false))
	invoke(t, claude, "record_test_result", map[string]any{"project_id": projectID, "operation_id": "e2e-test-pass", "task_id": good, "locator": "go test ./backup", "git_revision": rev, "outcome": "pass", "body": "ok backup 0.4s"}, false)
	invoke(t, claude, "update_task", map[string]any{"project_id": projectID, "operation_id": "e2e-task-tested", "task_id": good, "expected_revision": 1, "state": "tested", "git_revision": rev}, false)
	invoke(t, claude, "update_task", map[string]any{"project_id": projectID, "operation_id": "e2e-task-verify", "task_id": good, "expected_revision": 2, "state": "verified", "git_revision": rev}, true) // agents cannot verify
	invoke(t, claude, "checkpoint_session", map[string]any{"project_id": projectID, "operation_id": "e2e-checkpoint-1", "session_id": sessionLine[1], "expected_revision": 1, "git_revision": rev, "body": "Backup done and tested; migration runner blocked on deadlock. Next: serialize DDL."}, false)

	// 10. Context compaction: the PreCompact hook records a marker on the same session.
	rev2 := commit(t, f.repo, "second.txt", "two")
	if out, e = f.hook(claudeHooks, "PreCompact", `{}`); e != nil {
		t.Fatal(out)
	}
	if f.sql("SELECT COUNT(*) FROM events WHERE kind='checkpoint_session' AND body LIKE '%compact%' AND git_revision=?", rev2) != 1 {
		t.Fatal("compaction checkpoint not recorded at the new revision")
	}

	// 11 & 12. Switch to Codex, then OpenCode: each resumes from memory alone.
	claude.Close()
	codex := f.agent("codex")
	brief := invoke(t, codex, "get_project_context", map[string]any{"project_id": projectID, "git_revision": rev2}, false).(map[string]any)
	flat, _ := json.Marshal(brief)
	for _, want := range []string{"SQLite version for PocketClaw", "Dashboard must stay optional", "Runner deadlocks", "Backup done and tested", "serialize DDL"} {
		if !strings.Contains(string(flat), want) {
			t.Errorf("Codex briefing lacks %q", want)
		}
	}
	if len(brief["tasks"].([]any)) != 2 {
		t.Error("tasks missing for Codex")
	}
	codexSession := id(invoke(t, codex, "start_session", map[string]any{"project_id": projectID, "operation_id": "e2e-codex-start", "agent": "codex", "git_revision": rev2}, false))
	invoke(t, codex, "record_note", map[string]any{"project_id": projectID, "operation_id": "e2e-codex-note", "title": "Small note from Codex", "body": "Windows users will want the data directory under LOCALAPPDATA.", "session_id": codexSession}, false)
	invoke(t, codex, "finish_session", map[string]any{"project_id": projectID, "operation_id": "e2e-codex-finish", "session_id": codexSession, "expected_revision": 1, "git_revision": rev2, "body": "Partially completed: reviewed state, added a Windows note. Blocked task unchanged. Tests not run. Next: serialize DDL."}, false)
	codex.Close()
	opencode := f.agent("opencode")
	if found := invoke(t, opencode, "search", map[string]any{"project_id": projectID, "query": "LOCALAPPDATA"}, false).([]any); len(found) == 0 {
		t.Fatal("OpenCode cannot find Codex's note")
	}
	h := invoke(t, opencode, "get_latest_handoff", map[string]any{"project_id": projectID}, false).([]any)
	if len(h) != 1 || !strings.Contains(h[0].(map[string]any)["body"].(string), "Partially completed") {
		t.Fatal("OpenCode did not receive the Codex handoff", h)
	}
	opencode.Close()

	// 13. Unexpected interruption: the Claude session never ended; the next start resumes it
	// and the owner view reports it as unfinished.
	if out, e = f.hook(claudeHooks, "SessionStart", `{}`); e != nil || !strings.Contains(out, "resumed") {
		t.Fatal("interrupted session not resumed:", out)
	}
	if p, _ := f.ul("", nil, "projects"); !strings.Contains(p, "unfinished session") {
		t.Fatal("owner view does not flag the unfinished session:", p)
	}

	// 17. An idea from several sessions earlier is found by search.
	if out, e = f.ul("", nil, "search", projectID, "PocketClaw"); e != nil || !strings.Contains(out, "SQLite version for PocketClaw") {
		t.Fatal("older idea not found:", out)
	}

	// 19 & the privacy rules: secrets are not captured by agents or by prompt capture.
	cl := f.agent("claude")
	invoke(t, cl, "record_note", map[string]any{"project_id": projectID, "operation_id": "e2e-secret-note", "title": "oops", "body": "deploy token gh" + "p_" + strings.Repeat("Ab12", 10)}, true)
	if out, e = f.ul("", nil, "capture", "set", "prompts"); e == nil {
		t.Fatal("prompt capture enabled without consent:", out)
	}
	if out, e = f.ul("", nil, "capture", "set", "prompts", "--yes"); e != nil {
		t.Fatal(out)
	}
	before := f.sql("SELECT COUNT(*) FROM records WHERE title='User request'")
	if out, e = f.hook(claudeHooks, "UserPromptSubmit", `{"prompt":"my pass`+`word: `+strings.Repeat("hunter2", 4)+` please log in"}`); e != nil {
		t.Fatal(out)
	}
	if f.sql("SELECT COUNT(*) FROM records WHERE title='User request'") != before {
		t.Fatal("a credential-looking prompt was stored")
	}
	if out, e = f.hook(claudeHooks, "UserPromptSubmit", `{"prompt":"Can we also support Arabic project names in the overview page?"}`); e != nil {
		t.Fatal(out)
	}
	if f.sql("SELECT COUNT(*) FROM records WHERE title='User request' AND body LIKE '%Arabic project names%'") != 1 {
		t.Fatal("opted-in prompt not captured as a note")
	}
	if out, e = f.hook(claudeHooks, "UserPromptSubmit", `{"prompt":"Can we also support Arabic project names in the overview page?"}`); e != nil {
		t.Fatal(out)
	}
	if f.sql("SELECT COUNT(*) FROM records WHERE title='User request' AND body LIKE '%Arabic project names%'") != 1 {
		t.Fatal("repeating a prompt created a second record")
	}
	if out, e = f.hook(claudeHooks, "UserPromptSubmit", `{"prompt":"ok"}`); e != nil || f.sql("SELECT COUNT(*) FROM records WHERE title='User request'") != before+1 {
		t.Fatal("trivial prompt was stored:", out)
	}

	// 14 & 15. Memory unavailable: another process holds the database write lock.
	// The hook reports it, queues the write, and later replays it exactly once.
	busy, e := sql.Open("sqlite", filepath.Join(f.home, "memory.db")+"?_pragma=busy_timeout(2000)")
	if e != nil {
		t.Fatal(e)
	}
	defer busy.Close()
	conn, e := busy.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); e != nil {
		t.Fatal(e)
	}
	out, e = f.hook(claudeHooks, "UserPromptSubmit", `{"prompt":"Remember that exports must keep Arabic text intact end to end."}`, "THOTH_SQLITE_BUSY_MS=300")
	if e != nil || !strings.Contains(out, "queued") {
		t.Fatal("unavailable memory was not reported as queued:", e, out)
	}
	if st, _ := f.ul("", nil, "capture", "pending"); !strings.Contains(st, "pending writes: 1") {
		t.Fatal("queue not recorded:", st)
	}
	queueFile := filepath.Join(f.home, "capture", "pending.jsonl")
	saved, e := os.ReadFile(queueFile)
	if e != nil || info(t, queueFile)&0077 != 0 {
		t.Fatal("pending queue missing or not owner-only")
	}
	if _, e = conn.ExecContext(context.Background(), "ROLLBACK"); e != nil {
		t.Fatal(e)
	}
	conn.Close()
	if out, e = f.hook(claudeHooks, "Stop", `{}`); e != nil || !strings.Contains(out, "replayed 1") {
		t.Fatal("queue not replayed on the next hook:", e, out)
	}
	count := func() int {
		return f.sql("SELECT COUNT(*) FROM records WHERE title='User request' AND body LIKE '%exports must keep Arabic%'")
	}
	if count() != 1 {
		t.Fatal("queued prompt not recorded exactly once")
	}
	// The same queued write delivered a second time (an uncertain retry) must not duplicate it.
	if e = os.WriteFile(queueFile, saved, 0600); e != nil {
		t.Fatal(e)
	}
	if out, e = f.ul("", nil, "capture", "retry"); e != nil || count() != 1 {
		t.Fatal("replaying an already-delivered write duplicated it:", e, out, count())
	}

	// 18. Cross-project isolation: narrow Claude to another project.
	other := id(invoke(t, cl, "create_project", map[string]any{"name": "Other", "repository": "local:99999999-8888-4777-8666-555555555555"}, false))
	if out, e = f.ul("y\n", nil, "permissions", "claude", "--projects", other); e != nil {
		t.Fatal(out)
	}
	cl.Close()
	scoped := f.agent("claude")
	invoke(t, scoped, "get_project_context", map[string]any{"project_id": projectID}, true)
	if ps := invoke(t, scoped, "list_projects", map[string]any{}, false).([]any); len(ps) != 1 || ps[0].(map[string]any)["id"] != other {
		t.Fatal("scoped agent sees other projects", ps)
	}
	scoped.Close()
	// Scoped hooks cannot reach the other project either: nothing is recorded there.
	n := f.sql("SELECT COUNT(*) FROM events WHERE project_id=?", projectID)
	_, _ = f.hook(claudeHooks, "Stop", `{}`)
	if f.sql("SELECT COUNT(*) FROM events WHERE project_id=?", projectID) != n {
		t.Fatal("a scoped hook wrote into a project it is not allowed to use")
	}

	// 20. Disabling automatic capture keeps everything already stored.
	if out, e = f.ul("", nil, "capture", "set", "off"); e != nil {
		t.Fatal(out)
	}
	total := f.sql("SELECT (SELECT COUNT(*) FROM events)+(SELECT COUNT(*) FROM records)+(SELECT COUNT(*) FROM sessions)")
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreCompact", "Stop", "SessionEnd"} {
		if out, e = f.hook(codexHooks, ev, `{"prompt":"a long enough request that would otherwise be recorded"}`); e != nil || strings.TrimSpace(out) != "" {
			t.Fatalf("%s produced output or failed with capture off: %v %q", ev, e, out)
		}
	}
	if f.sql("SELECT (SELECT COUNT(*) FROM events)+(SELECT COUNT(*) FROM records)+(SELECT COUNT(*) FROM sessions)") != total {
		t.Fatal("capture off still wrote memory")
	}
	if out, e = f.ul("", nil, "search", projectID, "PocketClaw"); e != nil || !strings.Contains(out, "SQLite version for PocketClaw") {
		t.Fatal("existing memory lost after disabling capture")
	}
	// Explicit agent writes still work: "off" disables hooks, not MCP.
	again := f.agent("codex")
	invoke(t, again, "record_note", map[string]any{"project_id": projectID, "operation_id": "e2e-after-off", "title": "Still works", "body": "Agent MCP writes are unaffected by capture off."}, false)

	// 16. Restart persistence and integrity after all of the above.
	again.Close()
	if out, e = f.ul("", nil, "doctor"); e != nil || !strings.Contains(out, "UltraLite is healthy") {
		t.Fatal("doctor:", out)
	}
	if out, _ = f.ul("", nil, "brief", projectID, "--head", rev2); !strings.Contains(out, "Small note from Codex") || !strings.Contains(out, "stale") {
		t.Fatal("restart briefing:", out)
	}
	if p, _ := f.ul("", nil, "capture", "pending"); !strings.Contains(p, "pending writes: 0") {
		t.Fatal("queue not empty at the end:", p)
	}
	_ = time.Second
}

func info(t *testing.T, path string) os.FileMode {
	st, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	return st.Mode().Perm()
}

func regexpFind(t *testing.T, text, pattern string) []string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("pattern %q not found in:\n%s", pattern, text)
	}
	return m
}
