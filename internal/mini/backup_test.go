package mini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
