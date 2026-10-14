// Package memory holds the edition-independent ThothMemory domain: identities,
// input bounds, authorization, revision/evidence rules and the transactional
// mutation algorithm. PostgreSQL (Full) and MariaDB (Mini) only persist it.
package memory

import (
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	FullVersion      = "0.2.2"
	MiniVersion      = "0.1.2"
	UltraLiteVersion = "0.1.0"
)

type Principal struct {
	ID       string   `json:"id"`
	Token    string   `json:"token"`
	Write    bool     `json:"write"`
	Owner    bool     `json:"owner"`
	Projects []string `json:"projects"`
}

func (p Principal) Allows(id string) bool {
	for _, v := range p.Projects {
		if v == "*" || v == id {
			return true
		}
	}
	return false
}

// Input is shared internally. MCP publishes a separate, narrow schema per tool.
type Input struct {
	ProjectID        string   `json:"project_id,omitempty"`
	OperationID      string   `json:"operation_id,omitempty"`
	Repository       string   `json:"repository,omitempty"`
	Name             string   `json:"name,omitempty"`
	TaskID           string   `json:"task_id,omitempty"`
	SessionID        string   `json:"session_id,omitempty"`
	Agent            string   `json:"agent,omitempty"`
	Branch           string   `json:"branch,omitempty"`
	GitRevision      string   `json:"git_revision,omitempty"`
	ExpectedRevision int      `json:"expected_revision,omitempty"`
	Title            string   `json:"title,omitempty"`
	Body             string   `json:"body,omitempty"`
	State            string   `json:"state,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	EvidenceID       string   `json:"evidence_id,omitempty"`
	Supersedes       string   `json:"supersedes,omitempty"`
	SourceKind       string   `json:"source_kind,omitempty"`
	Locator          string   `json:"locator,omitempty"`
	ArtifactDigest   string   `json:"artifact_digest,omitempty"`
	Outcome          string   `json:"outcome,omitempty"`
	Query            string   `json:"query,omitempty"`
	ExactID          string   `json:"exact_id,omitempty"`
	After            string   `json:"after,omitempty"`
	Before           string   `json:"before,omitempty"`
	Limit            int      `json:"limit,omitempty"`
	Offset           int      `json:"offset,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
}

var ErrConflict = errors.New("conflict: stale revision, claim ownership or idempotency payload")
var ErrForbidden = errors.New("forbidden: credential does not authorize this operation")

var (
	TaskStates   = []string{"planned", "active", "blocked", "implemented", "committed", "tested", "verified", "released"}
	SourceKinds  = []string{"git_commit", "pull_request", "build", "test", "ci", "log", "human", "inference", "release"}
	Outcomes     = []string{"pass", "fail", "unknown"}
	RecordKinds  = []string{"decision", "issue", "observation", "handoff", "idea", "note"}
	ClaimMinutes = 30
)

func IsWrite(tool string) bool {
	switch tool {
	case "create_project", "create_task", "start_session", "checkpoint_session", "claim_task", "release_task", "update_task", "append_event", "record_decision", "record_issue", "record_observation", "record_idea", "record_note", "attach_evidence", "record_test_result", "finish_session", "attest_evidence":
		return true
	}
	return false
}

func CanonicalRepository(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 1000 {
		return "", errors.New("repository exceeds 1000 characters")
	}
	if strings.HasPrefix(raw, "local:") {
		if !UUID(strings.TrimPrefix(raw, "local:")) {
			return "", errors.New("local identity must be local:<uuid>")
		}
		return raw, nil
	}
	if strings.HasPrefix(raw, "git@") {
		raw = "ssh://git@" + strings.Replace(strings.TrimPrefix(raw, "git@"), ":", "/", 1)
	}
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "ssh") {
		return "", errors.New("use canonical https/ssh remote or explicit local:<uuid>")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return "", errors.New("remote must not contain query, fragment or custom port")
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if path == "" || strings.Contains(path, "..") {
		return "", errors.New("invalid repository path")
	}
	if strings.EqualFold(u.Hostname(), "github.com") {
		path = strings.ToLower(path)
	}
	return strings.ToLower(u.Hostname()) + "/" + path, nil
}

func UUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func Revision(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}

func validate(a Input) error {
	for _, v := range []string{a.ProjectID, a.TaskID, a.SessionID, a.EvidenceID, a.Supersedes, a.ExactID} {
		if v != "" && !UUID(v) {
			return errors.New("invalid UUID")
		}
	}
	if a.GitRevision != "" && !Revision(a.GitRevision) {
		return errors.New("git_revision must be a full commit SHA")
	}
	if len(a.Body) > 8000 || len(a.Title) > 500 || len(a.Query) > 500 || len(a.Agent) > 100 || len(a.Branch) > 500 || len(a.Locator) > 2000 || len(a.ArtifactDigest) > 200 || len(a.Kind) > 100 {
		return errors.New("input exceeds limit")
	}
	if a.Offset < 0 || a.Offset > 10000 || a.Limit < 0 || a.Limit > 100 {
		return errors.New("invalid pagination: limit <=100, offset <=10000")
	}
	for _, s := range []string{a.After, a.Before} {
		if s != "" {
			if _, e := time.Parse(time.RFC3339, s); e != nil {
				return errors.New("time filter must be RFC3339")
			}
		}
	}
	for _, v := range a.DependsOn {
		if !UUID(v) {
			return errors.New("invalid dependency UUID")
		}
	}
	if len(a.DependsOn) > 50 {
		return errors.New("too many dependencies")
	}
	return nil
}

// normalize lowercases identifiers so every backend compares UUIDs and
// revisions identically. The idempotency hash is taken before normalization,
// so retries of payloads accepted by earlier releases still replay.
func normalize(a Input) Input {
	for _, f := range []*string{&a.ProjectID, &a.TaskID, &a.SessionID, &a.EvidenceID, &a.Supersedes, &a.ExactID, &a.GitRevision} {
		*f = strings.ToLower(*f)
	}
	if len(a.DependsOn) > 0 {
		deps := make([]string, len(a.DependsOn))
		for i, v := range a.DependsOn {
			deps[i] = strings.ToLower(v)
		}
		a.DependsOn = deps
	}
	return a
}

// ValidateCredentials applies the startup policy shared by both editions.
func ValidateCredentials(ps []Principal) error {
	if len(ps) == 0 {
		return errors.New("at least one credential required")
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if len(p.Token) < 32 || p.ID == "" || len(p.ID) > 200 || p.Owner || len(p.Projects) == 0 || seen[p.Token] || seen["principal:"+p.ID] {
			return errors.New("invalid or duplicate agent credential; owner privileges forbidden on HTTP")
		}
		seen[p.Token] = true
		seen["principal:"+p.ID] = true
	}
	return nil
}

// Supersedable lists the record kinds a new record of the given kind may
// replace. A decision may replace an idea: adopting a proposal resolves it
// without rewriting it. Nothing else crosses kinds.
func Supersedable(kind string) []string {
	if kind == "decision" {
		return []string{"decision", "idea"}
	}
	return []string{kind}
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{30,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|api[_-]?key|access[_-]?token|auth[_-]?token|bearer)\b["']?\s*[:=]?\s*["']?[A-Za-z0-9+/_=-]{20,}`),
}

// ErrSecret is returned when a write looks like it carries a credential.
var ErrSecret = errors.New("content looks like a credential (private key, token or password) and was not stored; describe it without the value")

// ScreenSecrets refuses text that looks like a credential. It is deliberately
// conservative: a rejected note can be rewritten, a stored secret cannot be
// un-stored from append-only history.
func ScreenSecrets(parts ...string) error {
	for _, text := range parts {
		for _, re := range secretPatterns {
			if re.MatchString(text) {
				return ErrSecret
			}
		}
	}
	return nil
}
