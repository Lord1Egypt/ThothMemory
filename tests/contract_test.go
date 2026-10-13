package tests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/contract"
	"github.com/Lord1Egypt/ThothMemory/internal/mariadb"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/store"
	"github.com/jackc/pgx/v5"
)

type pgAdmin struct{ s *store.Store }

func (a pgAdmin) ExpireClaim(ctx context.Context, task string) error {
	_, e := a.s.Pool.Exec(ctx, "UPDATE tasks SET claim_expires_at=now()-interval '1 second' WHERE id=$1", task)
	return e
}
func (a pgAdmin) ExhaustBudget(ctx context.Context, project string) error {
	_, e := a.s.Pool.Exec(ctx, "UPDATE projects SET event_budget=100,event_count=100 WHERE id=$1", project)
	return e
}
func (a pgAdmin) MutateHistory(ctx context.Context, table, project string) error {
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, upd := tx.Exec(ctx, "UPDATE "+pgx.Identifier{table}.Sanitize()+" SET project_id=project_id WHERE project_id=$1", project)
	if upd == nil {
		return nil
	}
	tx.Rollback(ctx)
	tx, e = a.s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, del := tx.Exec(ctx, "DELETE FROM "+pgx.Identifier{table}.Sanitize()+" WHERE project_id=$1", project); del == nil {
		return nil
	}
	return upd
}
func (a pgAdmin) LockProject(ctx context.Context, project string) (func(), error) {
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, "SELECT 1 FROM projects WHERE id=$1 FOR UPDATE", project); e != nil {
		tx.Rollback(ctx)
		return nil, e
	}
	return func() { tx.Rollback(ctx) }, nil
}
func (a pgAdmin) CountOperations(ctx context.Context, project, op string) (int, error) {
	var n int
	e := a.s.Pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE project_id=$1 AND operation_id=$2", project, op).Scan(&n)
	return n, e
}

func postgresHarness(t *testing.T) contract.Harness {
	t.Helper()
	dsn := os.Getenv("THOTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("THOTH_TEST_DATABASE_URL unset; PostgreSQL contract NOT executed")
	}
	s, e := store.Open(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Pool.Close)
	if e = s.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	return contract.Harness{Backend: s, Admin: pgAdmin{s}}
}

type mariaAdmin struct{ s *mariadb.Store }

func (a mariaAdmin) ExpireClaim(ctx context.Context, task string) error {
	_, e := a.s.DB.ExecContext(ctx, "UPDATE tasks SET claim_expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=?", task)
	return e
}
func (a mariaAdmin) ExhaustBudget(ctx context.Context, project string) error {
	_, e := a.s.DB.ExecContext(ctx, "UPDATE projects SET event_budget=100,event_count=100 WHERE id=?", project)
	return e
}
func (a mariaAdmin) MutateHistory(ctx context.Context, table, project string) error {
	try := func(q string) error {
		tx, e := a.s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		_, e = tx.ExecContext(ctx, q, project)
		return e
	}
	upd := try("UPDATE `" + table + "` SET project_id=project_id WHERE project_id=?")
	if upd == nil {
		return nil
	}
	if try("DELETE FROM `"+table+"` WHERE project_id=?") == nil {
		return nil
	}
	return upd
}
func (a mariaAdmin) LockProject(ctx context.Context, project string) (func(), error) {
	tx, e := a.s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, "SELECT id FROM projects WHERE id=? FOR UPDATE", project); e != nil {
		tx.Rollback()
		return nil, e
	}
	return func() { tx.Rollback() }, nil
}
func (a mariaAdmin) CountOperations(ctx context.Context, project, op string) (int, error) {
	var n int
	e := a.s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM operations WHERE project_id=? AND operation_id=?", project, op).Scan(&n)
	return n, e
}

// mariaDatabase creates a fresh database on the test server and opens it as
// the server owner. The database is dropped when the test ends.
func mariaDatabase(t *testing.T) (*mariadb.Store, mariadb.Config) {
	t.Helper()
	addr, pw := os.Getenv("THOTH_TEST_MARIADB_ADDR"), os.Getenv("THOTH_TEST_MARIADB_ROOT_PASSWORD")
	if addr == "" {
		t.Skip("THOTH_TEST_MARIADB_ADDR unset; MariaDB contract NOT executed")
	}
	ctx := context.Background()
	root, e := mariadb.Open(ctx, mariadb.Config{User: "root", Password: pw, Addr: addr})
	if e != nil {
		t.Fatal(e)
	}
	name := fmt.Sprintf("thoth_test_%d", time.Now().UnixNano())
	if _, e = root.DB.ExecContext(ctx, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		root.DB.ExecContext(context.Background(), "DROP DATABASE "+name)
		root.Close()
	})
	cfg := mariadb.Config{User: "root", Password: pw, Addr: addr, Database: name}
	s, e := mariadb.Open(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, cfg
}

func mariaHarness(t *testing.T) contract.Harness {
	t.Helper()
	s, _ := mariaDatabase(t)
	if e := s.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := s.Migrate(context.Background()); e != nil {
		t.Fatal("repeat migration:", e)
	}
	return contract.Harness{Backend: s, Admin: mariaAdmin{s}}
}

func TestContractPostgreSQL(t *testing.T) { contract.Run(t, postgresHarness(t)) }
func TestContractMariaDB(t *testing.T)    { contract.Run(t, mariaHarness(t)) }

// All editions must return identical response shapes for every read.
func TestContractShapeParity(t *testing.T) {
	full := contract.Shapes(t, postgresHarness(t))
	for name, h := range map[string]contract.Harness{"mini": mariaHarness(t), "ultralite": sqliteHarness(t)} {
		other := contract.Shapes(t, h)
		for tool, shape := range full {
			if other[tool] != shape {
				t.Errorf("%s: %s shape differs\nfull: %s\n%s: %s", name, tool, shape, name, other[tool])
			}
		}
	}
}

// The Mini runtime login appends history and updates mutable state only.
func TestMariaDBRuntimePrivileges(t *testing.T) {
	h := mariaHarness(t)
	owner := h.Backend.(*mariadb.Store)
	ctx := context.Background()
	var db string
	if e := owner.DB.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&db); e != nil {
		t.Fatal(e)
	}
	user := "thoth_app"
	pw := strings.Repeat("Rt", 16)
	if e := owner.GrantRuntime(ctx, user, pw); e != nil {
		t.Fatal(e)
	}
	if e := owner.GrantRuntime(ctx, user, pw); e != nil {
		t.Fatal("repeat grant:", e)
	}
	app, e := mariadb.Open(ctx, mariadb.Config{User: user, Password: pw, Addr: os.Getenv("THOTH_TEST_MARIADB_ADDR"), Database: db})
	if e != nil {
		t.Fatal(e)
	}
	defer app.Close()
	if e = app.Register(ctx, contract.Agent); e != nil {
		t.Fatal(e)
	}
	if e = app.Register(ctx, contract.Agent); e != nil {
		t.Fatal("repeat registration needs no UPDATE privilege:", e)
	}
	project := contract.ID(must(app.Call(ctx, contract.Agent, "create_project", memory.Input{Name: "priv", Repository: "https://github.com/thoth-contract/priv"})))
	must(app.Call(ctx, contract.Agent, "create_project", memory.Input{Name: "priv", Repository: "https://github.com/thoth-contract/priv"}))
	must(app.Call(ctx, contract.Agent, "start_session", memory.Input{ProjectID: project, OperationID: "priv-session", Agent: "new-agent", GitRevision: strings.Repeat("a", 40)}))
	must(app.Call(ctx, contract.Agent, "start_session", memory.Input{ProjectID: project, OperationID: "priv-session-2", Agent: "new-agent", GitRevision: strings.Repeat("a", 40)}))
	ev := contract.ID(must(app.Call(ctx, contract.Agent, "attach_evidence", memory.Input{ProjectID: project, OperationID: "priv-evidence", SourceKind: "log", Locator: "log:1", GitRevision: strings.Repeat("a", 40), Outcome: "pass", Body: "x"})))
	for _, q := range []string{"UPDATE events SET body='x'", "DELETE FROM records", "UPDATE operations SET payload_hash=''", "UPDATE principals SET id=id"} {
		if _, e = app.DB.ExecContext(ctx, q); e == nil {
			t.Errorf("runtime login executed %q", q)
		}
	}
	if _, e = app.DB.ExecContext(ctx, "INSERT INTO attestations(id,project_id,evidence_id,principal_id,statement) VALUES(UUID(),?,?,?,'self')", project, ev, contract.Agent.ID); e == nil {
		t.Error("runtime login inserted an attestation")
	}
}

func must(v any, e error) any {
	if e != nil {
		panic(e)
	}
	return v
}

func TestMariaDBMigrationSafety(t *testing.T) {
	s, _ := mariaDatabase(t)
	ctx := context.Background()
	if e := s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.ExecContext(ctx, "UPDATE schema_migrations SET checksum=REPEAT('0',64)"); e != nil {
		t.Fatal(e)
	}
	if e := s.Migrate(ctx); e == nil || !strings.Contains(e.Error(), "checksum") {
		t.Fatal("checksum corruption accepted:", e)
	}
	if _, e := s.DB.ExecContext(ctx, "UPDATE schema_migrations SET status='applying'"); e != nil {
		t.Fatal(e)
	}
	if e := s.Migrate(ctx); e == nil || !strings.Contains(e.Error(), "interrupted") {
		t.Fatal("interrupted migration not refused:", e)
	}
	var n int
	if e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema=DATABASE()").Scan(&n); e != nil || n != 12 {
		t.Fatal("append-only triggers", n, e)
	}
	if e := s.DB.QueryRowContext(ctx, "SELECT 1 FROM projects LIMIT 1").Scan(&n); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal(e)
	}
}
