package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// GuardImage must equal the image of the compose.yaml `guard` and `db` services (a test checks it).
const GuardImage = "postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873"

var (
	volumeName  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	projectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

// freshIdentity returns a unique Compose project name and data-volume name for a new installation.
func freshIdentity() (project, volume string) {
	var b [6]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	return "thothmemory-" + id, "thothmemory-" + id + "-data"
}

// envValue returns the last KEY=value in dotenv text, or "" and false.
func envValue(text, key string) (string, bool) {
	v, ok := "", false
	for _, line := range strings.Split(text, "\n") {
		if k, val, found := strings.Cut(strings.TrimRight(line, "\r"), "="); found && k == key {
			v, ok = val, true
		}
	}
	return v, ok
}

// upsertEnv sets key=value, appending when absent. Other lines are preserved byte for byte.
func upsertEnv(text, key, value string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if k, _, found := strings.Cut(line, "="); found && k == key {
			lines[i] = key + "=" + value
			return strings.Join(lines, "\n")
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + key + "=" + value + "\n"
}

func docker(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), e
}

// AdoptVolume deliberately attaches an EXISTING data volume (an older installation) to this folder.
// It never creates, deletes or migrates anything: it checks the volume, marks it as owned by this
// installation, and records it in .env. The migration happens later, when the owner runs setup.
func AdoptVolume(ctx context.Context, args []string) error {
	var name, confirm, project string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--confirm" && i+1 < len(args):
			confirm = args[i+1]
			i++
		case args[i] == "--project" && i+1 < len(args):
			project = args[i+1]
			i++
		case name == "" && !strings.HasPrefix(args[i], "-"):
			name = args[i]
		default:
			return errors.New("usage: adopt-volume VOLUME --confirm VOLUME [--project PROJECT]")
		}
	}
	if !volumeName.MatchString(name) {
		return errors.New("usage: adopt-volume VOLUME --confirm VOLUME [--project PROJECT]")
	}
	if project == "" {
		project = "thothmemory"
	}
	if !projectName.MatchString(project) {
		return errors.New("invalid project name")
	}
	if out, e := docker(ctx, "volume", "inspect", "--format", "{{.Name}}", name); e != nil || out != name {
		return fmt.Errorf("volume %q does not exist; nothing to adopt", name)
	}
	if confirm != name {
		return fmt.Errorf("refusing: this attaches the existing volume %q to this folder and the next setup will run migrations on it.\nBack it up first (docs/UPGRADE.md), then repeat with: adopt-volume %s --confirm %s", name, name, name)
	}
	env, e := os.ReadFile(".env")
	if e != nil {
		return errors.New(".env not found: run this inside the existing installation folder")
	}
	text := string(env)
	if v, ok := envValue(text, "THOTH_VOLUME"); ok && v != name {
		return fmt.Errorf("this folder already uses volume %q; refusing to change it", v)
	}
	if out, _ := docker(ctx, "ps", "--filter", "volume="+name, "--filter", "status=running", "--format", "{{.Names}}"); out != "" {
		return fmt.Errorf("volume %q is in use by running container(s): %s; stop them first", name, strings.ReplaceAll(out, "\n", ", "))
	}
	want := "thothmemory-volume-v1 " + name + " " + project
	// Writes the marker only into a PostgreSQL data layout that carries no marker yet.
	script := `m=/v/.thothmemory-volume; if [ -f "$m" ]; then [ "$(cat "$m")" = "$1" ] && echo already || { echo owned-by-another-installation; exit 3; }; exit 0; fi
[ -n "$(find /v -maxdepth 3 -name PG_VERSION 2>/dev/null | head -1)" ] || { echo not-a-data-volume; exit 4; }
echo "$1" > "$m" && echo marked`
	out, e := docker(ctx, "run", "--rm", "--user", "postgres", "--network", "none", "-v", name+":/v", "--entrypoint", "sh", GuardImage, "-ec", script, "sh", want)
	if e != nil {
		return fmt.Errorf("volume %q was not adopted (%s)", name, strings.TrimSpace(out))
	}
	text = upsertEnv(upsertEnv(text, "COMPOSE_PROJECT_NAME", project), "THOTH_VOLUME", name)
	if e = os.WriteFile(".env", []byte(text), 0o600); e != nil {
		return e
	}
	fmt.Printf("Volume %s (%s) is now this installation's data volume under project %s.\nNo migration has run. Review .env, then run setup to upgrade it.\n", name, out, project)
	return nil
}
