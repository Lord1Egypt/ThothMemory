package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Lord1Egypt/ThothMemory/internal/capture"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"strings"
)

func call(ctx context.Context, c *mcp.ClientSession, name string, args any) (any, error) {
	r, e := c.CallTool(ctx, &mcp.CallToolParams{Name: "memory_" + name, Arguments: args})
	if e != nil {
		return nil, e
	}
	if r.IsError {
		return nil, errors.New(r.Content[0].(*mcp.TextContent).Text)
	}
	var out struct {
		Data any `json:"data"`
	}
	for _, v := range r.Content {
		if text, ok := v.(*mcp.TextContent); ok {
			if e = json.Unmarshal([]byte(text.Text), &out); e == nil {
				return out.Data, nil
			}
		}
	}
	return nil, errors.New("missing tool data")
}
func operation() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }

// mcpCaller adapts an MCP client session to the capture engine.
type mcpCaller struct{ c *mcp.ClientSession }

func (m mcpCaller) Call(ctx context.Context, tool string, args map[string]any) (any, error) {
	return call(ctx, m.c, tool, args)
}

// CaptureDir is where capture settings and the retry queue live for the
// HTTP editions: beside the client configuration.
func CaptureDir(config string) capture.Dir {
	abs, _ := filepath.Abs(config)
	return capture.Dir(filepath.Join(filepath.Dir(abs), ".capture"))
}

// Lifecycle handles one client lifecycle event (hooks and plugins call it).
func Lifecycle(ctx context.Context, path string, args []string) error {
	if len(args) != 2 {
		return errors.New("lifecycle AGENT start|prompt|checkpoint|compacted|idle|error|end")
	}
	dir, _ := os.Getwd()
	ev := capture.Event{Agent: args[0], Name: args[1], Dir: dir, Stdin: capture.ReadStdin(os.Stdin)}
	d := CaptureDir(path)
	if mode, e := d.Mode(); e == nil && mode == capture.ModeOff {
		return nil
	}
	c, e := Client(ctx, path)
	if e != nil {
		// Unreachable service: start has nothing to show, others queue nothing
		// they cannot describe; say so on stderr and let the agent carry on.
		fmt.Fprintln(os.Stderr, "ThothMemory unavailable; record unsynced progress honestly and retry later.")
		return nil
	}
	defer c.Close()
	return capture.Run(ctx, mcpCaller{c}, d, ev, os.Stdout)
}

// Capture manages the capture mode and the retry queue for the HTTP editions.
func Capture(ctx context.Context, path string, args []string) error {
	d := CaptureDir(path)
	return CaptureCommand(d, args, func() (capture.Caller, func(), error) {
		c, e := Client(ctx, path)
		if e != nil {
			return nil, nil, e
		}
		return mcpCaller{c}, func() { c.Close() }, nil
	})
}

// CaptureCommand implements `capture status|set MODE|pending|retry` for any edition.
func CaptureCommand(d capture.Dir, args []string, connect func() (capture.Caller, func(), error)) error {
	if len(args) == 0 {
		args = []string{"status"}
	}
	switch args[0] {
	case "status":
		mode, e := d.Mode()
		if e != nil {
			return e
		}
		pending, rejected := capture.Pending(d)
		fmt.Printf("capture mode: %s\n  off       record nothing\n  metadata  lifecycle facts only (default once hooks are installed)\n  prompts   also store your request text as notes (opt-in)\npending writes: %d, rejected: %d (%s)\n", mode, pending, rejected, d)
		return nil
	case "set":
		if len(args) < 2 {
			return errors.New("capture set off|metadata|prompts [--yes]")
		}
		if args[1] == capture.ModePrompts && !(len(args) == 3 && args[2] == "--yes") {
			return errors.New("prompts mode stores the text you type to your agents (credential-looking prompts are skipped). Rerun with --yes to consent")
		}
		if e := d.SetMode(args[1]); e != nil {
			return e
		}
		fmt.Println("capture mode set to " + args[1] + ". Existing memory is untouched.")
		return nil
	case "retry", "pending":
		pending, rejected := capture.Pending(d)
		if args[0] == "pending" || pending == 0 {
			fmt.Printf("pending writes: %d, rejected: %d\n", pending, rejected)
			return nil
		}
		c, closeFn, e := connect()
		if e != nil {
			return e
		}
		defer closeFn()
		sent, left, e := capture.Drain(context.Background(), c, d)
		fmt.Printf("replayed %d, still pending %d\n", sent, left)
		return e
	}
	return errors.New("capture status|set MODE|pending|retry")
}
func Demo(ctx context.Context, path string) error {
	c, e := Client(ctx, path)
	if e != nil {
		return e
	}
	defer c.Close()
	v, e := call(ctx, c, "create_project", map[string]any{"name": "Cross-agent demonstration", "repository": "https://github.com/thothmemory-example/demo.git"})
	if e != nil {
		return e
	}
	project := v.(map[string]any)["id"].(string)
	fmt.Println("Demo project:", project)
	head := strings.Repeat("a", 40)
	var task string
	for i, agent := range []string{"claude-code", "codex", "opencode", "claude-code"} {
		session, e := Client(ctx, path)
		if e != nil {
			return e
		}
		op := fmt.Sprintf("demo-%s-%d", operation(), i)
		s, e := call(ctx, session, "start_session", map[string]any{"project_id": project, "operation_id": op + "-start", "agent": agent, "git_revision": head})
		if e != nil {
			session.Close()
			return e
		}
		if i == 0 {
			t, e := call(ctx, session, "create_task", map[string]any{"project_id": project, "operation_id": op + "-task", "title": "Share context across Claude, Codex and OpenCode", "git_revision": head})
			if e != nil {
				session.Close()
				return e
			}
			task = t.(map[string]any)["id"].(string)
		} else {
			if _, e = call(ctx, session, "get_task", map[string]any{"project_id": project, "task_id": task}); e != nil {
				session.Close()
				return e
			}
		}
		_, e = call(ctx, session, "append_event", map[string]any{"project_id": project, "operation_id": op + "-event", "task_id": task, "agent": agent, "body": "Simulated " + agent + " retrieved shared task through real MCP transport."})
		if e == nil {
			_, e = call(ctx, session, "finish_session", map[string]any{"project_id": project, "operation_id": op + "-finish", "session_id": s.(map[string]any)["id"], "expected_revision": 1, "git_revision": head, "agent": agent, "body": "Simulated " + agent + " completed protocol connectivity check. No code correctness claimed. Next agent: retrieve task " + task})
		}
		session.Close()
		if e != nil {
			return e
		}
		fmt.Println(agent + ": session, task retrieval/write, handoff OK")
	}
	return nil
}
