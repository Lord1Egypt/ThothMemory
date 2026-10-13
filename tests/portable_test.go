package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/contract"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/portable"
)

func call(t *testing.T, b memory.Backend, p memory.Principal, tool string, a memory.Input) any {
	t.Helper()
	v, e := memory.Call(context.Background(), b, p, tool, a)
	if e != nil {
		t.Fatalf("%s: %v", tool, e)
	}
	return v
}

func export(t *testing.T, b portable.Backend, project string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, e := portable.Export(context.Background(), b, project, &buf); e != nil {
		t.Fatal(e)
	}
	return buf.Bytes()
}

func importDoc(t *testing.T, b portable.Backend, data []byte) (*portable.Result, error) {
	t.Helper()
	doc, e := portable.Read(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	return portable.Import(context.Background(), b, doc, "local-owner")
}

// normalized returns a JSON round trip so both backends compare as clients see them.
func normalized(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func TestPortableFullMiniRoundTrip(t *testing.T) {
	pgH, miniH := postgresHarness(t), mariaHarness(t)
	full, mini := pgH.Backend.(portable.Backend), miniH.Backend.(portable.Backend)
	ctx := context.Background()
	for _, b := range []memory.Backend{full, mini} {
		for _, p := range []memory.Principal{contract.Agent, contract.Owner} {
			if e := b.Register(ctx, p); e != nil {
				t.Fatal(e)
			}
		}
	}
	a, o := contract.Agent, contract.Owner
	head := strings.Repeat("9", 40)
	project := contract.ID(call(t, full, a, "create_project", memory.Input{Name: "Portable نقل", Repository: fmt.Sprintf("https://github.com/thoth-contract/portable-%d", time.Now().UnixNano())}))
	task := contract.ID(call(t, full, a, "create_task", memory.Input{ProjectID: project, OperationID: "port-task", Title: "Portable task", GitRevision: head}))
	dep := contract.ID(call(t, full, a, "create_task", memory.Input{ProjectID: project, OperationID: "port-dep", Title: "Dependent", DependsOn: []string{task}}))
	sid := contract.ID(call(t, full, a, "start_session", memory.Input{ProjectID: project, OperationID: "port-session", Agent: "codex", GitRevision: head, Branch: "main"}))
	call(t, full, a, "checkpoint_session", memory.Input{ProjectID: project, OperationID: "port-checkpoint", SessionID: sid, ExpectedRevision: 1, GitRevision: head, Body: "checkpoint مع نص عربي"})
	ev := contract.ID(call(t, full, a, "record_test_result", memory.Input{ProjectID: project, OperationID: "port-test", TaskID: task, Locator: "ci://port", GitRevision: head, Outcome: "pass", Body: "pass"}))
	call(t, full, o, "attest_evidence", memory.Input{ProjectID: project, OperationID: "port-attest", EvidenceID: ev, Body: "owner reviewed"})
	d1 := contract.ID(call(t, full, a, "record_decision", memory.Input{ProjectID: project, OperationID: "port-decision-1", Title: "Decision", Body: "v1"}))
	call(t, full, a, "record_decision", memory.Input{ProjectID: project, OperationID: "port-decision-2", Title: "Decision", Body: "v2", Supersedes: d1})
	call(t, full, a, "update_task", memory.Input{ProjectID: project, OperationID: "port-verify", TaskID: task, ExpectedRevision: 1, State: "verified", GitRevision: head})

	// Full -> Mini: identical reads afterwards.
	doc := export(t, full, project)
	res, e := importDoc(t, mini, doc)
	if e != nil {
		t.Fatal(e)
	}
	if res.Inserted["events"] == 0 || res.Inserted["attestations"] != 1 || res.Inserted["records"] != 3 {
		t.Fatal("unexpected import result", res)
	}
	for _, tool := range []struct {
		name string
		in   memory.Input
	}{{"get_project_context", memory.Input{ProjectID: project, GitRevision: head}}, {"list_tasks", memory.Input{ProjectID: project}}, {"get_latest_handoff", memory.Input{ProjectID: project}}, {"get_recent_activity", memory.Input{ProjectID: project, Limit: 100}}, {"explore_evidence", memory.Input{ProjectID: project}}, {"explore_task", memory.Input{ProjectID: project, TaskID: task}}} {
		f, m := normalized(call(t, full, a, tool.name, tool.in)), normalized(call(t, mini, a, tool.name, tool.in))
		if !reflect.DeepEqual(f, m) {
			fb, _ := json.Marshal(f)
			mb, _ := json.Marshal(m)
			t.Errorf("%s differs after import\nfull: %s\nmini: %s", tool.name, fb, mb)
		}
	}
	// Idempotent: the same document again changes nothing.
	if res, e = importDoc(t, mini, doc); e != nil || !res.AlreadyApplied {
		t.Fatal("repeat import", res, e)
	}
	// Idempotency survives the move: replaying an operation on Mini returns the original result.
	if _, e = memory.Call(ctx, mini, a, "update_task", memory.Input{ProjectID: project, OperationID: "port-verify", TaskID: task, ExpectedRevision: 1, State: "verified", GitRevision: head}); e != nil {
		t.Fatal("imported operation did not replay:", e)
	}
	// Work continues on Mini, then fast-forwards back into Full.
	call(t, mini, a, "update_task", memory.Input{ProjectID: project, OperationID: "mini-dep-active", TaskID: dep, ExpectedRevision: 1, State: "active"})
	call(t, mini, a, "finish_session", memory.Input{ProjectID: project, OperationID: "mini-finish", SessionID: sid, ExpectedRevision: 2, GitRevision: head, Body: "finished on the phone"})
	back := export(t, mini, project)
	res, e = importDoc(t, full, back)
	if e != nil {
		t.Fatal(e)
	}
	if res.FastForwarded["tasks"] != 1 || res.FastForwarded["sessions"] != 1 || res.Inserted["events"] != 2 {
		t.Fatal("fast-forward result", res)
	}
	got := call(t, full, a, "get_task", memory.Input{ProjectID: project, TaskID: dep}).([]memory.Row)[0]
	if got["state"] != "active" || got["revision"] != float64(2) {
		t.Fatal("task not fast-forwarded", got)
	}
	if h := call(t, full, a, "get_latest_handoff", memory.Input{ProjectID: project}).([]memory.Row); len(h) != 1 || h[0]["body"] != "finished on the phone" {
		t.Fatal("handoff from Mini missing", h)
	}
	// An older export never overwrites newer target state.
	res, e = importDoc(t, full, doc)
	if e != nil {
		t.Fatal("older export refused:", e)
	}
	if res.KeptNewer["tasks"] != 1 || res.KeptNewer["sessions"] != 1 || len(res.Inserted) != 0 {
		t.Fatal("older export result", res)
	}
	// Divergent edits of the same task are refused and nothing is written.
	call(t, full, a, "update_task", memory.Input{ProjectID: project, OperationID: "full-diverge", TaskID: dep, ExpectedRevision: 2, State: "blocked"})
	call(t, mini, a, "update_task", memory.Input{ProjectID: project, OperationID: "mini-diverge", TaskID: dep, ExpectedRevision: 2, State: "implemented"})
	before := len(call(t, full, a, "get_recent_activity", memory.Input{ProjectID: project, Limit: 100}).([]memory.Row))
	var conflict *portable.ConflictError
	if _, e = importDoc(t, full, export(t, mini, project)); !errors.As(e, &conflict) {
		t.Fatal("divergent import not refused:", e)
	}
	if after := len(call(t, full, a, "get_recent_activity", memory.Input{ProjectID: project, Limit: 100}).([]memory.Row)); after != before {
		t.Fatal("refused import wrote events", before, after)
	}
	// Damaged or inconsistent documents are rejected before any write.
	tampered := bytes.Replace(doc, []byte("owner reviewed"), []byte("owner REVIEWED"), 1)
	if _, e = importDoc(t, mini, tampered); e == nil || !strings.Contains(e.Error(), "SHA-256") {
		t.Fatal("tampered export accepted:", e)
	}
	var d portable.Document
	if e = json.Unmarshal(doc, &d); e != nil {
		t.Fatal(e)
	}
	d.Payload["tasks"] = d.Payload["tasks"][:0]
	d.Counts["tasks"] = 0
	b, _ := json.Marshal(d)
	if _, e = portable.Read(bytes.NewReader(b)); e == nil {
		t.Fatal("document with a stale checksum accepted")
	}
}
