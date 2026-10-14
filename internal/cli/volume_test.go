package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFreshIdentityIsUniqueAndNeverTheHistoricalName(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, v := freshIdentity()
		if !projectName.MatchString(p) || !volumeName.MatchString(v) {
			t.Fatalf("invalid identity %q %q", p, v)
		}
		if p == "thothmemory" || v == "thothmemory_memory" || v == "thothmemory" || seen[v] {
			t.Fatalf("collides with a historical or earlier name: %q %q", p, v)
		}
		seen[v] = true
	}
}

func TestUpsertEnvPreservesOtherLines(t *testing.T) {
	in := "THOTH_IMAGE=x\nTHOTH_PORT=7319\n"
	out := upsertEnv(upsertEnv(in, "THOTH_VOLUME", "v"), "THOTH_VOLUME", "w")
	if out != in+"THOTH_VOLUME=w\n" {
		t.Fatalf("%q", out)
	}
	if v, ok := envValue(out, "THOTH_VOLUME"); !ok || v != "w" {
		t.Fatal(v, ok)
	}
	if _, ok := envValue(out, "MISSING"); ok {
		t.Fatal("found a missing key")
	}
}

func inTempDir(t *testing.T) {
	t.Helper()
	old, _ := os.Getwd()
	if e := os.Chdir(t.TempDir()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestInitClientsWritesIsolatedIdentityOnlyForFreshFolders(t *testing.T) {
	inTempDir(t)
	if e := initClients(); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(".env")
	first := string(b)
	if p, ok := envValue(first, "COMPOSE_PROJECT_NAME"); !ok || p == "thothmemory" {
		t.Fatalf("project: %q\n%s", p, first)
	}
	if v, ok := envValue(first, "THOTH_VOLUME"); !ok || v == "thothmemory_memory" {
		t.Fatalf("volume: %q", v)
	}
	if e := initClients(); e != nil { // idempotent: identity is kept
		t.Fatal(e)
	}
	if b, _ = os.ReadFile(".env"); string(b) != first {
		t.Fatal(".env changed on repeat")
	}
}

func TestInitClientsRefusesToPickAVolumeForALegacyEnv(t *testing.T) {
	inTempDir(t)
	legacy := "THOTH_IMAGE=x\nTHOTH_PORT=7319\nTHOTH_DATABASE=thoth\n"
	if e := os.WriteFile(".env", []byte(legacy), 0o600); e != nil {
		t.Fatal(e)
	}
	e := initClients()
	if e == nil || !strings.Contains(e.Error(), "adopt-volume") {
		t.Fatalf("expected a refusal naming adopt-volume, got %v", e)
	}
	if b, _ := os.ReadFile(".env"); string(b) != legacy {
		t.Fatal("legacy .env was modified")
	}
}

func TestComposeFileKeepsTheIsolationGuarantees(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("..", "..", "compose.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	c := string(b)
	for _, must := range []string{
		`name: "${COMPOSE_PROJECT_NAME:?`,
		`name: "${THOTH_VOLUME:?`,
		"service_completed_successfully",
		"guard:",
		"external: true",
		"REFUSING",
	} {
		if !strings.Contains(c, must) {
			t.Errorf("compose.yaml lost %q", must)
		}
	}
	if regexp.MustCompile(`(?m)^name:\s*thothmemory\s*$`).MatchString(c) || strings.Contains(c, "name: thothmemory_memory") {
		t.Error("compose.yaml hard-codes the historical project or volume name")
	}
	if n := strings.Count(c, GuardImage); n != 2 {
		t.Errorf("guard and db must both use the pinned image %s (found %d)", GuardImage, n)
	}
}

func TestAdoptVolumeRejectsBadInputBeforeTouchingDocker(t *testing.T) {
	for _, args := range [][]string{{}, {"--confirm", "x"}, {"bad name"}, {"v", "--project", "UPPER"}, {"v", "extra", "arg"}} {
		if e := AdoptVolume(t.Context(), args); e == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
