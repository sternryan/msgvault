// Package search provides Gmail-like search query parsing.
package search

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// reRelativeDate matches relative date values like "7d", "2w", "1m", "1y".
// Pre-compiled at package level to avoid repeated regexp compilation overhead.
var reRelativeDate = regexp.MustCompile(`^(\d+)([dwmy])$`)

// Query represents a parsed search query with all supported filters.
type Query struct {
	TextTerms     []string   // Full-text search terms
	FromAddrs     []string   // from: filters
	ToAddrs       []string   // to: filters
	CcAddrs       []string   // cc: filters
	BccAddrs      []string   // bcc: filters
	SubjectTerms  []string   // subject: filters
	Labels        []string   // label: filters
	HasAttachment *bool      // has:attachment
	BeforeDate    *time.Time // before: filter
	AfterDate     *time.Time // after: filter
	LargerThan    *int64     // larger: filter (bytes)
	SmallerThan   *int64     // smaller: filter (bytes)
	AccountID     *int64     // in: account filter
	HideDeleted   bool       // exclude messages where deleted_from_source_at IS NOT NULL
}

// IsEmpty returns true if the query has no search criteria.
func (q *Query) IsEmpty() bool {
	return len(q.TextTerms) == 0 &&
		len(q.FromAddrs) == 0 &&
		len(q.ToAddrs) == 0 &&
		len(q.CcAddrs) == 0 &&
		len(q.BccAddrs) == 0 &&
		len(q.SubjectTerms) == 0 &&
		len(q.Labels) == 0 &&
		q.HasAttachment == nil &&
		q.BeforeDate == nil &&
		q.AfterDate == nil &&
		q.LargerThan == nil &&
		q.SmallerThan == nil
}

// toLowerFast returns a lowercase ASCII version of s, avoiding allocation
// when s is already lowercase (the common case for operator names).
func toLowerFast(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			// Found uppercase: allocate and convert the rest.
			b := make([]byte, len(s))
			copy(b, s[:i])
			b[i] = c + 32
			for i++; i < len(s); i++ {
				c = s[i]
				if c >= 'A' && c <= 'Z' {
					b[i] = c + 32
				} else {
					b[i] = c
				}
			}
			return string(b)
		}
	}
	return s // already lowercase, no allocation
}

// applyOperator dispatches the operator:value pair to the appropriate handler
// using a switch statement, which compiles to an efficient jump table and avoids
// the overhead of map hashing and indirect function calls.
func applyOperator(q *Query, op, value string, now *time.Time) bool {
	switch op {
	case "from":
		q.FromAddrs = append(q.FromAddrs, strings.ToLower(value))
	case "to":
		q.ToAddrs = append(q.ToAddrs, strings.ToLower(value))
	case "cc":
		q.CcAddrs = append(q.CcAddrs, strings.ToLower(value))
	case "bcc":
		q.BccAddrs = append(q.BccAddrs, strings.ToLower(value))
	case "subject":
		q.SubjectTerms = append(q.SubjectTerms, value)
	case "label", "l":
		if v := strings.TrimSpace(value); v != "" {
			q.Labels = append(q.Labels, v)
		}
	case "has":
		if low := strings.ToLower(value); low == "attachment" || low == "attachments" {
			b := true
			q.HasAttachment = &b
		}
	case "before":
		if t := parseDate(value); t != nil {
			q.BeforeDate = t
		}
	case "after":
		if t := parseDate(value); t != nil {
			q.AfterDate = t
		}
	case "older_than":
		if now.IsZero() {
			*now = time.Now().UTC()
		}
		if t := parseRelativeDate(value, *now); t != nil {
			q.BeforeDate = t
		}
	case "newer_than":
		if now.IsZero() {
			*now = time.Now().UTC()
		}
		if t := parseRelativeDate(value, *now); t != nil {
			q.AfterDate = t
		}
	case "larger":
		if size := parseSize(value); size != nil {
			q.LargerThan = size
		}
	case "smaller":
		if size := parseSize(value); size != nil {
			q.SmallerThan = size
		}
	default:
		return false
	}
	return true
}

// Parser holds configuration for query parsing.
type Parser struct {
	Now func() time.Time // Time source (mockable for testing)
}

// NewParser creates a Parser with default settings.
func NewParser() *Parser {
	return &Parser{Now: func() time.Time { return time.Now().UTC() }}
}

// Parse parses a Gmail-like search query string into a Query object.
//
// Supported operators:
//   - from:, to:, cc:, bcc: - address filters
//   - subject: - subject text search
//   - label: or l: - label filter
//   - has:attachment - attachment filter
//   - before:, after: - date filters (YYYY-MM-DD)
//   - older_than:, newer_than: - relative date filters (e.g., 7d, 2w, 1m, 1y)
//   - larger:, smaller: - size filters (e.g., 5M, 100K)
//   - Bare words and "quoted phrases" - full-text search
func (p *Parser) Parse(queryStr string) *Query {
	q := &Query{}
	// When a custom Now function is provided, call it eagerly so callers can
	// rely on exactly one Now() invocation per Parse call (e.g. for testing).
	// When Now is nil (the defaultParser case), skip the syscall here and fetch
	// time.Now() lazily only if a time-dependent operator (older_than/newer_than)
	// is actually present in the query.
	var now time.Time
	if p.Now != nil {
		now = p.Now()
	}

	tokens := tokenize(queryStr)

	for _, token := range tokens {
		if isQuotedPhrase(token) {
			q.TextTerms = append(q.TextTerms, unquote(token))
			continue
		}

		if idx := strings.Index(token, ":"); idx != -1 {
			op := toLowerFast(token[:idx])
			value := unquote(token[idx+1:])

			if !applyOperator(q, op, value, &now) {
				q.TextTerms = append(q.TextTerms, token)
			}
			continue
		}

		q.TextTerms = append(q.TextTerms, token)
	}

	return q
}

// defaultParser is a reusable parser instance for the Parse convenience function.
// Safe for concurrent use: Parser.Parse does not mutate the Parser struct.
// Now is nil so that Parse avoids the time.Now() syscall for the common case of
// queries that contain no relative-date operators (older_than/newer_than).
var defaultParser = &Parser{Now: nil}

// Parse is a convenience function that parses using default settings.
func Parse(queryStr string) *Query {
	return defaultParser.Parse(queryStr)
}

// unquote removes surrounding double quotes from a string if present.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// isQuotedPhrase returns true if the token is a double-quoted phrase.
func isQuotedPhrase(token string) bool {
	return len(token) > 2 && token[0] == '"' && token[len(token)-1] == '"'
}

// tokenize splits a query string, preserving quoted phrases and operator:value pairs.
// Handles cases like subject:"foo bar" where the operator and quoted value should stay together.
//
// Uses byte-level iteration instead of rune-level: the only characters with special
// meaning ('"', '\”, ' ', ':') are single-byte ASCII, and multi-byte UTF-8 sequences
// never contain those byte values, so byte-by-byte processing is both correct and
// avoids the UTF-8 decoding overhead of a range loop plus the ASCII branch in WriteRune.
func tokenize(queryStr string) []string {
	// Fast path: no quotes → split on ASCII spaces only (the tokenizer's sole delimiter).
	// Scanning for space bytes and slicing the original string avoids the strings.Builder
	// allocation and per-byte WriteByte overhead.  Unlike strings.Fields this only splits
	// on ' ' (0x20), matching the tokenizer's behaviour of treating tabs/newlines as
	// ordinary characters rather than delimiters.
	if strings.IndexAny(queryStr, `"'`) == -1 {
		if len(queryStr) == 0 {
			return nil
		}
		// Pre-allocate to exact capacity to avoid repeated slice growth.
		// strings.Count uses SIMD internally, so this extra pass is cheaper
		// than the 2-3 reallocation+copy cycles it eliminates.
		nspaces := strings.Count(queryStr, " ")
		tokens := make([]string, 0, nspaces+1)
		start := 0
		for i := 0; i < len(queryStr); i++ {
			if queryStr[i] == ' ' {
				if i > start {
					tokens = append(tokens, queryStr[start:i])
				}
				start = i + 1
			}
		}
		if start < len(queryStr) {
			tokens = append(tokens, queryStr[start:])
		}
		return tokens
	}

	var tokens []string
	var current strings.Builder
	inQuotes := false
	var quoteChar byte
	// Track if we just saw a colon (for op:"value" handling)
	afterColon := false
	// Track if this quoted section started as op:"value" (quote immediately after colon)
	opQuoted := false

	for i := 0; i < len(queryStr); i++ {
		b := queryStr[i]
		if (b == '"' || b == '\'') && !inQuotes {
			// Start of quoted section
			inQuotes = true
			quoteChar = b
			// If we just saw a colon, this is an op:"value" case
			opQuoted = afterColon
			// If we just saw a colon, keep building the same token (op:"value" case)
			if !afterColon && current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// Include the quote in the token for op:"value" case
			if afterColon {
				current.WriteByte(b)
			}
			afterColon = false
		} else if b == quoteChar && inQuotes {
			// End of quoted section
			inQuotes = false
			// Check if this was an op:"value" case (quote started after colon)
			if opQuoted {
				// Include the closing quote and save the whole token
				current.WriteByte(b)
				tokens = append(tokens, current.String())
				current.Reset()
			} else if current.Len() > 0 {
				// Standalone quoted phrase (may contain colons, but not op:"value")
				tokens = append(tokens, "\""+current.String()+"\"")
				current.Reset()
			}
			quoteChar = 0
			opQuoted = false
		} else if b == ' ' && !inQuotes {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			afterColon = false
		} else {
			current.WriteByte(b)
			afterColon = (b == ':')
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// parseDate parses date strings like YYYY-MM-DD or YYYY/MM/DD.
func parseDate(value string) *time.Time {
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"01/02/2006",
		"02/01/2006",
	}

	value = strings.TrimSpace(value)
	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

// parseRelativeDate parses relative dates like 7d, 2w, 1m, 1y relative to now.
func parseRelativeDate(value string, now time.Time) *time.Time {
	value = strings.TrimSpace(strings.ToLower(value))
	match := reRelativeDate.FindStringSubmatch(value)
	if match == nil {
		return nil
	}

	amount, _ := strconv.Atoi(match[1])
	unit := match[2]

	var result time.Time
	switch unit {
	case "d":
		result = now.AddDate(0, 0, -amount)
	case "w":
		result = now.AddDate(0, 0, -amount*7)
	case "m":
		result = now.AddDate(0, -amount, 0)
	case "y":
		result = now.AddDate(-amount, 0, 0)
	default:
		return nil
	}

	return &result
}

// parseSize parses size strings like 5M, 100K, 1G into bytes.
func parseSize(value string) *int64 {
	value = strings.TrimSpace(strings.ToUpper(value))
	multipliers := map[string]int64{
		"K":  1024,
		"KB": 1024,
		"M":  1024 * 1024,
		"MB": 1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(value, suffix) {
			numStr := value[:len(value)-len(suffix)]
			if num, err := strconv.ParseFloat(numStr, 64); err == nil {
				result := int64(num * float64(mult))
				return &result
			}
			return nil
		}
	}

	// Plain number (bytes)
	if num, err := strconv.ParseInt(value, 10, 64); err == nil {
		return &num
	}
	return nil
}
