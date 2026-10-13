// Command thothmemory-ultralite is ThothMemory UltraLite: shared agent memory
// in one native executable and one SQLite file. No Docker, no database
// server, no web server: agents launch it over MCP stdio; owners use the CLI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/cli"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/sqlite"
	"github.com/Lord1Egypt/ThothMemory/internal/ultralite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const usage = `ThothMemory UltraLite %s (SQLite; headless: no server, no web UI)

  init                          create the data directory, credentials and database
  connect codex|claude|opencode|all [--scope project|user] [--dir PATH] [--yes]
                                register clients to launch memory directly (mcp-stdio)
  doctor                        verify files, permissions, integrity and a real MCP session
  mcp-stdio --token-file FILE   serve one client over stdin/stdout (clients launch this)
  status                        storage usage and counts

Inspect memory (read-only, as the local owner):
  projects                      every project: activity, blockers, next action
  brief PROJECT [--head SHA]    resume briefing; --head marks stale evidence
  search PROJECT QUERY          search records, tasks, events and evidence
  tasks PROJECT [--state S]     tasks, optionally by state
  timeline PROJECT              recent activity
  call TOOL [JSON]              any read tool by MCP name, e.g. call get_task '{"project_id":"…","task_id":"…"}'

Manage memory:
  backup [FILE]                 consistent online snapshot with SHA-256 sidecar
  restore FILE [--confirm]      verify, then replace memory (previous file is kept)
  export PROJECT > FILE         portable project export (Full, Mini and UltraLite)
  import < FILE                 fast-forward import; never overwrites newer state
  attest PROJECT EVIDENCE OPERATION STATEMENT   owner review of reported evidence
  permissions PRINCIPAL ...     narrow, read-only or revoke a client credential
  instructions [--dir PATH]     add the shared memory protocol to AGENTS.md/CLAUDE.md

Data directory: %s (override with --data DIR or THOTH_HOME)
`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx, os.Args[1:]); e != nil {
		slog.Error("operation failed", "error", memory.SafeError(e))
		os.Exit(1)
	}
}

func interactive(f *os.File) bool {
	st, e := f.Stat()
	return e == nil && st.Mode()&os.ModeCharDevice != 0
}

// flag removes "--name VALUE" from args and returns VALUE.
func flag(args []string, name string) (string, []string) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1], append(append([]string{}, args[:i]...), args[i+2:]...)
		}
	}
	return "", args
}

func run(ctx context.Context, args []string) error {
	dir, args := flag(args, "--data")
	if dir == "" {
		d, e := ultralite.DefaultDir()
		if e != nil {
			return e
		}
		dir = d
	}
	dir, _ = filepath.Abs(dir)
	h := ultralite.Home{Dir: dir}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Printf(usage, memory.UltraLiteVersion, dir)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version":
		fmt.Println(memory.UltraLiteVersion)
		return nil
	case "init":
		if e := ultralite.Init(ctx, h); e != nil {
			return e
		}
		fmt.Printf("UltraLite memory ready in %s\nNext: thothmemory-ultralite connect all   (then restart your agents)\n", dir)
		return nil
	case "mcp-stdio":
		token, _ := flag(rest, "--token-file")
		if token == "" {
			return errors.New("mcp-stdio --token-file FILE")
		}
		// stdout carries the protocol; diagnostics go to stderr only.
		return ultralite.Stdio(ctx, h, token)
	case "projects", "brief", "search", "tasks", "timeline", "call":
		return inspect(ctx, h, cmd, rest)
	case "status":
		st, e := ultralite.Status(ctx, h)
		if e != nil {
			return e
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	case "doctor":
		return doctor(ctx, h)
	case "backup":
		target := ""
		if len(rest) == 1 {
			target = rest[0]
		}
		path, sum, e := ultralite.Backup(ctx, h, target)
		if e == nil {
			fmt.Printf("Backup verified: %s\nsha256=%s\n", path, sum)
		}
		return e
	case "restore":
		if len(rest) < 1 {
			return errors.New("restore FILE [--confirm]")
		}
		confirmed := len(rest) == 2 && rest[1] == "--confirm"
		kept, e := ultralite.Restore(ctx, h, rest[0], confirmed)
		if e == nil {
			fmt.Printf("Restored. Previous memory kept at: %s\n", kept)
		}
		return e
	case "export", "import", "attest":
		s, release, e := ultralite.Open(ctx, h)
		if e != nil {
			return e
		}
		defer release()
		if cmd != "attest" {
			return cli.Portable(ctx, s, cmd, rest)
		}
		if len(rest) != 4 {
			return errors.New("attest PROJECT_ID EVIDENCE_ID OPERATION_ID STATEMENT")
		}
		p := memory.Principal{ID: "local-owner", Owner: true, Write: true, Projects: []string{"*"}}
		if e = s.Register(ctx, p); e != nil {
			return e
		}
		v, e := s.Call(ctx, p, "attest_evidence", memory.Input{ProjectID: rest[0], EvidenceID: rest[1], OperationID: rest[2], Body: rest[3]})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "connect":
		return cli.ConnectWith(ctx, rest, cli.Launch{
			Args: func(client string) []string {
				return []string{"mcp-stdio", "--data", dir, "--token-file", filepath.Join(h.Secrets(), client+"_token")}
			},
			Verify: func(ctx context.Context) error { return doctor(ctx, h) },
		})
	case "permissions":
		// The policy lives in DIR/.secrets; each new mcp-stdio session reads it.
		if e := os.Chdir(dir); e != nil {
			return e
		}
		return cli.Permissions(rest)
	case "instructions":
		return cli.Instructions(rest)
	}
	return fmt.Errorf("unknown command %q; run help", cmd)
}

// doctor checks files, integrity and a genuine stdio MCP session through a
// child process, then confirms the child exited when the session closed.
func doctor(ctx context.Context, h ultralite.Home) error {
	fail := 0
	report := func(ok bool, what string, detail any) {
		mark := "OK  "
		if !ok {
			mark, fail = "FAIL", fail+1
		}
		fmt.Printf("%s %s %v\n", mark, what, detail)
	}
	for _, p := range []struct {
		path string
		mode os.FileMode
	}{{h.Dir, 0700}, {h.Secrets(), 0700}, {h.DB(), 0600}} {
		st, e := os.Stat(p.path)
		switch {
		case e != nil:
			report(false, "exists", p.path)
		case runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0:
			report(false, "owner-only permissions", fmt.Sprintf("%s is %v", p.path, st.Mode().Perm()))
		default:
			report(true, "owner-only", p.path)
		}
	}
	if fail > 0 {
		return errors.New("run: thothmemory-ultralite init")
	}
	st, e := ultralite.Status(ctx, h)
	if e != nil {
		report(false, "database", memory.SafeError(e))
		return errors.New("doctor found problems")
	}
	report(true, "database", fmt.Sprintf("%v, schema %v, %v bytes", st["engine"], st["schema"], st["database_bytes"]))
	s, err := sqlite.Open(ctx, h.DB())
	if err == nil {
		err = sqlite.Check(ctx, s.DB)
		s.Close()
	}
	report(err == nil, "integrity and foreign keys", memory.SafeError(errOrNil(err)))
	exe, _ := os.Executable()
	child := exec.Command(exe, "--data", h.Dir, "mcp-stdio", "--token-file", filepath.Join(h.Secrets(), "read_token"))
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	start := time.Now()
	cs, e := mcp.NewClient(&mcp.Implementation{Name: "thothmemory-doctor", Version: memory.UltraLiteVersion}, nil).Connect(cctx, &mcp.CommandTransport{Command: child}, nil)
	if e != nil {
		report(false, "MCP stdio handshake", e)
		return errors.New("doctor found problems")
	}
	tools, e := cs.ListTools(cctx, nil)
	if e == nil {
		_, e = cs.CallTool(cctx, &mcp.CallToolParams{Name: "memory_get_health", Arguments: map[string]any{}})
	}
	elapsed := time.Since(start)
	_ = cs.Close()
	report(e == nil, "MCP stdio session (read credential)", fmt.Sprintf("%d tools, handshake+health %v", len(tools.Tools), elapsed.Round(time.Millisecond)))
	exited := child.ProcessState != nil && child.ProcessState.Exited()
	report(exited, "stdio process exited after the session", fmt.Sprint(exited))
	if fail > 0 {
		return errors.New("doctor found problems")
	}
	fmt.Println("UltraLite is healthy.")
	return nil
}

func errOrNil(e error) error {
	if e == nil {
		return errors.New("ok")
	}
	return e
}

// inspect runs read tools as the local owner through the same validated path
// MCP clients use. Stored content is printed as data, never interpreted.
func inspect(ctx context.Context, h ultralite.Home, cmd string, args []string) error {
	head, args := flag(args, "--head")
	state, args := flag(args, "--state")
	tool, in := "", map[string]any{}
	need := func(n int, form string) error {
		if len(args) != n {
			return errors.New("usage: " + cmd + " " + form)
		}
		return nil
	}
	switch cmd {
	case "projects":
		tool = "list_projects"
		if e := need(0, ""); e != nil {
			return e
		}
	case "brief":
		tool, in["project_id"] = "get_project_context", first(args)
		if head != "" {
			in["git_revision"] = head
		}
		if e := need(1, "PROJECT [--head SHA]"); e != nil {
			return e
		}
	case "search":
		if e := need(2, "PROJECT QUERY"); e != nil {
			return e
		}
		tool, in["project_id"], in["query"] = "search", args[0], args[1]
	case "tasks":
		tool, in["project_id"] = "list_tasks", first(args)
		if state != "" {
			in["state"] = state
		}
		if e := need(1, "PROJECT [--state STATE]"); e != nil {
			return e
		}
	case "timeline":
		tool, in["project_id"], in["limit"] = "get_recent_activity", first(args), 50
		if e := need(1, "PROJECT"); e != nil {
			return e
		}
	case "call":
		if len(args) < 1 || len(args) > 2 {
			return errors.New("usage: call TOOL [JSON]")
		}
		tool = args[0]
		if len(args) == 2 {
			if e := json.Unmarshal([]byte(args[1]), &in); e != nil {
				return errors.New("arguments must be a JSON object")
			}
		}
	}
	if memory.IsWrite(tool) {
		return errors.New("inspection is read-only; agents write through MCP")
	}
	s, release, e := ultralite.Open(ctx, h)
	if e != nil {
		return e
	}
	defer release()
	if cmd == "projects" {
		rows, e := memory.Call(ctx, s, ultralite.Owner, "overview", memory.Input{})
		if e != nil {
			return e
		}
		list := rows.([]memory.Row)
		if len(list) == 0 {
			fmt.Println("No projects yet. Agents register one with memory_create_project.")
		}
		for _, r := range list {
			last := "no activity"
			if v, ok := r["last_activity_at"].(string); ok {
				last = "active " + v
			}
			fmt.Printf("%s  %s\n  %s  (%s)\n  next: %s\n\n", r["id"], r["name"], r["repository"], last, r["next_recommended_action"])
		}
		return nil
	}
	b, _ := json.Marshal(in)
	v, e := ultralite.Call(ctx, s, ultralite.Owner, tool, b)
	if e != nil {
		return e
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(map[string]any{"data": v})
}

func first(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
