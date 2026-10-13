package mariadb

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// searchText is the stored search representation: NFC then lowercase, the
// same transformation PostgreSQL applies with normalize() and lower().
func searchText(parts ...string) string {
	return strings.ToLower(norm.NFC.String(strings.Join(parts, " ")))
}

// searchTerms derives the literal phrase and per-term substrings for a query.
// Without stemming dictionaries, long ASCII words drop one common English
// suffix so "migrate" still finds "migrations"; other scripts match literally.
func searchTerms(q string) (string, []string) {
	phrase := strings.TrimSpace(searchText(q))
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

func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}
