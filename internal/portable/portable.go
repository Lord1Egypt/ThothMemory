// Package portable moves one project between ThothMemory editions through a
// backend-neutral, checksummed JSON document. Import is transactional,
// repeatable and fast-forward only: it never overwrites newer state.
package portable

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
)

const (
	Format        = "thothmemory-export"
	FormatVersion = 1
	// MaxBytes bounds an import document before it is parsed.
	MaxBytes = 512 << 20
	maxRows  = 2_000_000
)

// Table describes one portable table. Columns exclude derived search data
// and per-installation counters; Key identifies a row; Mutable rows change in
// place (tasks, sessions) while all others are append-only history.
type Table struct {
	Name    string
	Columns []string
	Key     []string
	Mutable bool
}

// Tables are listed in foreign-key insertion order.
var Tables = []Table{
	{"projects", []string{"id", "name", "repository", "created_at", "event_budget"}, []string{"id"}, false},
	{"tasks", []string{"id", "project_id", "title", "state", "revision", "git_revision", "claimed_by", "claim_expires_at", "updated_at"}, []string{"id"}, true},
	{"task_dependencies", []string{"project_id", "task_id", "depends_on"}, []string{"project_id", "task_id", "depends_on"}, false},
	{"sessions", []string{"id", "project_id", "agent_id", "principal_id", "branch", "git_revision", "started_at", "finished_at", "revision"}, []string{"id"}, true},
	{"evidence", []string{"id", "project_id", "task_id", "source_kind", "locator", "git_revision", "artifact_digest", "outcome", "summary", "reported_by", "created_at"}, []string{"id"}, false},
	{"attestations", []string{"id", "project_id", "evidence_id", "principal_id", "statement", "created_at"}, []string{"id"}, false},
	{"records", []string{"id", "project_id", "task_id", "session_id", "kind", "title", "body", "git_revision", "principal_id", "agent_id", "evidence_id", "supersedes", "created_at"}, []string{"id"}, false},
	{"events", []string{"id", "project_id", "task_id", "session_id", "kind", "body", "principal_id", "agent_id", "git_revision", "created_at", "details"}, []string{"id"}, false},
	{"operations", []string{"project_id", "operation_id", "principal_id", "payload_hash", "response", "created_at"}, []string{"project_id", "principal_id", "operation_id"}, false},
}

var jsonColumns = map[string]bool{"details": true, "response": true}
var intColumns = map[string]bool{"revision": true, "event_budget": true}

// Reader is a consistent read snapshot; Writer one import transaction. Both
// are implemented by each backend using its owner connection.
type Reader interface {
	// Select returns the given columns of project rows (projects: id=project).
	Select(ctx context.Context, t Table, project string) ([]memory.Row, error)
	Close(ctx context.Context) error
}
type Writer interface {
	Get(ctx context.Context, t Table, key []any) (memory.Row, bool, error)
	Insert(ctx context.Context, t Table, values []any) error
	Update(ctx context.Context, t Table, values []any) error
	EnsurePrincipal(ctx context.Context, id string) error
	EnsureAgent(ctx context.Context, id string) error
	Imported(ctx context.Context, project, checksum string) (bool, error)
	// EntityEvents returns kind and details of the target's events for one
	// task (column task_id) or session (column session_id).
	EntityEvents(ctx context.Context, project, column, id string) ([]memory.Row, error)
	// Finish recounts the event budget slot usage and records provenance.
	Finish(ctx context.Context, project string, r Provenance) error
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}
type Backend interface {
	memory.Backend
	SchemaVersion(ctx context.Context) (string, error)
	ExportReader(ctx context.Context) (Reader, error)
	ImportWriter(ctx context.Context) (Writer, error)
}

type Provenance struct {
	ExportID, SourceEdition, SourceVersion, Checksum, Principal string
	FormatVersion                                               int
	Counts                                                      map[string]int
}

type Document struct {
	Format        string                       `json:"format"`
	FormatVersion int                          `json:"format_version"`
	ExportID      string                       `json:"export_id"`
	ExportedAt    string                       `json:"exported_at"`
	Source        Source                       `json:"source"`
	ProjectID     string                       `json:"project_id"`
	Counts        map[string]int               `json:"counts"`
	PayloadSHA256 string                       `json:"payload_sha256"`
	Payload       map[string][]json.RawMessage `json:"payload"`
}
type Source struct {
	Product string `json:"product"`
	Edition string `json:"edition"`
	Version string `json:"version"`
	Schema  string `json:"schema"`
}

// canonical renders a row's portable columns deterministically: timestamps
// as UTC microseconds, integers as integers, JSON columns re-encoded.
func canonical(t Table, r memory.Row) (json.RawMessage, error) {
	out := make(map[string]any, len(t.Columns))
	for _, c := range t.Columns {
		v, e := canonicalValue(c, r[c])
		if e != nil {
			return nil, fmt.Errorf("%s.%s: %w", t.Name, c, e)
		}
		out[c] = v
	}
	return json.Marshal(out)
}

func canonicalValue(col string, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch {
	case jsonColumns[col]:
		var raw []byte
		switch x := v.(type) {
		case string:
			raw = []byte(x)
		case []byte:
			raw = x
		default:
			b, e := json.Marshal(x)
			if e != nil {
				return nil, e
			}
			raw = b
		}
		var j any
		if e := json.Unmarshal(raw, &j); e != nil {
			return nil, errors.New("invalid JSON")
		}
		return j, nil
	case strings.HasSuffix(col, "_at"):
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("timestamp must be a string")
		}
		t, e := time.Parse(time.RFC3339Nano, s)
		if e != nil {
			return nil, errors.New("invalid timestamp")
		}
		return t.UTC().Format("2006-01-02T15:04:05.000000Z"), nil
	case intColumns[col]:
		switch x := v.(type) {
		case float64:
			return int64(x), nil
		case int64:
			return x, nil
		case json.Number:
			return x.Int64()
		}
		return nil, errors.New("integer expected")
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return nil, errors.New("string expected")
}

// values converts a canonical row into typed values in column order.
func values(t Table, raw json.RawMessage) ([]any, memory.Row, error) {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	var m map[string]any
	if e := d.Decode(&m); e != nil {
		return nil, nil, errors.New("invalid row in " + t.Name)
	}
	if len(m) != len(t.Columns) {
		return nil, nil, errors.New("unexpected columns in " + t.Name)
	}
	out := make([]any, len(t.Columns))
	for i, c := range t.Columns {
		v, ok := m[c]
		if !ok {
			return nil, nil, fmt.Errorf("%s.%s missing", t.Name, c)
		}
		switch {
		case v == nil:
		case jsonColumns[c]:
			b, _ := json.Marshal(v)
			v = b
		case strings.HasSuffix(c, "_at"):
			ts, e := time.Parse(time.RFC3339Nano, fmt.Sprint(v))
			if e != nil {
				return nil, nil, fmt.Errorf("%s.%s invalid timestamp", t.Name, c)
			}
			v = ts.UTC()
		case intColumns[c]:
			n, e := v.(json.Number).Int64()
			if e != nil {
				return nil, nil, fmt.Errorf("%s.%s invalid integer", t.Name, c)
			}
			v = n
		default:
			s, ok := v.(string)
			if !ok {
				return nil, nil, fmt.Errorf("%s.%s must be a string", t.Name, c)
			}
			v = s
		}
		out[i] = v
	}
	return out, m, nil
}

func payloadSum(p map[string][]json.RawMessage) string {
	h := sha256.New()
	for _, t := range Tables {
		h.Write([]byte(t.Name + "\n"))
		for _, r := range p[t.Name] {
			h.Write(r)
			h.Write([]byte("\n"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Export writes one project from a consistent snapshot.
func Export(ctx context.Context, b Backend, project string, w io.Writer) (*Document, error) {
	if !memory.UUID(project) {
		return nil, errors.New("project UUID required")
	}
	project = strings.ToLower(project)
	schema, e := b.SchemaVersion(ctx)
	if e != nil {
		return nil, e
	}
	r, e := b.ExportReader(ctx)
	if e != nil {
		return nil, e
	}
	defer r.Close(context.WithoutCancel(ctx))
	info := b.Info()
	doc := &Document{Format: Format, FormatVersion: FormatVersion, ExportID: newID(), ExportedAt: time.Now().UTC().Format(time.RFC3339), Source: Source{info.Product, info.Edition, info.Version, schema}, ProjectID: project, Counts: map[string]int{}, Payload: map[string][]json.RawMessage{}}
	for _, t := range Tables {
		rows, e := r.Select(ctx, t, project)
		if e != nil {
			return nil, e
		}
		if t.Name == "projects" && len(rows) != 1 {
			return nil, errors.New("project not found")
		}
		list := make([]json.RawMessage, 0, len(rows))
		for _, row := range rows {
			c, e := canonical(t, row)
			if e != nil {
				return nil, e
			}
			list = append(list, c)
		}
		sort.Slice(list, func(i, j int) bool { return string(list[i]) < string(list[j]) })
		doc.Payload[t.Name], doc.Counts[t.Name] = list, len(list)
	}
	doc.PayloadSHA256 = payloadSum(doc.Payload)
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return doc, enc.Encode(doc)
}

type Result struct {
	ProjectID      string         `json:"project_id"`
	AlreadyApplied bool           `json:"already_imported"`
	Inserted       map[string]int `json:"inserted"`
	FastForwarded  map[string]int `json:"fast_forwarded"`
	KeptNewer      map[string]int `json:"kept_newer_target"`
	Unchanged      map[string]int `json:"unchanged"`
}

// ConflictError reports why an import was refused; nothing was written.
type ConflictError struct{ Reason string }

func (c *ConflictError) Error() string { return "import refused, nothing written: " + c.Reason }

// Read parses and validates a document: format, checksum, bounds, one
// project, and referential integrity inside the document.
func Read(r io.Reader) (*Document, error) {
	var doc Document
	d := json.NewDecoder(io.LimitReader(r, MaxBytes+1))
	d.DisallowUnknownFields()
	if e := d.Decode(&doc); e != nil {
		return nil, errors.New("not a ThothMemory export document or larger than 512 MiB")
	}
	if doc.Format != Format || doc.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("unsupported export format %q version %d", doc.Format, doc.FormatVersion)
	}
	if doc.Source.Edition != "full" && doc.Source.Edition != "mini" || len(doc.Source.Version) == 0 || len(doc.Source.Version) > 50 || !memory.UUID(doc.ExportID) || !memory.UUID(doc.ProjectID) {
		return nil, errors.New("invalid export header")
	}
	known := map[string]bool{}
	total := 0
	for _, t := range Tables {
		known[t.Name] = true
		total += len(doc.Payload[t.Name])
		if doc.Counts[t.Name] != len(doc.Payload[t.Name]) {
			return nil, fmt.Errorf("row count mismatch in %s", t.Name)
		}
	}
	for name := range doc.Payload {
		if !known[name] {
			return nil, errors.New("unknown table " + name)
		}
	}
	if total > maxRows {
		return nil, errors.New("export exceeds 2,000,000 rows")
	}
	// The document may be indented; rows are hashed and compared compact.
	for name, rows := range doc.Payload {
		for i, raw := range rows {
			var buf bytes.Buffer
			if e := json.Compact(&buf, raw); e != nil {
				return nil, errors.New("invalid row in " + name)
			}
			rows[i] = buf.Bytes()
		}
	}
	if payloadSum(doc.Payload) != doc.PayloadSHA256 {
		return nil, errors.New("payload SHA-256 mismatch: the export is damaged or was modified")
	}
	return &doc, integrity(&doc)
}

func integrity(doc *Document) error {
	ids := map[string]map[string]bool{}
	for _, t := range Tables {
		ids[t.Name] = map[string]bool{}
		for _, raw := range doc.Payload[t.Name] {
			_, m, e := values(t, raw)
			if e != nil {
				return e
			}
			if t.Name == "projects" {
				if m["id"] != doc.ProjectID {
					return errors.New("project row does not match the export header")
				}
			} else if m["project_id"] != doc.ProjectID {
				return fmt.Errorf("%s row belongs to another project", t.Name)
			}
			for _, c := range t.Columns {
				if (c == "id" || strings.HasSuffix(c, "_id") && c != "principal_id" && c != "agent_id" && c != "operation_id" || c == "supersedes" || c == "depends_on") && m[c] != nil && !memory.UUID(fmt.Sprint(m[c])) {
					return fmt.Errorf("%s.%s is not a UUID", t.Name, c)
				}
			}
			if id, ok := m["id"].(string); ok {
				ids[t.Name][id] = true
			}
		}
	}
	if len(ids["projects"]) != 1 {
		return errors.New("export must contain exactly one project")
	}
	refs := []struct{ table, col, target string }{
		{"task_dependencies", "task_id", "tasks"}, {"task_dependencies", "depends_on", "tasks"},
		{"evidence", "task_id", "tasks"}, {"attestations", "evidence_id", "evidence"},
		{"records", "task_id", "tasks"}, {"records", "session_id", "sessions"}, {"records", "evidence_id", "evidence"}, {"records", "supersedes", "records"},
		{"events", "task_id", "tasks"}, {"events", "session_id", "sessions"},
	}
	for _, ref := range refs {
		t := table(ref.table)
		for _, raw := range doc.Payload[ref.table] {
			_, m, _ := values(t, raw)
			if v, ok := m[ref.col].(string); ok && !ids[ref.target][v] {
				return fmt.Errorf("%s.%s references a %s row missing from the export", ref.table, ref.col, ref.target)
			}
		}
	}
	return nil
}

func table(name string) Table {
	for _, t := range Tables {
		if t.Name == name {
			return t
		}
	}
	panic("unknown table " + name)
}

// Import applies a validated document in one transaction.
func Import(ctx context.Context, b Backend, doc *Document, principal string) (*Result, error) {
	res := &Result{ProjectID: doc.ProjectID, Inserted: map[string]int{}, FastForwarded: map[string]int{}, KeptNewer: map[string]int{}, Unchanged: map[string]int{}}
	w, e := b.ImportWriter(ctx)
	if e != nil {
		return nil, e
	}
	defer w.Rollback(context.WithoutCancel(ctx))
	if done, e := w.Imported(ctx, doc.ProjectID, doc.PayloadSHA256); e != nil {
		return nil, e
	} else if done {
		res.AlreadyApplied = true
		return res, nil
	}
	if e = w.EnsurePrincipal(ctx, principal); e != nil {
		return nil, e
	}
	history := lineage(doc)
	for _, t := range Tables {
		rows := doc.Payload[t.Name]
		if t.Name == "records" {
			rows = supersessionOrder(rows)
		}
		for _, raw := range rows {
			vals, m, e := values(t, raw)
			if e != nil {
				return nil, e
			}
			if e = references(ctx, w, t, m); e != nil {
				return nil, e
			}
			key := make([]any, len(t.Key))
			for i, k := range t.Key {
				key[i] = vals[indexOf(t.Columns, k)]
			}
			existing, found, e := w.Get(ctx, t, key)
			if e != nil {
				return nil, e
			}
			if !found {
				if e = w.Insert(ctx, t, vals); e != nil {
					if t.Name == "projects" {
						return nil, &ConflictError{"cannot create the project; its repository may already be registered under another ID (" + memory.SafeError(e) + ")"}
					}
					return nil, e
				}
				res.Inserted[t.Name]++
				continue
			}
			mine, e := canonical(t, existing)
			if e != nil {
				return nil, e
			}
			if t.Name == "projects" {
				if existing["repository"] != m["repository"] || existing["name"] != m["name"] {
					return nil, &ConflictError{"the target already holds this project ID with a different name or repository"}
				}
				res.Unchanged[t.Name]++
				continue
			}
			if string(mine) == string(raw) {
				res.Unchanged[t.Name]++
				continue
			}
			if !t.Mutable {
				return nil, &ConflictError{fmt.Sprintf("immutable %s row %v differs between source and target", t.Name, key)}
			}
			id := fmt.Sprint(m["id"])
			switch {
			case history.contains(t.Name, id, map[string]any(existing)):
				if e = w.Update(ctx, t, vals); e != nil {
					return nil, e
				}
				res.FastForwarded[t.Name]++
			default:
				newer, e := targetHasNewer(ctx, w, doc.ProjectID, t, id, m)
				if e != nil {
					return nil, e
				}
				if newer {
					res.KeptNewer[t.Name]++
					continue
				}
				return nil, &ConflictError{fmt.Sprintf("%s %s diverged: source revision %v and target revision %v have no common history", strings.TrimSuffix(t.Name, "s"), id, m["revision"], existing["revision"])}
			}
		}
	}
	if e = w.Finish(ctx, doc.ProjectID, Provenance{ExportID: doc.ExportID, SourceEdition: doc.Source.Edition, SourceVersion: doc.Source.Version, Checksum: doc.PayloadSHA256, Principal: principal, FormatVersion: doc.FormatVersion, Counts: doc.Counts}); e != nil {
		return nil, e
	}
	return res, w.Commit(ctx)
}

func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

// references registers principals and agents a row names, which exist
// outside any project and are created on demand as plain identifiers.
func references(ctx context.Context, w Writer, t Table, m map[string]any) error {
	for _, c := range []string{"principal_id", "reported_by", "claimed_by"} {
		if v, ok := m[c].(string); ok && v != "" {
			if e := w.EnsurePrincipal(ctx, v); e != nil {
				return e
			}
		}
	}
	if t.Name == "sessions" {
		return w.EnsureAgent(ctx, m["agent_id"].(string))
	}
	return nil
}

// supersessionOrder inserts a superseded record before its replacement.
func supersessionOrder(rows []json.RawMessage) []json.RawMessage {
	t := table("records")
	byID, next := map[string]json.RawMessage{}, map[string]string{}
	var roots []string
	for _, raw := range rows {
		_, m, e := values(t, raw)
		if e != nil {
			return rows // invalid rows were already rejected by Read
		}
		id := m["id"].(string)
		byID[id] = raw
		if s, ok := m["supersedes"].(string); ok {
			next[s] = id
		} else {
			roots = append(roots, id)
		}
	}
	out := make([]json.RawMessage, 0, len(rows))
	seen := map[string]bool{}
	for _, id := range roots {
		for id != "" && !seen[id] {
			seen[id] = true
			out = append(out, byID[id])
			id = next[id]
		}
	}
	// Records superseding rows outside this export keep their relative order.
	for _, raw := range rows {
		_, m, _ := values(t, raw)
		if !seen[m["id"].(string)] {
			out = append(out, raw)
		}
	}
	return out
}

// versions are the states a task or session passed through, taken from the
// result snapshots that every mutation stores in its event details.
type versions map[string]map[string][]map[string]any

func lineage(doc *Document) versions {
	t := table("events")
	var events []memory.Row
	for _, raw := range doc.Payload["events"] {
		if _, m, e := values(t, raw); e == nil {
			events = append(events, m)
		}
	}
	return states(events)
}

var mutations = map[string]string{"create_task": "tasks", "claim_task": "tasks", "release_task": "tasks", "update_task": "tasks", "start_session": "sessions", "checkpoint_session": "sessions", "finish_session": "sessions"}

// states extracts entity snapshots from events carrying kind and details.
func states(events []memory.Row) versions {
	v := versions{"tasks": {}, "sessions": {}}
	for _, ev := range events {
		tableName, ok := mutations[fmt.Sprint(ev["kind"])]
		if !ok {
			continue
		}
		var raw []byte
		switch d := ev["details"].(type) {
		case []byte:
			raw = d
		case string:
			raw = []byte(d)
		default:
			raw, _ = json.Marshal(d)
		}
		var d struct {
			Result map[string]any `json:"result"`
		}
		if json.Unmarshal(raw, &d) != nil {
			continue
		}
		if id, ok := d.Result["id"].(string); ok {
			v[tableName][id] = append(v[tableName][id], d.Result)
		}
	}
	return v
}

// contains reports whether row is one of the recorded states of an entity.
func (v versions) contains(tableName, id string, row map[string]any) bool {
	for _, state := range v[tableName][id] {
		if same(state, row) {
			return true
		}
	}
	return false
}

// same compares the facts that define a task or session version.
func same(a, b map[string]any) bool {
	for _, k := range []string{"revision", "state", "git_revision", "claimed_by", "branch", "finished_at"} {
		_, inA := a[k]
		_, inB := b[k]
		if !inA && !inB {
			continue
		}
		av, _ := canonicalValue(k, a[k])
		bv, _ := canonicalValue(k, b[k])
		if fmt.Sprint(av) != fmt.Sprint(bv) {
			return false
		}
	}
	return true
}

// targetHasNewer reports whether the incoming row is an older state recorded
// in the target's own history, so keeping the target loses nothing.
func targetHasNewer(ctx context.Context, w Writer, project string, t Table, id string, incoming map[string]any) (bool, error) {
	column := "task_id"
	if t.Name == "sessions" {
		column = "session_id"
	}
	events, e := w.EntityEvents(ctx, project, column, id)
	if e != nil {
		return false, e
	}
	return states(events).contains(t.Name, id, incoming), nil
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails: Go aborts the process instead
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
