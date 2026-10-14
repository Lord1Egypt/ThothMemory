package memorycore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Lord1Egypt/ThothMemory/pkg/memorycore"
)

// The embedding contract PocketClaw would rely on: no subprocess, socket or
// listener, same validation and authorization as MCP, safe for concurrent use.
func TestEmbeddingContract(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "app-private")
	if e := memorycore.Init(ctx, dir); e != nil {
		t.Fatal(e)
	}
	m, e := memorycore.Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	agent, e := m.Session(ctx, filepath.Join(dir, ".secrets", "agent_token"))
	if e != nil {
		t.Fatal(e)
	}
	reader, e := m.Session(ctx, filepath.Join(dir, ".secrets", "read_token"))
	if e != nil {
		t.Fatal(e)
	}
	if len(agent.Tools()) != 25 || len(reader.Tools()) >= len(agent.Tools()) {
		t.Fatal("tool lists", len(agent.Tools()), len(reader.Tools()))
	}
	for _, tool := range reader.Tools() {
		if !tool.ReadOnly {
			t.Fatal("read session sees write tool", tool.Name)
		}
	}
	call := func(s *memorycore.Session, tool, args string) (map[string]any, error) {
		out, e := s.Call(ctx, tool, []byte(args))
		if e != nil {
			return nil, e
		}
		var v struct{ Data map[string]any }
		return v.Data, json.Unmarshal(out, &v)
	}
	p, e := call(agent, "memory_create_project", `{"name":"Pocket","repository":"local:11111111-2222-4333-8444-555555555555"}`)
	if e != nil {
		t.Fatal(e)
	}
	project := p["id"].(string)
	if _, e = call(agent, "record_idea", fmt.Sprintf(`{"project_id":%q,"operation_id":"embed-idea-1","title":"Embed","body":"Run memory inside the app process."}`, project)); e != nil {
		t.Fatal(e)
	}
	// Validation and authorization are the MCP ones, with sanitized errors.
	if _, e = call(agent, "record_idea", `{"project_id":"x","bogus":1}`); e == nil {
		t.Fatal("invalid arguments accepted")
	}
	if _, e = call(reader, "record_idea", fmt.Sprintf(`{"project_id":%q,"operation_id":"embed-idea-2","title":"x","body":"y"}`, project)); e == nil || !strings.Contains(e.Error(), "forbidden") {
		t.Fatal("read-only session wrote:", e)
	}
	// Build a synthetic credential at runtime; never put credential-shaped literals
	// in source control. Keep checking both refusal and sanitized error output.
	secretFixture := strings.Repeat("fixture", 5)
	if _, e = call(agent, "record_note", fmt.Sprintf(`{"project_id":%q,"operation_id":"embed-secret-1","title":"x","body":%q}`, project, "password: "+secretFixture)); e == nil || strings.Contains(e.Error(), secretFixture) {
		t.Fatal("secret accepted or echoed:", e)
	}
	// Concurrent goroutines, as an agent runtime would call it.
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := call(agent, "record_note", fmt.Sprintf(`{"project_id":%q,"operation_id":"embed-note-%02d","title":"Note %d","body":"concurrent note %d"}`, project, i, i, i))
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	out, e := reader.Call(ctx, "search", []byte(fmt.Sprintf(`{"project_id":%q,"query":"concurrent note","limit":100}`, project)))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Count(string(out), `"kind":"note"`) != 20 {
		t.Fatal("expected 20 notes", string(out))
	}
	// Backup while open, then close releases everything: a second Open succeeds.
	sum, e := m.Backup(ctx, filepath.Join(t.TempDir(), "b.db"))
	if e != nil || len(sum) != 64 {
		t.Fatal("backup", e)
	}
	m.Close()
	again, e := memorycore.Open(ctx, dir)
	if e != nil {
		t.Fatal("reopen after Close:", e)
	}
	again.Close()
}
