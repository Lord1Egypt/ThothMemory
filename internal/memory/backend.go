package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Row is one JSON object returned to MCP clients and the dashboard. Both
// backends produce identical keys for the same entity.
type Row = map[string]any

// Info identifies an edition and the optional behaviour its backend provides.
type Info struct {
	Product      string          `json:"product"`
	Edition      string          `json:"edition"`
	Version      string          `json:"version"`
	Database     string          `json:"database"`
	Capabilities map[string]bool `json:"capabilities"`
}

type SearchQuery struct {
	Text, TaskID, Agent, EvidenceID, ExactID, CurrentRevision string
	After, Before                                             *time.Time
	Limit, Offset                                             int
}

// ExploreQuery selects one paginated dashboard view of a project's records.
type ExploreQuery struct {
	View          string // sessions, records, evidence, events, task, imports
	TaskID, Kind  string
	Limit, Offset int
}

// Backend persists the shared domain. Implementations must keep every query
// bounded and project-scoped; authorization happens before any call.
type Backend interface {
	Info() Info
	Ping(ctx context.Context) error
	Register(ctx context.Context, p Principal) error
	ListProjects(ctx context.Context, all bool, ids []string, limit, offset int) ([]Row, error)
	CreateProject(ctx context.Context, name, repository string) (Row, error)
	Tasks(ctx context.Context, project, task, state string, limit, offset int) ([]Row, error)
	Activity(ctx context.Context, project, task string, limit, offset int) ([]Row, error)
	LatestHandoff(ctx context.Context, project string) ([]Row, error)
	Search(ctx context.Context, project string, q SearchQuery) ([]Row, error)
	// Context returns the sections named in ContextSections.
	Context(ctx context.Context, project string) (map[string][]Row, error)
	Overview(ctx context.Context, all bool, ids []string) ([]Row, error)
	Explore(ctx context.Context, project string, q ExploreQuery) ([]Row, error)
	Begin(ctx context.Context) (Tx, error)
}

// StatusReporter is implemented by backends that can describe their storage.
type StatusReporter interface {
	Status(ctx context.Context) (Row, error)
}

var ContextSections = []string{"project", "tasks", "sessions", "checkpoints", "handoff", "decisions_and_issues", "evidence"}

// Requirement is the evidence a task state demands: passing evidence of one
// of Kinds, bound to the same task and full revision, owner-attested if set.
type Requirement struct {
	Kinds    []string
	Attested bool
}

type RecordInput struct {
	Project, Task, Session, Kind, Title, Body, Revision, Principal, Agent, Evidence, Supersedes string
}
type EvidenceInput struct {
	Project, Task, SourceKind, Locator, Revision, Digest, Outcome, Summary, ReportedBy string
}
type EventInput struct {
	Project, Task, Session, Kind, Body, Principal, Agent, Revision string
	Details                                                        []byte
}

// Tx is one mutation transaction. Methods returning a Row return ErrConflict
// when the guarded row did not match (stale revision, foreign claim, closed
// session). Empty optional identifiers are stored as NULL.
type Tx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	// LockOperation serializes concurrent uses of one operation ID until commit.
	LockOperation(ctx context.Context, project, principal, operation string) error
	Operation(ctx context.Context, project, principal, operation string) (hash string, response []byte, found bool, err error)
	ReserveEvent(ctx context.Context, project string) (bool, error)
	SessionOpen(ctx context.Context, project, session, principal string) (bool, error)
	EnsureAgent(ctx context.Context, agent string) error
	InsertTask(ctx context.Context, project, title, revision string) (Row, error)
	InsertDependency(ctx context.Context, project, task, dependsOn string) error
	InsertSession(ctx context.Context, project, agent, principal, branch, revision string) (Row, error)
	AdvanceSession(ctx context.Context, project, session string, expected int, revision, branch string, finish bool) (Row, error)
	ReleaseTask(ctx context.Context, project, task string, expected int, principal string) (Row, error)
	EvidenceSatisfies(ctx context.Context, project, task, revision string, r Requirement) (bool, error)
	DependenciesOpen(ctx context.Context, project, task string) (bool, error)
	UpdateTask(ctx context.Context, project, task string, expected int, state, revision string, claim bool, principal string) (Row, error)
	RecordIs(ctx context.Context, project, id, kind string) (bool, error)
	InsertRecord(ctx context.Context, r RecordInput) (Row, error)
	InsertEvidence(ctx context.Context, e EvidenceInput) (Row, error)
	InsertAttestation(ctx context.Context, project, evidence, principal, statement string) (Row, error)
	InsertEvent(ctx context.Context, e EventInput) error
	InsertOperation(ctx context.Context, project, operation, principal, hash string, response []byte) error
}

// DatabaseError is how a backend reports a driver failure without exposing
// SQL, schema names or server messages to clients.
type DatabaseError struct {
	State     string
	Code      int
	Retryable bool
	Err       error
}

func (e *DatabaseError) Error() string    { return fmt.Sprintf("database error %s: %v", e.State, e.Err) }
func (e *DatabaseError) Unwrap() error    { return e.Err }
func (e *DatabaseError) SQLState() string { return e.State }

func SafeError(err error) string {
	if errors.Is(err, ErrConflict) || errors.Is(err, ErrForbidden) {
		return err.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "operation timed out; retry with the same operation_id"
	}
	var de *DatabaseError
	if errors.As(err, &de) {
		if de.Retryable {
			return "transient database conflict; retry with the same operation_id"
		}
		// Codes only: server messages can quote SQL text or stored values.
		return fmt.Sprintf("database rejected operation: constraint or availability failure (SQLSTATE %s, code %d)", de.State, de.Code)
	}
	var pe interface{ SQLState() string }
	if errors.As(err, &pe) {
		return "database rejected operation: constraint or availability failure"
	}
	msg := err.Error()
	if strings.Contains(msg, "connect") || strings.Contains(msg, "postgres") || strings.Contains(msg, "mysql") {
		return "database unavailable"
	}
	return msg
}
