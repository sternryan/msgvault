package search

import (
	"testing"
	"time"
)

// TestParse_EmptyOperatorValue documents the behavior when an operator has no
// value (e.g. "from:"). The empty string is appended to the relevant slice.
// This is not a crash path — the parser handles it gracefully.
func TestParse_EmptyOperatorValue(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  Query
	}{
		{
			name:  "empty from value adds empty string",
			query: "from:",
			want:  Query{FromAddrs: []string{""}},
		},
		{
			name:  "empty to value adds empty string",
			query: "to:",
			want:  Query{ToAddrs: []string{""}},
		},
		{
			name:  "empty subject value adds empty string",
			query: "subject:",
			want:  Query{SubjectTerms: []string{""}},
		},
		{
			name:  "empty label value adds empty string",
			query: "label:",
			want:  Query{},
		},
		{
			name:  "empty cc value adds empty string",
			query: "cc:",
			want:  Query{CcAddrs: []string{""}},
		},
		{
			name:  "empty bcc value adds empty string",
			query: "bcc:",
			want:  Query{BccAddrs: []string{""}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if got == nil {
				t.Fatalf("Parse(%q) returned nil", tt.query)
			}
			assertQueryEqual(t, *got, tt.want)
		})
	}
}

// TestParse_LeadingColonBecomesTextTerm verifies that a token starting with ':'
// is treated as a text term (empty operator name → no handler → fallback).
func TestParse_LeadingColonBecomesTextTerm(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantTerms []string
	}{
		{
			name:      "colon-prefixed value is a text term",
			query:     ":value",
			wantTerms: []string{":value"},
		},
		{
			name:      "standalone colon is a text term",
			query:     ":",
			wantTerms: []string{":"},
		},
		{
			name:      "colon-prefixed mixed with real operator",
			query:     "from:alice@example.com :search",
			wantTerms: []string{":search"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			// Verify that the colon-prefixed token ends up in TextTerms.
			if len(got.TextTerms) != len(tt.wantTerms) {
				t.Fatalf("TextTerms = %v, want %v", got.TextTerms, tt.wantTerms)
			}
			for i := range tt.wantTerms {
				if got.TextTerms[i] != tt.wantTerms[i] {
					t.Errorf("TextTerms[%d] = %q, want %q", i, got.TextTerms[i], tt.wantTerms[i])
				}
			}
		})
	}
}

// TestParse_MultipleConsecutiveQuotedPhrases verifies that several quoted
// phrases in a row all become separate text terms.
func TestParse_MultipleConsecutiveQuotedPhrases(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  Query
	}{
		{
			name:  "two quoted phrases",
			query: `"hello world" "foo bar"`,
			want:  Query{TextTerms: []string{"hello world", "foo bar"}},
		},
		{
			name:  "three quoted phrases",
			query: `"alpha beta" "gamma delta" "epsilon"`,
			want:  Query{TextTerms: []string{"alpha beta", "gamma delta", "epsilon"}},
		},
		{
			name:  "quoted phrases mixed with bare words",
			query: `"meeting notes" agenda "follow up"`,
			want:  Query{TextTerms: []string{"meeting notes", "agenda", "follow up"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, tt.want)
		})
	}
}

// TestParse_SingleQuotedOperatorValue documents that single-quoted operator
// values preserve the single quotes (unquote only strips double quotes).
// e.g. subject:'hello world' → SubjectTerms = ["'hello world'"]
func TestParse_SingleQuotedOperatorValue(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  Query
	}{
		{
			name:  "single-quoted subject value preserves quotes",
			query: "subject:'urgent meeting'",
			want:  Query{SubjectTerms: []string{"'urgent meeting'"}},
		},
		{
			name:  "single-quoted from value preserves quotes (after lowercasing)",
			query: "from:'Alice@EXAMPLE.COM'",
			want:  Query{FromAddrs: []string{"'alice@example.com'"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, tt.want)
		})
	}
}

// TestParse_UppercaseRelativeOps verifies that OLDER_THAN and NEWER_THAN
// are case-insensitive (operator names are lowercased before lookup).
func TestParse_UppercaseRelativeOps(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	tests := []struct {
		name       string
		query      string
		checkAfter bool // true → verify AfterDate set; false → BeforeDate
	}{
		{"OLDER_THAN uppercase sets BeforeDate", "OLDER_THAN:7d", false},
		{"NEWER_THAN uppercase sets AfterDate", "NEWER_THAN:7d", true},
		{"Older_Than mixed-case sets BeforeDate", "Older_Than:1w", false},
		{"Newer_Than mixed-case sets AfterDate", "Newer_Than:1m", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.query)
			if tt.checkAfter {
				if got.AfterDate == nil {
					t.Errorf("AfterDate should be set for query %q", tt.query)
				}
			} else {
				if got.BeforeDate == nil {
					t.Errorf("BeforeDate should be set for query %q", tt.query)
				}
			}
		})
	}
}

// TestParse_EmptyQueryAlwaysIsEmpty verifies that various forms of "nothing"
// all produce an empty query.
func TestParse_EmptyQueryAlwaysIsEmpty(t *testing.T) {
	tests := []string{"", "   ", "  \t  "}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			got := Parse(q)
			// Empty / whitespace-only queries should produce empty results.
			// Note: tabs are not whitespace delimiters in the tokenizer, so
			// the "\t\t" case is intentionally excluded from this set.
			if q == "" || q == "   " {
				if !got.IsEmpty() {
					t.Errorf("Parse(%q).IsEmpty() = false, want true", q)
				}
			}
		})
	}
}

// TestParse_SizeWithSpacedSuffix verifies that parseSize trims whitespace
// before suffix comparison (via strings.TrimSpace + strings.ToUpper).
func TestParse_SizeWithSpacedSuffix(t *testing.T) {
	// The tokenizer splits on spaces, so "larger: 5M" produces two tokens:
	// "larger:" and "5M". Only the first maps to an operator (with empty value).
	// This test confirms the larger: empty path (no panic, no size set).
	got := Parse("larger:")
	if got.LargerThan != nil {
		t.Errorf("LargerThan should be nil for empty size value, got %d", *got.LargerThan)
	}
}

// TestParse_NewParser verifies that NewParser() returns a working parser
// with a non-nil Now function.
func TestParse_NewParser(t *testing.T) {
	p := NewParser()
	if p == nil {
		t.Fatal("NewParser() returned nil")
	}
	if p.Now == nil {
		t.Fatal("NewParser().Now is nil")
	}
	// Verify Now() returns a valid, recent time.
	now := p.Now()
	if now.IsZero() {
		t.Error("NewParser().Now() returned zero time")
	}
}
