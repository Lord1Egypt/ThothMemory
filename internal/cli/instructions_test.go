package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	thothmemory "github.com/Lord1Egypt/ThothMemory"
	"github.com/Lord1Egypt/ThothMemory/internal/mcptools"
)

func TestInstructionsPreserveExistingTextAndRefresh(t *testing.T) {
	dir := t.TempDir()
	original := "# My rules\nAlways use tabs.\n\n## Notes\nKeep this exactly."
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Instructions([]string{"--dir", dir, "--yes"}); err != nil {
		t.Fatal(err)
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	proto, _ := os.ReadFile(filepath.Join(dir, "THOTHMEMORY_AGENT_PROTOCOL.md"))
	if !strings.HasPrefix(string(agents), original) {
		t.Fatalf("existing instructions altered:\n%s", agents)
	}
	if string(proto) != thothmemory.AgentProtocol || !strings.Contains(string(proto), "record_idea") {
		t.Fatal("protocol file not installed verbatim")
	}
	if strings.Contains(string(agents), "@THOTHMEMORY") || !strings.Contains(string(claude), "@THOTHMEMORY_AGENT_PROTOCOL.md") {
		t.Fatal("only CLAUDE.md should use the @import")
	}
	first := string(agents)
	if err := Instructions([]string{"--dir", dir, "--yes"}); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(again) != first || strings.Count(string(again), blockBegin) != 1 {
		t.Fatal("repeat install is not idempotent")
	}
	// A hand edit outside the block survives a refresh; the block itself is replaced.
	edited := strings.Replace(first, "Keep this exactly.", "Keep this exactly.\nUser added line.", 1)
	edited = strings.Replace(edited, "Never store secrets.", "STALE TEXT", 1)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Instructions([]string{"--dir", dir, "--yes"}); err != nil {
		t.Fatal(err)
	}
	refreshed, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(refreshed), "User added line.") || strings.Contains(string(refreshed), "STALE TEXT") || strings.Count(string(refreshed), blockBegin) != 1 {
		t.Fatalf("refresh damaged or kept stale block:\n%s", refreshed)
	}
}

func TestProtocolMatchesRepositoryFile(t *testing.T) {
	b, err := os.ReadFile("../../THOTHMEMORY_AGENT_PROTOCOL.md")
	if err != nil || string(b) != thothmemory.AgentProtocol {
		t.Fatal("embedded protocol differs from THOTHMEMORY_AGENT_PROTOCOL.md")
	}
	for _, tool := range []string{"memory_record_idea", "memory_record_note", "memory_get_project_context", "memory_start_session", "memory_checkpoint_session", "memory_finish_session", "memory_search", "memory_release_task", "memory_record_test_result"} {
		if !strings.Contains(thothmemory.AgentProtocol, tool) {
			t.Errorf("protocol never mentions %s", tool)
		}
	}
}

// The protocol may only name tools that exist: a fictional tool name would
// teach every agent a call that fails.
func TestProtocolNamesOnlyRealTools(t *testing.T) {
	real := map[string]bool{}
	for _, c := range mcptools.Contracts {
		real["memory_"+c.Name] = true
	}
	for _, name := range regexp.MustCompile("memory_[a-z_]+").FindAllString(thothmemory.AgentProtocol, -1) {
		if !real[name] {
			t.Errorf("protocol names %s, which is not a tool", name)
		}
	}
}
