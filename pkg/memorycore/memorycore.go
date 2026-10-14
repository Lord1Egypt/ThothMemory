// Package memorycore embeds ThothMemory UltraLite in another Go program, such
// as an Android app's Go core: no subprocess, socket, HTTP server or daemon.
// Tool names, JSON arguments, validation, authorization and results are
// exactly those of the MCP tools (without the "memory_" prefix), so an agent
// runtime can expose them to a model unchanged.
//
// Stored memory is untrusted data written by agents. Callers must present it
// to models as data, never as instructions.
package memorycore

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Lord1Egypt/ThothMemory/internal/mcptools"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/sqlite"
	"github.com/Lord1Egypt/ThothMemory/internal/ultralite"
)

// Memory is one open UltraLite data directory. It is safe for concurrent use.
type Memory struct {
	home    ultralite.Home
	store   *sqlite.Store
	release func()
}

// Init creates (or keeps) the data directory, credentials and database.
func Init(ctx context.Context, dataDir string) error {
	return ultralite.Init(ctx, ultralite.Home{Dir: dataDir})
}

// Open opens an initialized data directory and holds its instance lock until Close.
func Open(ctx context.Context, dataDir string) (*Memory, error) {
	h := ultralite.Home{Dir: dataDir}
	s, release, e := ultralite.Open(ctx, h)
	if e != nil {
		return nil, e
	}
	return &Memory{h, s, release}, nil
}

func (m *Memory) Close() { m.release() }

// Session is an authenticated principal: the same scoped credential an MCP
// client would present. Read-only and project-restricted policies apply.
type Session struct {
	m *Memory
	p memory.Principal
}

// Session authenticates with the credential stored in tokenFile.
func (m *Memory) Session(ctx context.Context, tokenFile string) (*Session, error) {
	p, e := m.home.Principal(tokenFile)
	if e != nil {
		return nil, e
	}
	if e = m.store.Register(ctx, p); e != nil {
		return nil, e
	}
	return &Session{m, p}, nil
}

// Call runs one tool and returns {"data": ...} JSON, as MCP structured content.
// Errors carry the same sanitized messages MCP clients receive.
func (s *Session) Call(ctx context.Context, tool string, args []byte) ([]byte, error) {
	v, e := ultralite.Call(ctx, s.m.store, s.p, strings.TrimPrefix(tool, "memory_"), args)
	if e != nil {
		return nil, safe{memory.SafeError(e)}
	}
	return json.Marshal(map[string]any{"data": v})
}

type safe struct{ msg string }

func (e safe) Error() string { return e.msg }

// Tool describes one callable tool with its JSON Schema.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	ReadOnly    bool           `json:"read_only"`
	InputSchema map[string]any `json:"input_schema"`
}

// Tools lists the tools this session may call (write tools are omitted for
// read-only credentials), with the schemas MCP publishes.
func (s *Session) Tools() []Tool {
	out := []Tool{}
	for _, c := range mcptools.Contracts {
		if memory.IsWrite(c.Name) && !s.p.Write {
			continue
		}
		out = append(out, Tool{Name: c.Name, Description: c.Description, ReadOnly: !memory.IsWrite(c.Name), InputSchema: mcptools.Schema(c)})
	}
	return out
}

// Backup writes a consistent, integrity-checked snapshot of the open memory to
// target (which must not exist) and returns its SHA-256. It is safe to call
// while other goroutines use the memory.
func (m *Memory) Backup(ctx context.Context, target string) (string, error) {
	return m.store.Backup(ctx, target)
}

// Status reports storage and row counts as JSON.
func (m *Memory) Status(ctx context.Context) ([]byte, error) {
	st, e := m.store.Status(ctx)
	if e != nil {
		return nil, e
	}
	return json.Marshal(st)
}
