package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"

	"github.com/Lord1Egypt/ThothMemory/internal/contract"
	"github.com/Lord1Egypt/ThothMemory/internal/sqlite"
)

type sqliteAdmin struct{ s *sqlite.Store }

func (a sqliteAdmin) ExpireClaim(ctx context.Context, task string) error {
	_, e := a.s.DB.ExecContext(ctx, "UPDATE tasks SET claim_expires_at='2000-01-01T00:00:00.000000Z' WHERE id=?", task)
	return e
}
func (a sqliteAdmin) ExhaustBudget(ctx context.Context, project string) error {
	_, e := a.s.DB.ExecContext(ctx, "UPDATE projects SET event_budget=100,event_count=100 WHERE id=?", project)
	return e
}
func (a sqliteAdmin) MutateHistory(ctx context.Context, table, project string) error {
	try := func(q string) error {
		tx, e := a.s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		_, e = tx.ExecContext(ctx, q, project)
		return e
	}
	upd := try("UPDATE " + table + " SET project_id=project_id WHERE project_id=?")
	if upd == nil {
		return nil
	}
	if try("DELETE FROM "+table+" WHERE project_id=?") == nil {
		return nil
	}
	return upd
}

// LockProject holds SQLite's single write lock, the closest equivalent of a
// row lock: a concurrent writer waits for it.
func (a sqliteAdmin) LockProject(ctx context.Context, project string) (func(), error) {
	tx, e := a.s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE projects SET event_count=event_count WHERE id=?", project); e != nil {
		tx.Rollback()
		return nil, e
	}
	return func() { tx.Rollback() }, nil
}
func (a sqliteAdmin) CountOperations(ctx context.Context, project, op string) (int, error) {
	var n int
	e := a.s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM operations WHERE project_id=? AND operation_id=?", project, op).Scan(&n)
	return n, e
}

func sqliteHarness(t *testing.T) contract.Harness {
	t.Helper()
	ctx := context.Background()
	s, e := sqlite.Open(ctx, filepath.Join(t.TempDir(), "memory.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Migrate(ctx); e != nil {
		t.Fatal("repeat migration:", e)
	}
	return contract.Harness{Backend: s, Admin: sqliteAdmin{s}}
}

func TestContractSQLite(t *testing.T) { contract.Run(t, sqliteHarness(t)) }

// The helper process opens the database file named by THOTH_SQLITE_HELPER
// and performs one role, so tests exercise genuinely separate processes.
func TestSQLiteHelperProcess(t *testing.T) {
	path := os.Getenv("THOTH_SQLITE_HELPER")
	if path == "" {
		t.Skip("helper process only")
	}
	ctx := context.Background()
	s, e := sqlite.Open(ctx, path)
	if e != nil {
		fmt.Println("open:", e)
		os.Exit(2)
	}
	if e = s.Migrate(ctx); e != nil {
		fmt.Println("migrate:", e)
		os.Exit(2)
	}
	_ = s.Register(ctx, contract.Agent)
	project, task, role := os.Getenv("THOTH_PROJECT"), os.Getenv("THOTH_TASK"), os.Getenv("THOTH_ROLE")
	switch role {
	case "race":
		_, e = memory.Call(ctx, s, contract.Agent, "update_task", memory.Input{ProjectID: project, TaskID: task, OperationID: "race-process-" + os.Getenv("THOTH_N"), ExpectedRevision: 1, State: "implemented"})
		for i := 0; i < 20; i++ {
			if _, err := memory.Call(ctx, s, contract.Agent, "append_event", memory.Input{ProjectID: project, OperationID: fmt.Sprintf("proc-%s-%02d", os.Getenv("THOTH_N"), i), Body: "parallel process write"}); err != nil {
				fmt.Println("append:", err)
				os.Exit(3)
			}
		}
		if e == nil {
			fmt.Println("WON")
		} else if !errors.Is(e, memory.ErrConflict) {
			fmt.Println("unexpected:", e)
			os.Exit(4)
		}
	case "loop":
		for i := 0; ; i++ {
			if _, err := memory.Call(ctx, s, contract.Agent, "append_event", memory.Input{ProjectID: project, OperationID: fmt.Sprintf("loop-%07d", i), Body: strings.Repeat("crash test ", 20)}); err != nil {
				fmt.Println("append:", err)
				os.Exit(3)
			}
		}
	}
	os.Exit(0)
}

func helper(t *testing.T, path, role, project, task, n string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSQLiteHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "THOTH_SQLITE_HELPER="+path, "THOTH_ROLE="+role, "THOTH_PROJECT="+project, "THOTH_TASK="+task, "THOTH_N="+n)
	return cmd
}

func countRows(t *testing.T, s *sqlite.Store, table, project string) int {
	var n int
	if e := s.DB.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE project_id=?", project).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestSQLiteMultiProcessWriters(t *testing.T) {
	h := sqliteHarness(t)
	s := h.Backend.(*sqlite.Store)
	ctx := context.Background()
	if e := s.Register(ctx, contract.Agent); e != nil {
		t.Fatal(e)
	}
	project := contract.ID(must(s.Call(ctx, contract.Agent, "create_project", memory.Input{Name: "processes", Repository: "https://github.com/thoth-contract/processes"})))
	task := contract.ID(must(s.Call(ctx, contract.Agent, "create_task", memory.Input{ProjectID: project, OperationID: "process-task", Title: "contended"})))
	const procs = 6
	cmds := make([]*exec.Cmd, procs)
	outs := make([]*strings.Builder, procs)
	for i := range cmds {
		cmds[i], outs[i] = helper(t, s.Path, "race", project, task, fmt.Sprint(i)), &strings.Builder{}
		cmds[i].Stdout = outs[i]
		if e := cmds[i].Start(); e != nil {
			t.Fatal(e)
		}
	}
	wins := 0
	for i, c := range cmds {
		if e := c.Wait(); e != nil {
			t.Fatalf("process %d: %v %s", i, e, outs[i])
		}
		if strings.Contains(outs[i].String(), "WON") {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d processes won the same expected revision", wins)
	}
	// Every mutation is one event plus one operation; nothing lost or duplicated.
	events, ops := countRows(t, s, "events", project), countRows(t, s, "operations", project)
	if want := 1 + 1 + procs*20; events != want || ops != want {
		t.Fatalf("events=%d operations=%d want %d", events, ops, want)
	}
	if e := sqlite.Check(ctx, s.DB); e != nil {
		t.Fatal(e)
	}
}

func TestSQLiteCrashRecoveryAndOnlineBackup(t *testing.T) {
	h := sqliteHarness(t)
	s := h.Backend.(*sqlite.Store)
	ctx := context.Background()
	if e := s.Register(ctx, contract.Agent); e != nil {
		t.Fatal(e)
	}
	project := contract.ID(must(s.Call(ctx, contract.Agent, "create_project", memory.Input{Name: "crash", Repository: "https://github.com/thoth-contract/crash"})))
	writer := helper(t, s.Path, "loop", project, "", "0")
	if e := writer.Start(); e != nil {
		t.Fatal(e)
	}
	time.Sleep(700 * time.Millisecond)
	// Online backup while the other process writes.
	backup := filepath.Join(t.TempDir(), "online.db")
	sum, e := s.Backup(ctx, backup)
	if e != nil || len(sum) != 64 {
		t.Fatal("online backup:", e)
	}
	time.Sleep(300 * time.Millisecond)
	if e = writer.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	_ = writer.Wait()
	// Reopen after SIGKILL: WAL recovery must leave whole mutations only.
	s.Close()
	re, e := sqlite.Open(ctx, s.Path)
	if e != nil {
		t.Fatal(e)
	}
	defer re.Close()
	if e = sqlite.Check(ctx, re.DB); e != nil {
		t.Fatal("integrity after SIGKILL:", e)
	}
	events, ops := countRows(t, re, "events", project), countRows(t, re, "operations", project)
	if events != ops || events < 10 {
		t.Fatalf("after SIGKILL events=%d operations=%d (must be equal and nonzero)", events, ops)
	}
	t.Logf("SIGKILL during writes: %d complete mutations recovered, none partial", events)
	b, e := sqlite.Open(ctx, backup)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	be, bo := countRows(t, b, "events", project), countRows(t, b, "operations", project)
	if be != bo || be == 0 || be > events {
		t.Fatalf("online backup inconsistent: events=%d operations=%d", be, bo)
	}
	t.Logf("online backup during writes: consistent snapshot of %d mutations", be)
}
