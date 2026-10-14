package mini

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// clientDefaults writes a private option file so the owner password never
// appears in a process argument list; the caller removes it.
func (l Layout) clientDefaults() (string, error) {
	pw, e := l.secret("db_owner_password")
	if e != nil {
		return "", errors.New("owner credential unavailable; run inside the Mini container")
	}
	f, e := os.CreateTemp(l.Run, ".client-*.cnf")
	if e != nil {
		return "", e
	}
	// The Unix socket is reachable only within the container's network namespace and the
	// password is still required. That is not a strong isolation boundary (ThothDock can
	// share networking and UID), so authentication is what protects the database.
	// MariaDB 11.4 clients still negotiate TLS over it against a certificate the
	// server generates at each start, valid from that second; a client clock that
	// reads earlier (VM/WSL2 time steps, emulation) fails with "certificate is not
	// yet valid". Disabling TLS here is scoped to the socket branch only: the TCP
	// branch below keeps the client defaults, including certificate verification.
	conn := "skip-ssl\nsocket=" + l.Socket()
	if l.Addr != "" {
		host, port, _ := strings.Cut(l.Addr, ":")
		conn = "protocol=tcp\nhost=" + host + "\nport=" + port
	}
	_, e = fmt.Fprintf(f, "[client]\nuser=%s\npassword=%s\n%s\n", ownerUser, pw, conn)
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(f.Name())
		return "", e
	}
	return f.Name(), nil
}

// Backup streams a consistent logical dump of the active database to w and
// returns its SHA-256. --single-transaction reads one InnoDB snapshot without
// blocking writers; triggers are included so restored history stays immutable.
func Backup(ctx context.Context, l Layout, w io.Writer) (string, error) {
	cnf, e := l.clientDefaults()
	if e != nil {
		return "", e
	}
	defer os.Remove(cnf)
	h := sha256.New()
	cmd := exec.CommandContext(ctx, "mariadb-dump", "--defaults-extra-file="+cnf, "--single-transaction", "--triggers", "--routines", "--hex-blob", "--skip-comments", "--no-tablespaces", l.Database())
	cmd.Stdout = io.MultiWriter(w, h)
	cmd.Stderr = os.Stderr
	if e = cmd.Run(); e != nil {
		return "", errors.New("mariadb-dump failed")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Restore verifies r against the expected SHA-256 before touching the server,
// loads it into a NEW database and validates it. Activation is a separate,
// explicit owner step; the current database is never modified.
func Restore(ctx context.Context, l Layout, r io.Reader, want string) (string, error) {
	if len(want) != 64 {
		return "", errors.New("restore requires --sha256 with the backup's 64-character checksum")
	}
	spool, e := os.CreateTemp(l.Data, ".restore-*.sql")
	if e != nil {
		return "", e
	}
	defer os.Remove(spool.Name())
	defer spool.Close()
	h := sha256.New()
	if _, e = io.Copy(io.MultiWriter(spool, h), r); e != nil {
		return "", e
	}
	if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(want) {
		return "", errors.New("backup SHA-256 integrity check failed; nothing was changed")
	}
	if _, e = spool.Seek(0, 0); e != nil {
		return "", e
	}
	if e = refuseForeignStatements(spool); e != nil {
		return "", e
	}
	if _, e = spool.Seek(0, 0); e != nil {
		return "", e
	}
	name := "thoth_restore_" + time.Now().UTC().Format("20060102_150405")
	owner, e := l.Owner(ctx)
	if e != nil {
		return "", e
	}
	defer owner.Close()
	if _, e = owner.DB.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); e != nil {
		return "", errors.New("could not create restore database")
	}
	cnf, e := l.clientDefaults()
	if e != nil {
		return "", e
	}
	defer os.Remove(cnf)
	cmd := exec.CommandContext(ctx, "mariadb", "--defaults-extra-file="+cnf, "--batch", name)
	cmd.Stdin, cmd.Stderr = spool, os.Stderr
	// A failed load must not leave a half-restored database that looks like a result.
	drop := func(c context.Context, db string) error {
		_, de := owner.DB.ExecContext(c, "DROP DATABASE `"+db+"`")
		return de
	}
	if e = cmd.Run(); e != nil {
		return "", discardPartial(ctx, drop, name, "restore load failed", e)
	}
	var projects, events int
	if e = owner.DB.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM `"+name+"`.projects),(SELECT COUNT(*) FROM `"+name+"`.events)").Scan(&projects, &events); e != nil {
		return "", discardPartial(ctx, drop, name, "restored schema validation failed", e)
	}
	fmt.Fprintf(os.Stderr, "Restored into %s: projects=%d events=%d. Active database unchanged.\n", name, projects, events)
	return name, nil
}

var restoreName = regexp.MustCompile(`^thoth_restore_[0-9]{8}_[0-9]{6}$`)

var cleanupTimeout = 30 * time.Second

// discardPartial drops the half-restored database with a bounded, caller-independent
// timeout and reports the truth: the original failure is always kept, and "dropped" is
// claimed only when DROP DATABASE succeeded. It refuses any name that is not a restore
// database, so the active database can never be dropped through this path.
func discardPartial(ctx context.Context, drop func(context.Context, string) error, name, what string, cause error) error {
	if !restoreName.MatchString(name) {
		return fmt.Errorf("%s: %v; refusing to clean up %q: not a restore database", what, cause, name)
	}
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if de := drop(dctx, name); de != nil {
		return fmt.Errorf("%s: %v; cleanup FAILED (%v): partial database %s remains and must NOT be activated, drop it manually; the active database is untouched", what, cause, de, name)
	}
	return fmt.Errorf("%s: %v; the partial database %s was dropped; the active database is untouched", what, cause, name)
}

// refuseForeignStatements rejects dumps that would act outside the target
// database: a ThothMemory dump never switches databases or manages accounts.
func refuseForeignStatements(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	for sc.Scan() {
		line := strings.ToUpper(strings.TrimSpace(sc.Text()))
		for _, bad := range []string{"USE ", "CREATE DATABASE", "DROP DATABASE", "CREATE USER", "GRANT ", "DROP USER", "ALTER USER", "INSTALL ", "SET PASSWORD", "\\! ", "SYSTEM ", "SOURCE "} {
			if strings.HasPrefix(line, bad) {
				return errors.New("backup contains statements outside a single ThothMemory database; refused")
			}
		}
	}
	return sc.Err()
}

// Activate selects a restored database for the next start after checking
// that it exists and carries an applied schema.
func Activate(ctx context.Context, l Layout, name string) error {
	if !databaseName.MatchString(name) {
		return errors.New("database must be thoth or thoth_restore_YYYYMMDD_HHMMSS")
	}
	owner, e := l.Owner(ctx)
	if e != nil {
		return e
	}
	defer owner.Close()
	var n int
	if e = owner.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+name+"`.schema_migrations WHERE status='applied'").Scan(&n); e != nil || n == 0 {
		return errors.New("database missing or has no applied ThothMemory schema")
	}
	tmp := filepath.Join(l.Data, ".active-database.tmp")
	if e = os.WriteFile(tmp, []byte(name+"\n"), 0600); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(l.Data, "active-database"))
}
