package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func compose(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
}

// Resolve through Compose itself so .env, shell overrides, and Compose's precedence
// select exactly the database used by the running application configuration.
func activeDatabase(ctx context.Context) (string, error) {
	b, e := compose(ctx, "config", "--format", "json").Output()
	if e != nil {
		return "", errors.New("cannot resolve active Compose database configuration")
	}
	return databaseFromConfig(b)
}
func databaseFromConfig(b []byte) (string, error) {
	var config struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	if e := json.Unmarshal(b, &config); e != nil {
		return "", errors.New("invalid Compose database configuration")
	}
	db := config.Services["app"].Environment["THOTH_DB_NAME"]
	if db == "" {
		return "", errors.New("active application database is not configured")
	}
	return db, nil
}
func backup(ctx context.Context, path, db string) error {
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		return errors.New("backup target exists or cannot be inspected")
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if e = privatePath(path, false); e != nil {
		return e
	}
	h := sha256.New()
	cmd := compose(ctx, "exec", "-T", "db", "pg_dump", "-U", "postgres", "-Fc", "--no-owner", "--no-privileges", "-d", db)
	cmd.Stdout = io.MultiWriter(f, h)
	cmd.Stderr = os.Stderr
	if e = cmd.Run(); e != nil {
		return errors.New("pg_dump failed")
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = exclusive(path+".sha256", []byte(hex.EncodeToString(h.Sum(nil))+"\n")); e != nil {
		return e
	}
	ok = true
	fmt.Println("Backup saved: " + path + " (SHA-256 sidecar)")
	return nil
}
func BackupRestore(ctx context.Context, action string, args []string) error {
	if len(args) == 0 {
		return errors.New(action + " requires an archive path; restore also requires --confirm")
	}
	path := args[0]
	db, err := activeDatabase(ctx)
	if err != nil {
		return err
	}
	if action == "backup" {
		return backup(ctx, path, db)
	}
	if len(args) != 2 || args[1] != "--confirm" {
		return errors.New("restore creates a recovery backup and a NEW database; rerun restore ARCHIVE --confirm after reviewing")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	side, e := os.ReadFile(path + ".sha256")
	if e != nil || strings.TrimSpace(string(side)) != hex.EncodeToString(h.Sum(nil)) {
		return errors.New("backup SHA-256 integrity check failed")
	}
	if _, e = f.Seek(0, 0); e != nil {
		return e
	}
	check := compose(ctx, "exec", "-T", "db", "pg_restore", "--list")
	check.Stdin = f
	if e = check.Run(); e != nil {
		return errors.New("invalid PostgreSQL archive")
	}
	stamp := time.Now().UTC().Format("20060102_150405")
	if e = backup(ctx, path+".recovery-"+stamp, db); e != nil {
		return e
	}
	newDB := "thoth_restore_" + stamp
	cmd := compose(ctx, "exec", "-T", "db", "createdb", "-U", "postgres", newDB)
	if b, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("create restore database failed: %s", b)
	}
	if _, e = f.Seek(0, 0); e != nil {
		return e
	}
	cmd = compose(ctx, "exec", "-T", "db", "pg_restore", "-U", "postgres", "--exit-on-error", "--single-transaction", "--no-owner", "--no-privileges", "-d", newDB)
	cmd.Stdin = f
	cmd.Stderr = os.Stderr
	if e = cmd.Run(); e != nil {
		return errors.New("restore failed; original database is untouched; inspect new database before retrying")
	}
	b, e := compose(ctx, "exec", "-T", "db", "psql", "-U", "postgres", "-d", newDB, "-Atc", "SELECT 'projects=' || count(*) FROM projects UNION ALL SELECT 'events=' || count(*) FROM events").Output()
	if e != nil {
		return errors.New("restored schema validation failed")
	}
	fmt.Printf("Restored into %s\n%sOriginal database retained. To activate, set THOTH_DATABASE=%s in .env and rerun setup; retain the recovery backup.\n", newDB, b, newDB)
	return nil
}
