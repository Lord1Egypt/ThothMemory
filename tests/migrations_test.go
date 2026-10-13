package tests

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/Lord1Egypt/ThothMemory/internal/store"
	"github.com/Lord1Egypt/ThothMemory/migrations"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUpgradePreservesExistingData(t *testing.T) {
	dsn := os.Getenv("THOTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("database unset; upgrade NOT executed")
	}
	ctx := context.Background()
	admin, e := store.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Pool.Close()
	schema := fmt.Sprintf("upgrade_%d", time.Now().UnixNano())
	if _, e = admin.Pool.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, e := store.Open(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Pool.Close()
	b, _ := migrations.Files.ReadFile("001_initial.sql")
	if _, e = s.Pool.Exec(ctx, string(b)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "CREATE TABLE schema_migrations(version text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES('001_initial.sql',$1)", fmt.Sprintf("%x", sha256.Sum256(b))); e != nil {
		t.Fatal(e)
	}
	var id string
	if e = s.Pool.QueryRow(ctx, "INSERT INTO projects(name,repository) VALUES('preserve me','github.com/tests/upgrade') RETURNING id::text").Scan(&id); e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	var name string
	var budget int
	if e = s.Pool.QueryRow(ctx, "SELECT name,event_budget FROM projects WHERE id=$1", id).Scan(&name, &budget); e != nil || name != "preserve me" || budget != 100000 {
		t.Fatal("upgrade lost data", name, budget, e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE schema_migrations SET checksum='corrupt' WHERE version='001_initial.sql'"); e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(ctx); e == nil {
		t.Fatal("checksum corruption accepted")
	}
}

// A collision partway through migration 005 must roll back all preceding DDL.
func TestAuditPatchMigrationAtomicity(t *testing.T) {
	dsn := os.Getenv("THOTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("database unset; migration rollback NOT executed")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Pool.Close()
	schema := fmt.Sprintf("patch_%d", time.Now().UnixNano())
	if _, err = admin.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Pool.Close()
	if _, err = s.Pool.Exec(ctx, "CREATE TABLE schema_migrations(version text PRIMARY KEY,checksum text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	files, _ := migrations.Files.ReadDir(".")
	for _, f := range files {
		if f.Name() >= "005" {
			continue
		}
		b, _ := migrations.Files.ReadFile(f.Name())
		if _, err = s.Pool.Exec(ctx, string(b)); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Pool.Exec(ctx, "INSERT INTO schema_migrations VALUES($1,$2)", f.Name(), fmt.Sprintf("%x", sha256.Sum256(b))); err != nil {
			t.Fatal(err)
		}
	}

	principal := store.Principal{ID: "migration-fixture", Write: true, Projects: []string{"*"}}
	if err = s.Register(ctx, principal); err != nil {
		t.Fatal(err)
	}
	project := auditProject(t, s, principal, "patch-upgrade")
	task, err := s.Call(ctx, principal, "create_task", store.Input{ProjectID: project, OperationID: "upgrade-task-create", Title: "SIMULATED preserve revision"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Call(ctx, principal, "update_task", store.Input{ProjectID: project, OperationID: "upgrade-task-active", TaskID: id(task), ExpectedRevision: 1, State: "active"}); err != nil {
		t.Fatal(err)
	}
	session, err := s.Call(ctx, principal, "start_session", store.Input{ProjectID: project, OperationID: "upgrade-session-start", Agent: "simulated-claude", GitRevision: strings.Repeat("a", 40)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Call(ctx, principal, "checkpoint_session", store.Input{ProjectID: project, OperationID: "upgrade-session-checkpoint", SessionID: id(session), ExpectedRevision: 1, GitRevision: strings.Repeat("a", 40), Body: "SIMULATED durable Arabic checkpoint إعدادات الاتصال"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Call(ctx, principal, "attach_evidence", store.Input{ProjectID: project, OperationID: "upgrade-reported-evidence", TaskID: id(task), SourceKind: "test", Outcome: "fail", GitRevision: strings.Repeat("a", 40), Locator: "test://simulated/migration", Body: "SIMULATED failed attempt"}); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		out := map[string]string{}
		for _, table := range []string{"projects", "principals", "agents", "tasks", "sessions", "records", "evidence", "events", "operations"} {
			var hash string
			query := "SELECT md5(coalesce(string_agg(v::text,E'\\n' ORDER BY v::text),'')) FROM (SELECT to_jsonb(t)-'search_vector'-'search_text'-'search_simple' AS v FROM " + table + " t) rows"
			if e := s.Pool.QueryRow(ctx, query).Scan(&hash); e != nil {
				t.Fatal(e)
			}
			out[table] = hash
		}
		return out
	}
	before := snapshot()
	if _, err = s.Pool.Exec(ctx, "ALTER TABLE events ADD COLUMN search_simple text"); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err == nil {
		t.Fatal("collision did not abort migration")
	}
	var leaked bool
	if err = s.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=$1 AND table_name='records' AND column_name='search_text')", schema).Scan(&leaked); err != nil || leaked {
		t.Fatal("partial migration committed", err)
	}
	var count int
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 4 {
		t.Fatal("migration history changed", count, err)
	}
	if _, err = s.Pool.Exec(ctx, "ALTER TABLE events DROP COLUMN search_simple"); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("patch migration changed IDs, revisions, provenance or history")
	}
}
