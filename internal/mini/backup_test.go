package mini

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A non-secret placeholder built at run time, not a literal credential.
var fixturePassword = "fixture-" + strings.Repeat("x", 8)

func layoutWithSecret(t *testing.T, addr string) Layout {
	t.Helper()
	l := Layout{Data: t.TempDir(), Run: t.TempDir(), Addr: addr}
	if e := os.MkdirAll(l.Secrets(), 0o700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(l.Secrets(), "db_owner_password"), []byte(fixturePassword+"\n"), 0o600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(l.Data, "install-id"), []byte(strings.Repeat("a", 32)+"\n"), 0o600); e != nil {
		t.Fatal(e)
	}
	return l
}

func clientDefaultsText(t *testing.T, l Layout) string {
	t.Helper()
	name, e := l.clientDefaults()
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(name)
	if fi, e := os.Stat(name); e != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("option file must be private (0600): %v %v", fi, e)
	}
	b, e := os.ReadFile(name)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

// Regression: native mariadb-dump/mariadb over the private Unix socket must not
// negotiate TLS, or a client clock behind the server's per-start certificate
// fails the backup with "certificate is not yet valid".
func TestClientDefaultsSocketDisablesTLS(t *testing.T) {
	t.Setenv("THOTH_DB_SOCKET", "")
	got := clientDefaultsText(t, layoutWithSecret(t, ""))
	if !strings.Contains(got, "\nskip-ssl\n") || !strings.Contains(got, "\nsocket=@thothmemory-mini-") {
		t.Fatalf("socket client options must disable TLS on the socket:\n%s", redact(got))
	}
}

// The TCP fallback must keep TLS and certificate verification untouched.
func TestClientDefaultsTCPKeepsTLS(t *testing.T) {
	got := clientDefaultsText(t, layoutWithSecret(t, "127.0.0.1:33306"))
	for _, weak := range []string{"skip-ssl", "ssl=0", "ssl-verify-server-cert=0", "skip-ssl-verify-server-cert", "ssl=off"} {
		if strings.Contains(got, weak) {
			t.Fatalf("TCP client options must not weaken TLS (%s):\n%s", weak, redact(got))
		}
	}
	if !strings.Contains(got, "protocol=tcp") {
		t.Fatalf("expected TCP options:\n%s", redact(got))
	}
}

func redact(s string) string { return strings.ReplaceAll(s, fixturePassword, "<redacted>") }

func TestDiscardPartialReportsDroppedOnlyWhenDropSucceeds(t *testing.T) {
	var gotName string
	var deadline time.Time
	var hasDeadline bool
	drop := func(c context.Context, db string) error {
		gotName = db
		deadline, hasDeadline = c.Deadline()
		return nil
	}
	parent, cancel := context.WithCancel(context.Background())
	cancel() // cleanup must still run when the caller's context is already cancelled
	e := discardPartial(parent, drop, "thoth_restore_20261014_000000", "restore load failed", errors.New("exit status 1"))
	if gotName != "thoth_restore_20261014_000000" {
		t.Fatalf("dropped %q", gotName)
	}
	if !hasDeadline || time.Until(deadline) > cleanupTimeout {
		t.Fatalf("cleanup must be bounded by %v (deadline=%v)", cleanupTimeout, deadline)
	}
	msg := e.Error()
	if !strings.Contains(msg, "exit status 1") || !strings.Contains(msg, "was dropped") || strings.Contains(msg, "FAILED") {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestDiscardPartialNeverClaimsDroppedWhenDropFails(t *testing.T) {
	drop := func(context.Context, string) error { return errors.New("lock wait timeout") }
	e := discardPartial(context.Background(), drop, "thoth_restore_20261014_000000", "restore load failed", errors.New("syntax error near line 1"))
	msg := e.Error()
	for _, want := range []string{"syntax error near line 1", "lock wait timeout", "cleanup FAILED", "thoth_restore_20261014_000000", "NOT be activated"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message lacks %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "was dropped") {
		t.Fatalf("claimed a drop that failed: %s", msg)
	}
}

func TestDiscardPartialRefusesToTouchTheActiveDatabase(t *testing.T) {
	for _, name := range []string{"thoth", "mysql", "thoth_restore_x", "thoth`; DROP DATABASE thoth; --"} {
		called := false
		e := discardPartial(context.Background(), func(context.Context, string) error { called = true; return nil }, name, "x", errors.New("y"))
		if called || e == nil || !strings.Contains(e.Error(), "not a restore database") {
			t.Fatalf("%q: called=%v err=%v", name, called, e)
		}
	}
}

func TestDiscardPartialStopsAtTheBound(t *testing.T) {
	old := cleanupTimeoutForTest(50 * time.Millisecond)
	defer cleanupTimeoutForTest(old)
	drop := func(c context.Context, _ string) error { <-c.Done(); return c.Err() }
	start := time.Now()
	e := discardPartial(context.Background(), drop, "thoth_restore_20261014_000000", "x", errors.New("y"))
	if time.Since(start) > 5*time.Second || !strings.Contains(e.Error(), "cleanup FAILED") {
		t.Fatalf("not bounded or not reported: %v after %v", e, time.Since(start))
	}
}

func cleanupTimeoutForTest(d time.Duration) time.Duration {
	old := cleanupTimeout
	cleanupTimeout = d
	return old
}

func TestRecordShutdownAppendsAndStaysBounded(t *testing.T) {
	l := Layout{Data: t.TempDir()}
	for i := 0; i < 60; i++ {
		l.recordShutdown("clean")
	}
	l.recordShutdown("error: exit status 1")
	b, e := os.ReadFile(l.ShutdownLog())
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 50 || !strings.HasSuffix(lines[49], " error: exit status 1") || !strings.HasSuffix(lines[48], " clean") {
		t.Fatalf("%d lines, tail %q", len(lines), lines[len(lines)-2:])
	}
}
