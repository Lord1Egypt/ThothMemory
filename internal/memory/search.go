package memory

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// SearchText is the stored search representation: NFC then lowercase, the
// same transformation PostgreSQL applies with normalize() and lower().
func SearchText(parts ...string) string {
	return strings.ToLower(norm.NFC.String(strings.Join(parts, " ")))
}

// SearchTerms derives the literal phrase and per-term substrings for a query.
// Without stemming dictionaries, long ASCII words drop one common English
// suffix so "migrate" still finds "migrations"; other scripts match literally.
func SearchTerms(q string) (string, []string) {
	phrase := strings.TrimSpace(SearchText(q))
	seen := map[string]bool{}
	var terms []string
	for _, f := range strings.Fields(phrase) {
		f = strings.Trim(f, `"'()`)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, stem(f))
		if len(terms) == 16 {
			break
		}
	}
	return phrase, terms
}

var suffixes = []string{"ations", "ation", "ings", "ing", "ions", "ion", "ies", "ed", "es", "e", "s"}

func stem(w string) string {
	if len(w) < 5 {
		return w
	}
	for _, c := range w {
		if c < 'a' || c > 'z' {
			return w
		}
	}
	for _, s := range suffixes {
		if strings.HasSuffix(w, s) && len(w)-len(s) >= 4 {
			return w[:len(w)-len(s)]
		}
	}
	return w
}

func LikePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// NewID returns a random (version 4) UUID.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails: Go aborts the process instead
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Timestamp renders a UTC time exactly as PostgreSQL's to_jsonb does, so all
// editions return identical timestamp strings.
func Timestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.999999") + "+00:00"
}

// Column lists mirror PostgreSQL to_jsonb(row) minus search columns, so every
// edition returns the same keys for each entity.
const (
	ProjectColumns     = "id,name,repository,created_at,event_count,event_budget"
	TaskColumns        = "id,project_id,title,state,revision,git_revision,claimed_by,updated_at,claim_expires_at"
	SessionColumns     = "id,project_id,agent_id,principal_id,branch,git_revision,started_at,finished_at,revision"
	RecordColumns      = "id,project_id,task_id,session_id,kind,title,body,git_revision,principal_id,agent_id,evidence_id,supersedes,created_at"
	EvidenceColumns    = "id,project_id,task_id,source_kind,locator,git_revision,artifact_digest,outcome,summary,reported_by,created_at"
	EventColumns       = "id,project_id,task_id,session_id,kind,body,principal_id,agent_id,git_revision,created_at"
	AttestationColumns = "id,project_id,evidence_id,principal_id,statement,created_at"
	ImportColumns      = "id,project_id,export_id,source_edition,source_version,format_version,checksum,counts,principal_id,imported_at"
)
