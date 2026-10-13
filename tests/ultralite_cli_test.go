package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ultraliteBinary builds the real executable once per test binary run.
var ultraliteBuild struct {
	once sync.Once
	path string
	err  error
}

func ultralite(t *testing.T) string {
	t.Helper()
	ultraliteBuild.once.Do(func() {
		dir, e := os.MkdirTemp("", "ultralite-bin-")
		if e != nil {
			ultraliteBuild.err = e
			return
		}
		ultraliteBuild.path = filepath.Join(dir, "thothmemory-ultralite")
		out, e := exec.Command("go", "build", "-buildvcs=false", "-o", ultraliteBuild.path, "../cmd/thothmemory-ultralite").CombinedOutput()
		if e != nil {
			ultraliteBuild.err = e
			ultraliteBuild.path = string(out)
		}
	})
	if ultraliteBuild.err != nil {
		t.Fatal(ultraliteBuild.err, ultraliteBuild.path)
	}
	return ultraliteBuild.path
}

func run(t *testing.T, home string, stdin []byte, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(ultralite(t), append([]string{"--data", home}, args...)...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	e := cmd.Run()
	if e != nil {
		return out.String() + errb.String(), e
	}
	return out.String(), nil
}

// stdioAgent launches mcp-stdio exactly as a registered client does.
func stdioAgent(t *testing.T, home, client string) (*mcp.ClientSession, *exec.Cmd) {
	t.Helper()
	cmd := exec.Command(ultralite(t), "--data", home, "mcp-stdio", "--token-file", filepath.Join(home, ".secrets", client+"_token"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cs, e := mcp.NewClient(&mcp.Implementation{Name: client, Version: "test"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if e != nil {
		t.Fatal(client, e)
	}
	return cs, cmd
}

func TestUltraLiteNativeCLIAndStdio(t *testing.T) {
	home := filepath.Join(t.TempDir(), "memory home")
	if out, e := run(t, home, nil, "init"); e != nil {
		t.Fatal(out)
	}
	if out, e := run(t, home, nil, "init"); e != nil {
		t.Fatal("repeat init:", out)
	}
	head := strings.Repeat("7", 40)
	claude, claudeCmd := stdioAgent(t, home, "claude")
	tools, e := claude.ListTools(context.Background(), nil)
	if e != nil || len(tools.Tools) != 23 {
		t.Fatal("stdio discovery", e)
	}
	project := id(invoke(t, claude, "create_project", map[string]any{"name": "UltraLite stdio", "repository": "local:11111111-2222-4333-8444-555555555555"}, false))
	task := id(invoke(t, claude, "create_task", map[string]any{"project_id": project, "operation_id": "ul-create-task", "title": "Headless memory", "git_revision": head}, false))
	sess := id(invoke(t, claude, "start_session", map[string]any{"project_id": project, "operation_id": "ul-start", "agent": "claude-code", "git_revision": head}, false))
	invoke(t, claude, "record_decision", map[string]any{"project_id": project, "operation_id": "ul-decision", "title": "SQLite WAL", "body": "استخدام SQLite مع WAL", "git_revision": head, "session_id": sess}, false)
	// Concurrent agents in separate processes contend for one task revision.
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	racers := []*exec.Cmd{}
	for _, c := range []string{"codex", "opencode"} {
		cs, cmd := stdioAgent(t, home, c)
		racers = append(racers, cmd)
		defer cs.Close()
		wg.Add(1)
		go func(c string, cs *mcp.ClientSession) {
			defer wg.Done()
			r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_update_task", Arguments: map[string]any{"project_id": project, "task_id": task, "operation_id": "ul-race-" + c, "expected_revision": 1, "state": "implemented"}})
			results <- e == nil && !r.IsError
		}(c, cs)
	}
	wg.Wait()
	close(results)
	defer func() {
		for _, c := range racers {
			_ = c.Process.Kill()
		}
	}()
	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d stdio agents won the same revision", wins)
	}
	invoke(t, claude, "finish_session", map[string]any{"project_id": project, "session_id": sess, "operation_id": "ul-finish", "expected_revision": 1, "git_revision": head, "body": "Handoff over stdio."}, false)
	claude.Close()
	done := make(chan error, 1)
	go func() { done <- claudeCmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("mcp-stdio process did not exit after the client disconnected")
	}
	// A different agent process resumes from the durable handoff.
	codex, _ := stdioAgent(t, home, "codex")
	h := invoke(t, codex, "get_latest_handoff", map[string]any{"project_id": project}, false).([]any)
	if len(h) != 1 || h[0].(map[string]any)["body"] != "Handoff over stdio." {
		t.Fatal("handoff not visible to the next agent", h)
	}
	codex.Close()
	// The read credential cannot write, even over stdio.
	reader, _ := stdioAgent(t, home, "read")
	invoke(t, reader, "append_event", map[string]any{"project_id": project, "operation_id": "ul-reader-write", "body": "x"}, true)
	reader.Close()
	// Revocation applies to the next session; nothing is cached.
	if out, e := run(t, home, []byte("y\n"), "permissions", "opencode", "--revoke"); e != nil {
		t.Fatal(out)
	}
	cmd := exec.Command(ultralite(t), "--data", home, "mcp-stdio", "--token-file", filepath.Join(home, ".secrets", "opencode_token"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if e = cmd.Run(); e == nil || !strings.Contains(stderr.String(), "unauthorized") {
		t.Fatal("revoked token was accepted:", e, stderr.String())
	}
	// Inspection runs read-only as the owner.
	if out, e := run(t, home, nil, "projects"); e != nil || !strings.Contains(out, "UltraLite stdio") {
		t.Fatal("projects:", out)
	}
	if out, e := run(t, home, nil, "search", project, "WAL"); e != nil || !strings.Contains(out, "استخدام SQLite") {
		t.Fatal("search:", out)
	}
	if out, e := run(t, home, nil, "call", "append_event", `{"project_id":"`+project+`","operation_id":"cli-write-01","body":"x"}`); e == nil {
		t.Fatal("inspection performed a write:", out)
	}
	// Backup, refuse restore while memory is in use, then restore.
	out, e := run(t, home, nil, "backup")
	if e != nil {
		t.Fatal(out)
	}
	backup := strings.TrimPrefix(strings.Split(out, "\n")[0], "Backup verified: ")
	if _, e = run(t, home, nil, "restore", backup); e == nil {
		t.Fatal("restore without --confirm succeeded")
	}
	for _, c := range racers {
		_ = c.Process.Kill()
		_ = c.Wait()
	}
	busy, busyCmd := stdioAgent(t, home, "codex")
	if out, e = run(t, home, nil, "restore", backup, "--confirm"); e == nil || !strings.Contains(out, "in use") {
		t.Fatal("restore while in use:", out)
	}
	busy.Close()
	_ = busyCmd.Wait()
	if out, e = run(t, home, nil, "restore", backup, "--confirm"); e != nil {
		t.Fatal(out)
	}
	if out, e = run(t, home, nil, "doctor"); e != nil || !strings.Contains(out, "UltraLite is healthy") {
		t.Fatal("doctor after restore:", out)
	}
	// Export and re-import into a fresh UltraLite home.
	exported, e := run(t, home, nil, "export", project)
	if e != nil {
		t.Fatal(exported)
	}
	other := filepath.Join(t.TempDir(), "second")
	if out, e = run(t, other, nil, "init"); e != nil {
		t.Fatal(out)
	}
	if out, e = run(t, other, []byte(exported), "import"); e != nil {
		t.Fatal(out)
	}
	var res struct {
		Inserted map[string]int `json:"inserted"`
	}
	if e = json.Unmarshal([]byte(out), &res); e != nil || res.Inserted["records"] != 2 {
		t.Fatal("import result", out)
	}
	if out, e = run(t, other, []byte(exported), "import"); e != nil || !strings.Contains(out, `"already_imported": true`) {
		t.Fatal("repeat import", out)
	}
}
