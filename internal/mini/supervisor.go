// Package mini runs ThothMemory Mini as one container: the Go service
// supervises a private mariadbd that has networking disabled and is reached
// only through a Unix socket inside the container. Nothing polls while idle.
package mini

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/cli"
	"github.com/Lord1Egypt/ThothMemory/internal/gateway"
	"github.com/Lord1Egypt/ThothMemory/internal/mariadb"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
)

const (
	ownerUser   = "root"
	runtimeUser = "thoth_app"
)

var databaseName = regexp.MustCompile(`^thoth(_restore_[0-9]{8}_[0-9]{6})?$`)

// Layout locates persistent data (one named volume) and runtime files.
type Layout struct {
	Data, Run, Config string
	// Addr, when set, makes mariadbd listen on this loopback TCP address
	// instead of the Unix socket, for runtimes whose socket paths fail.
	Addr string
}

func FromEnv() Layout {
	l := Layout{Data: env("THOTH_DATA", "/data"), Run: env("THOTH_RUN", "/run/thothmini"), Config: env("THOTH_DB_CONFIG", "/etc/thothmini/my.cnf"), Addr: os.Getenv("THOTH_DB_TCP")}
	return l
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func (l Layout) Secrets() string { return filepath.Join(l.Data, "secrets") }
func (l Layout) DataDir() string { return filepath.Join(l.Data, "mariadb") }
func (l Layout) Socket() string  { return filepath.Join(l.Run, "db.sock") }

func (l Layout) secret(name string) (string, error) {
	b, e := os.ReadFile(filepath.Join(l.Secrets(), name))
	return strings.TrimSpace(string(b)), e
}

// Database is the active database: "thoth", or a restored copy the owner
// explicitly activated. Restores never overwrite the original.
func (l Layout) Database() string {
	b, e := os.ReadFile(filepath.Join(l.Data, "active-database"))
	if name := strings.TrimSpace(string(b)); e == nil && databaseName.MatchString(name) {
		return name
	}
	return "thoth"
}

func (l Layout) config(user, password string) mariadb.Config {
	c := mariadb.Config{User: user, Password: password, Database: l.Database()}
	if l.Addr != "" {
		c.Addr = l.Addr
	} else {
		c.Socket = l.Socket()
	}
	return c
}

// Owner opens the privileged login used for migrations, attestation, export
// and import. It is only ever used from inside the container.
func (l Layout) Owner(ctx context.Context) (*mariadb.Store, error) {
	pw, e := l.secret("db_owner_password")
	if e != nil {
		return nil, errors.New("owner credential unavailable; run inside the Mini container")
	}
	return mariadb.Open(ctx, l.config(ownerUser, pw))
}

func (l Layout) serverArgs() []string {
	args := []string{"--defaults-file=" + l.Config, "--datadir=" + l.DataDir(), "--pid-file=" + filepath.Join(l.Run, "mariadbd.pid")}
	if l.Addr != "" {
		host, port, _ := strings.Cut(l.Addr, ":")
		return append(args, "--skip-networking=0", "--bind-address="+host, "--port="+port, "--socket="+l.Socket())
	}
	return append(args, "--socket="+l.Socket())
}

// install creates the system tables in a scratch directory and renames it
// into place, so an interrupted first start never leaves a partial datadir.
func (l Layout) install(ctx context.Context) error {
	if _, e := os.Stat(filepath.Join(l.DataDir(), "mysql")); e == nil {
		return nil
	}
	tmp := l.DataDir() + ".installing"
	if e := os.RemoveAll(tmp); e != nil {
		return e
	}
	if e := os.MkdirAll(tmp, 0700); e != nil {
		return e
	}
	slog.Info("initializing MariaDB data directory")
	cmd := exec.CommandContext(ctx, "mariadb-install-db", "--defaults-file="+l.Config, "--datadir="+tmp, "--auth-root-authentication-method=normal", "--skip-test-db", "--skip-name-resolve", "--force")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if e := cmd.Run(); e != nil {
		return fmt.Errorf("mariadb-install-db failed: %w", e)
	}
	return os.Rename(tmp, l.DataDir())
}

// Run initializes on first start, then supervises mariadbd and the service
// until ctx ends or either process fails.
func Run(ctx context.Context, l Layout) error {
	if e := cli.InitSecrets(l.Secrets()); e != nil {
		return e
	}
	if e := os.MkdirAll(l.Run, 0700); e != nil {
		return e
	}
	if e := l.install(ctx); e != nil {
		return e
	}
	db := exec.Command("mariadbd", l.serverArgs()...)
	db.Stdout, db.Stderr = os.Stderr, os.Stderr
	if e := db.Start(); e != nil {
		return fmt.Errorf("start mariadbd: %w", e)
	}
	exited := make(chan error, 1)
	go func() { exited <- db.Wait() }()
	defer stop(db, exited)

	ownerPW, e := l.secret("db_owner_password")
	if e != nil {
		return e
	}
	if e = bootstrap(ctx, l, ownerPW, exited); e != nil {
		return e
	}
	owner, e := waitOpen(ctx, l.config(ownerUser, ownerPW), exited)
	if e != nil {
		return e
	}
	if e = owner.Migrate(ctx); e != nil {
		owner.Close()
		return e
	}
	appPW, e := l.secret("db_app_password")
	if e == nil {
		e = owner.GrantRuntime(ctx, runtimeUser, appPW)
	}
	owner.Close()
	if e != nil {
		return e
	}
	app, e := mariadb.Open(ctx, l.config(runtimeUser, appPW))
	if e != nil {
		return e
	}
	defer app.Close()
	ps, e := gateway.LoadCredentials(filepath.Join(l.Secrets(), "tokens.json"))
	if e != nil {
		return e
	}
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- gateway.Serve(serveCtx, app, ps, env("THOTH_LISTEN", "127.0.0.1:7319")) }()
	select {
	case e = <-served:
		return e
	case e = <-exited:
		exited <- e
		cancel()
		<-served
		return fmt.Errorf("mariadbd exited unexpectedly: %v", e)
	}
}

// bootstrap replaces the empty install-time root password, creates the
// database and removes every other account mariadb-install-db created. A
// marker records completion; an interrupted bootstrap is finished on restart.
func bootstrap(ctx context.Context, l Layout, pw string, exited chan error) error {
	marker := filepath.Join(l.Secrets(), "mariadb-bootstrapped")
	if _, e := os.Stat(marker); e == nil {
		return nil
	}
	c := l.config(ownerUser, "")
	c.Database = ""
	s, e := waitOpen(ctx, c, exited)
	if accessDenied(e) {
		c.Password = pw
		s, e = waitOpen(ctx, c, exited)
	}
	if e != nil {
		return e
	}
	defer s.Close()
	if e = s.Bootstrap(ctx, "thoth", pw); e != nil {
		return e
	}
	return os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0600)
}

func accessDenied(e error) bool {
	var de *memory.DatabaseError
	return errors.As(e, &de) && de.State == "28000"
}

// waitOpen retries only during startup, while mariadbd creates its socket.
func waitOpen(ctx context.Context, c mariadb.Config, exited chan error) (*mariadb.Store, error) {
	deadline := time.Now().Add(120 * time.Second)
	for {
		s, e := mariadb.Open(ctx, c)
		if e == nil || accessDenied(e) {
			return s, e
		}
		if time.Now().After(deadline) {
			return nil, errors.New("MariaDB did not become ready within 120 s")
		}
		select {
		case err := <-exited:
			exited <- err
			return nil, fmt.Errorf("mariadbd exited during startup: %v", err)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// stop asks mariadbd for a clean InnoDB shutdown and waits for it; only a
// server that ignores SIGTERM for 60 seconds is killed.
func stop(db *exec.Cmd, exited chan error) {
	select {
	case e := <-exited:
		exited <- e
		return
	default:
	}
	_ = db.Process.Signal(syscall.SIGTERM)
	select {
	case e := <-exited:
		exited <- e
		slog.Info("MariaDB stopped cleanly")
	case <-time.After(60 * time.Second):
		_ = db.Process.Kill()
		e := <-exited
		exited <- e
		slog.Error("MariaDB did not stop within 60 s and was killed; InnoDB will recover at next start")
	}
}
