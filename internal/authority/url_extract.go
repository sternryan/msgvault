package authority

import (
	"regexp"
	"strings"
)

// MIRROR of triage/filters.go::ExtractURLs (kept in sync; cannot import
// triage due to AUTHGRAPH-03 reverse dependency — Plan 03 will rewire
// triage to import authority, so authority MUST NOT import triage).
//
// Returns deduplicated URLs in source order. Trailing punctuation
// (.,;)) commonly attached by prose is stripped before dedup.
var urlRe = regexp.MustCompile(`https?://[^\s)>"']+`)

func extractURLs(body string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, raw := range urlRe.FindAllString(body, -1) {
		clean := strings.TrimRight(raw, ".,;)")
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}
