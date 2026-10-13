// Command thothmemory-mini is ThothMemory Mini: shared agent memory backed by
// a private MariaDB, sized for ThothDock on Android.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/cli"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/mini"
)

const usage = `ThothMemory Mini %s (MariaDB)
Container:  run | healthcheck | status
Owner, inside the container (docker exec):
            attest PROJECT EVIDENCE OPERATION STATEMENT
            backup > FILE            (prints sha256 to stderr)
            restore --sha256 HEX < FILE
            activate DATABASE        (takes effect at next start)
            export PROJECT > FILE    (portable, checksummed project export)
            import < FILE            (fast-forward only; never overwrites newer state)
            token PRINCIPAL > FILE   (for client configuration)
            self-export > FILE       (writes this executable)
Clients:    doctor | bridge | connect | instructions | hooks | lifecycle | demo [--config client.json]
`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx, os.Args[1:]); e != nil {
		slog.Error("operation failed", "error", memory.SafeError(e))
		os.Exit(1)
	}
}

// interactive reports whether f is a terminal; secrets and binaries are
// only written to pipes or files so they never land in terminal scrollback.
func interactive(f *os.File) bool {
	st, e := f.Stat()
	return e == nil && st.Mode()&os.ModeCharDevice != 0
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Printf(usage, memory.MiniVersion)
		return nil
	}
	cmd := args[0]
	config, rest := cli.SplitConfig(args[1:])
	if ok, e := cli.ClientCommand(ctx, cmd, config, rest); ok {
		return e
	}
	l := mini.FromEnv()
	switch cmd {
	case "run":
		return mini.Run(ctx, l)
	case "healthcheck":
		addr := os.Getenv("THOTH_LISTEN")
		if addr == "" {
			addr = "127.0.0.1:7319"
		}
		if host, port, e := net.SplitHostPort(addr); e == nil && (host == "" || host == "0.0.0.0") {
			addr = "127.0.0.1:" + port
		}
		c := http.Client{Timeout: 3 * time.Second}
		r, e := c.Get("http://" + addr + "/healthz")
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return errors.New("not ready")
		}
		return nil
	case "status":
		return status(ctx, l)
	case "attest":
		if len(rest) != 4 {
			return errors.New("attest PROJECT_ID EVIDENCE_ID OPERATION_ID STATEMENT")
		}
		s, e := l.Owner(ctx)
		if e != nil {
			return e
		}
		defer s.Close()
		p := memory.Principal{ID: "local-owner", Owner: true, Write: true, Projects: []string{"*"}}
		if e = s.Register(ctx, p); e != nil {
			return e
		}
		v, e := s.Call(ctx, p, "attest_evidence", memory.Input{ProjectID: rest[0], EvidenceID: rest[1], OperationID: rest[2], Body: rest[3]})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "export", "import":
		s, e := l.Owner(ctx)
		if e != nil {
			return e
		}
		defer s.Close()
		return cli.Portable(ctx, s, cmd, rest)
	case "backup":
		if interactive(os.Stdout) {
			return errors.New("redirect the backup to a file: backup > thothmini.sql")
		}
		sum, e := mini.Backup(ctx, l, os.Stdout)
		if e == nil {
			fmt.Fprintln(os.Stderr, "sha256="+sum)
		}
		return e
	case "restore":
		if len(rest) != 2 || rest[0] != "--sha256" {
			return errors.New("restore --sha256 HEX < BACKUP.sql (restores into a new database)")
		}
		name, e := mini.Restore(ctx, l, os.Stdin, rest[1])
		if e == nil {
			fmt.Fprintf(os.Stderr, "Inspect it, then activate with: activate %s and restart the container.\n", name)
		}
		return e
	case "activate":
		if len(rest) != 1 {
			return errors.New("activate DATABASE")
		}
		return mini.Activate(ctx, l, rest[0])
	case "token":
		if len(rest) != 1 {
			return errors.New("token PRINCIPAL > FILE")
		}
		if interactive(os.Stdout) {
			return errors.New("refusing to print a credential to a terminal; redirect to a private file")
		}
		var ps []memory.Principal
		b, e := os.ReadFile(filepath.Join(l.Secrets(), "tokens.json"))
		if e == nil {
			e = json.Unmarshal(b, &ps)
		}
		if e != nil {
			return errors.New("token policy unavailable; run inside the Mini container")
		}
		for _, p := range ps {
			if p.ID == rest[0] {
				_, e = fmt.Fprint(os.Stdout, p.Token)
				return e
			}
		}
		return errors.New("unknown or revoked principal")
	case "self-export":
		if interactive(os.Stdout) {
			return errors.New("redirect to a file: self-export > thothmemory-mini")
		}
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		f, e := os.Open(exe)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = io.Copy(os.Stdout, f)
		return e
	case "version":
		fmt.Println(memory.MiniVersion)
		return nil
	}
	return fmt.Errorf("unknown command %q", cmd)
}

func status(ctx context.Context, l mini.Layout) error {
	s, e := l.Owner(ctx)
	if e != nil {
		return e
	}
	defer s.Close()
	out := map[string]any{"edition": "mini", "version": memory.MiniVersion, "database": l.Database()}
	if l.Addr != "" {
		out["database_transport"] = "tcp " + l.Addr
	} else {
		out["database_transport"] = "unix socket " + l.Socket() + " (networking disabled)"
	}
	if v, e := s.SchemaVersion(ctx); e == nil {
		out["schema"] = v
	}
	var server string
	_ = s.DB.QueryRowContext(ctx, "SELECT VERSION()").Scan(&server)
	out["mariadb"] = server
	counts := map[string]int{}
	for _, t := range []string{"projects", "tasks", "sessions", "events", "records", "evidence", "attestations", "imports"} {
		var n int
		if e = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); e != nil {
			return e
		}
		counts[t] = n
	}
	out["counts"] = counts
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
