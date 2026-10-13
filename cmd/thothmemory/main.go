package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Lord1Egypt/ThothMemory/internal/cli"
	"github.com/Lord1Egypt/ThothMemory/internal/gateway"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/store"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func secret(name string) (string, error) {
	if p := os.Getenv(name + "_FILE"); p != "" {
		b, e := os.ReadFile(p)
		return strings.TrimSpace(string(b)), e
	}
	v := os.Getenv(name)
	if v == "" {
		return "", errors.New(name + " or " + name + "_FILE required")
	}
	return v, nil
}
func dsn() (string, error) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v, nil
	}
	pw, e := secret("THOTH_DB_PASSWORD")
	if e != nil {
		return "", e
	}
	host := os.Getenv("THOTH_DB_HOST")
	if host == "" {
		host = "db"
	}
	user := os.Getenv("THOTH_DB_USER")
	if user == "" {
		user = "thoth_app"
	}
	db := os.Getenv("THOTH_DB_NAME")
	if db == "" {
		db = "thoth"
	}
	u := url.URL{Scheme: "postgres", Host: host, Path: "/" + db, User: url.UserPassword(user, pw)}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Println("ThothMemory " + memory.FullVersion + "\nCommands: serve, migrate, bridge, doctor, connect, instructions, permissions, backup, restore, attest, lifecycle, demo\nClient options: --config <client.json>. Run connect --help for scopes.")
		return nil
	}
	cmd := args[0]
	path := "client.json"
	rest := []string{}
	for i := 1; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			i++
			path = args[i]
		} else {
			rest = append(rest, args[i])
		}
	}
	switch cmd {
	case "permissions":
		return cli.Permissions(rest)
	case "init-config":
		return cli.InitConfig()
	case "bridge":
		return cli.Bridge(ctx, path)
	case "preflight":
		return cli.Preflight(ctx, path)
	case "doctor":
		return cli.Doctor(ctx, path)
	case "connect":
		return cli.Connect(ctx, path, rest)
	case "hooks":
		return cli.Hooks(ctx, path, rest)
	case "instructions":
		return cli.Instructions(rest)
	case "backup", "restore":
		return cli.BackupRestore(ctx, cmd, rest)
	case "lifecycle":
		return cli.Lifecycle(ctx, path, rest)
	case "demo":
		return cli.Demo(ctx, path)
	case "healthcheck":
		r, e := http.Get("http://127.0.0.1:7319/healthz")
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return errors.New("not ready")
		}
		return nil
	case "serve", "migrate", "attest":
		conn, e := dsn()
		if e != nil {
			return e
		}
		s, e := store.Open(ctx, conn)
		if e != nil {
			return errors.New("database connection failed")
		}
		defer s.Pool.Close()
		if cmd == "migrate" {
			if e = s.Migrate(ctx); e != nil {
				return e
			}
			pw, e := secret("THOTH_RUNTIME_PASSWORD")
			if e != nil {
				return e
			}
			var sql string
			e = s.Pool.QueryRow(ctx, "SELECT format('CREATE ROLE thoth_app LOGIN PASSWORD %L', $1::text)", pw).Scan(&sql)
			if e != nil {
				return e
			}
			var exists bool
			if e = s.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='thoth_app')").Scan(&exists); e != nil {
				return e
			}
			if !exists {
				if _, e = s.Pool.Exec(ctx, sql); e != nil {
					return e
				}
			}
			var databaseName string
			if e = s.Pool.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); e != nil {
				return e
			}
			_, e = s.Pool.Exec(ctx, "GRANT CONNECT ON DATABASE "+`"`+strings.ReplaceAll(databaseName, `"`, `""`)+`"`+" TO thoth_app; GRANT USAGE ON SCHEMA public TO thoth_app; GRANT SELECT ON ALL TABLES IN SCHEMA public TO thoth_app; GRANT INSERT ON principals,projects,agents,sessions,tasks,task_dependencies,evidence,events,records,operations TO thoth_app; GRANT UPDATE ON projects,sessions,tasks TO thoth_app")
			return e
		}
		if cmd == "attest" {
			if len(rest) != 4 {
				return errors.New("attest PROJECT_ID EVIDENCE_ID OPERATION_ID STATEMENT; run via privileged migrate service")
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
		}
		ps, e := gateway.LoadCredentials(os.Getenv("THOTH_TOKENS_FILE"))
		if e != nil {
			return e
		}
		addr := os.Getenv("THOTH_LISTEN")
		if addr == "" {
			addr = "127.0.0.1:7319"
		}
		return gateway.Serve(ctx, s, ps, addr)
	}
	return fmt.Errorf("unknown command %q", cmd)
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx, os.Args[1:]); e != nil {
		slog.Error("operation failed", "error", memory.SafeError(e))
		os.Exit(1)
	}
}
