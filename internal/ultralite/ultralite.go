// Package ultralite is ThothMemory UltraLite: one native executable and one
// SQLite file in a per-user data directory. It is headless by design: clients
// launch it per session over MCP stdio and the owner uses the CLI. It never
// opens a network listener and does not contain the web dashboard.
package ultralite

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/cli"
	"github.com/Lord1Egypt/ThothMemory/internal/mcptools"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Home is the data directory: memory.db, .secrets/ and backups/.
type Home struct{ Dir string }

// DefaultDir is the per-user application data directory: XDG data home on
// Linux, Application Support on macOS, %LOCALAPPDATA% on Windows.
func DefaultDir() (string, error) {
	if v := os.Getenv("THOTH_HOME"); v != "" {
		return filepath.Abs(v)
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "ThothMemory"), nil
		}
		return filepath.Join(home, "AppData", "Local", "ThothMemory"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "ThothMemory"), nil
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "thothmemory"), nil
	}
	return filepath.Join(home, ".local", "share", "thothmemory"), nil
}

func (h Home) DB() string      { return filepath.Join(h.Dir, "memory.db") }
func (h Home) Secrets() string { return filepath.Join(h.Dir, ".secrets") }
func (h Home) Backups() string { return filepath.Join(h.Dir, "backups") }
func (h Home) lockPath() string {
	return filepath.Join(h.Dir, "memory.db.instance-lock")
}

// Init creates the data directory, credentials and database. It is safe to
// repeat: existing credentials, policy and memory are kept.
func Init(ctx context.Context, h Home) error {
	if e := os.MkdirAll(h.Dir, 0700); e != nil {
		return e
	}
	if e := cli.PrivateDir(h.Dir); e != nil {
		return e
	}
	if e := cli.InitSecrets(h.Secrets(), false); e != nil {
		return e
	}
	s, e := h.open(ctx, true)
	if e != nil {
		return e
	}
	defer s.Close()
	ps, e := mcptools.LoadCredentials(filepath.Join(h.Secrets(), "tokens.json"))
	if e != nil {
		return e
	}
	for _, p := range ps {
		if e = s.Register(ctx, p); e != nil {
			return e
		}
	}
	return nil
}

// open opens the database and applies migrations, which is how an upgraded
// executable upgrades an existing file. Only init may create a new file, so
// a mistyped data directory never silently starts an empty memory.
func (h Home) open(ctx context.Context, create bool) (*sqlite.Store, error) {
	if _, e := os.Stat(h.DB()); e != nil && !create {
		return nil, fmt.Errorf("no UltraLite database at %s; run: thothmemory-ultralite init", h.DB())
	}
	s, e := sqlite.Open(ctx, h.DB())
	if e != nil {
		return nil, e
	}
	if e = s.Migrate(ctx); e != nil {
		s.Close()
		return nil, e
	}
	if runtime.GOOS != "windows" {
		// SQLite creates files with the process umask; memory is owner-only.
		for _, f := range []string{h.DB(), h.DB() + "-wal", h.DB() + "-shm"} {
			if _, err := os.Stat(f); err == nil {
				if err = cli.PrivateFile(f); err != nil {
					s.Close()
					return nil, err
				}
			}
		}
	}
	return s, nil
}

// Open opens an initialized database and holds the shared instance lock until
// the returned release is called, so restore can tell the memory is in use.
func Open(ctx context.Context, h Home) (*sqlite.Store, func(), error) {
	f, e := os.OpenFile(h.lockPath(), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, nil, e
	}
	if e = lock(f, false); e != nil {
		f.Close()
		return nil, nil, e
	}
	s, e := h.open(ctx, false)
	if e != nil {
		f.Close()
		return nil, nil, e
	}
	return s, func() { s.Close(); f.Close() }, nil
}

// Principal resolves the credential in tokenFile against the token policy,
// exactly as the HTTP service authenticates a bearer token.
func (h Home) Principal(tokenFile string) (memory.Principal, error) {
	b, e := os.ReadFile(tokenFile)
	if e != nil {
		return memory.Principal{}, errors.New("token file unreadable")
	}
	token := strings.TrimSpace(string(b))
	ps, e := mcptools.LoadCredentials(filepath.Join(h.Secrets(), "tokens.json"))
	if e != nil {
		return memory.Principal{}, e
	}
	found := -1
	for i, p := range ps {
		if subtle.ConstantTimeCompare([]byte(token), []byte(p.Token)) == 1 {
			found = i
		}
	}
	if found < 0 {
		return memory.Principal{}, errors.New("unauthorized: token is not in the current policy (revoked or wrong data directory)")
	}
	return ps[found], nil
}

// Stdio serves one client over stdin/stdout and returns when it disconnects.
// The client's own scoped token selects the principal: stdio does not bypass
// project allowlists or read-only credentials.
func Stdio(ctx context.Context, h Home, tokenFile string) error {
	p, e := h.Principal(tokenFile)
	if e != nil {
		return e
	}
	s, release, e := Open(ctx, h)
	if e != nil {
		return e
	}
	defer release()
	if e = s.Register(ctx, p); e != nil {
		return e
	}
	return mcptools.Server(s, p).Run(ctx, &mcp.StdioTransport{})
}

// Backup writes a verified online snapshot and a SHA-256 sidecar.
func Backup(ctx context.Context, h Home, target string) (string, string, error) {
	s, release, e := Open(ctx, h)
	if e != nil {
		return "", "", e
	}
	defer release()
	if target == "" {
		if e = os.MkdirAll(h.Backups(), 0700); e != nil {
			return "", "", e
		}
		if e = cli.PrivateDir(h.Backups()); e != nil {
			return "", "", e
		}
		target = filepath.Join(h.Backups(), "memory-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	}
	sum, e := s.Backup(ctx, target)
	if e != nil {
		return "", "", e
	}
	return target, sum, os.WriteFile(target+".sha256", []byte(sum+"\n"), 0600)
}

// Restore replaces the database with a verified backup. It refuses while any
// UltraLite process has the memory open, keeps the replaced file beside it,
// and requires explicit confirmation.
func Restore(ctx context.Context, h Home, backup string, confirmed bool) (string, error) {
	side, e := os.ReadFile(backup + ".sha256")
	if e != nil {
		return "", errors.New("backup has no .sha256 sidecar; refusing an unverifiable restore")
	}
	sum, e := sqlite.FileSum(backup)
	if e != nil || sum != strings.TrimSpace(string(side)) {
		return "", errors.New("backup SHA-256 integrity check failed; nothing changed")
	}
	if e = sqlite.Verify(ctx, backup); e != nil {
		return "", fmt.Errorf("backup is not a healthy UltraLite database: %w", e)
	}
	if !confirmed {
		return "", errors.New("backup verified. Restore replaces the current memory (it is kept as a .before-restore copy); rerun with --confirm")
	}
	f, e := os.OpenFile(h.lockPath(), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return "", e
	}
	defer f.Close()
	if lock(f, true) != nil {
		return "", errors.New("memory is in use by another UltraLite process (an agent session or CLI command); close your agents and retry")
	}
	kept := ""
	if _, e = os.Stat(h.DB()); e == nil {
		// Fold the WAL into the main file so the kept copy is self-contained.
		s, e := sqlite.Open(ctx, h.DB())
		if e != nil {
			return "", e
		}
		_, e = s.DB.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
		s.Close()
		if e != nil {
			return "", e
		}
		kept = h.DB() + ".before-restore-" + time.Now().UTC().Format("20060102T150405Z")
		if e = os.Rename(h.DB(), kept); e != nil {
			return "", e
		}
		for _, x := range []string{"-wal", "-shm"} {
			os.Remove(h.DB() + x)
		}
	}
	tmp := h.DB() + ".restoring"
	if e = copyFile(backup, tmp); e != nil {
		return "", e
	}
	if e = os.Rename(tmp, h.DB()); e != nil {
		return "", e
	}
	if s, e := h.open(ctx, false); e != nil {
		return "", fmt.Errorf("restored file did not open; previous memory kept at %s: %w", kept, e)
	} else {
		s.Close()
	}
	return kept, nil
}

func copyFile(from, to string) error {
	in, e := os.Open(from)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = io.Copy(out, in); e != nil {
		out.Close()
		os.Remove(to)
		return e
	}
	if e = out.Sync(); e != nil {
		out.Close()
		return e
	}
	return out.Close()
}

// Status reports storage and policy facts for doctor and the dashboard.
func Status(ctx context.Context, h Home) (map[string]any, error) {
	s, release, e := Open(ctx, h)
	if e != nil {
		return nil, e
	}
	defer release()
	out := map[string]any{"edition": "ultralite", "version": memory.UltraLiteVersion, "data_dir": h.Dir}
	st, e := s.Status(ctx)
	if e != nil {
		return nil, e
	}
	for k, v := range st {
		out[k] = v
	}
	b, _ := os.ReadFile(filepath.Join(h.Secrets(), "tokens.json"))
	var ps []memory.Principal
	_ = json.Unmarshal(b, &ps)
	ids := []string{}
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	out["principals"] = ids
	return out, nil
}

// Owner is the local owner identity used by CLI inspection and attestation;
// it exists only in-process and is never accepted from a client.
var Owner = memory.Principal{ID: "local-owner", Owner: true, Write: true, Projects: []string{"*"}}

// Call runs one tool by its MCP name with JSON arguments, applying exactly the
// schema validation and authorization an MCP client gets. It is the
// transport-free boundary for CLI inspection and in-process embedding.
func Call(ctx context.Context, b memory.Backend, p memory.Principal, tool string, args []byte) (any, error) {
	for _, c := range mcptools.Contracts {
		if c.Name == tool {
			if memory.IsWrite(tool) && !p.Write {
				return nil, memory.ErrForbidden
			}
			in, e := mcptools.Decode(c, args)
			if e != nil {
				return nil, e
			}
			return memory.Call(ctx, b, p, tool, in)
		}
	}
	return nil, errors.New("unknown tool " + tool)
}
