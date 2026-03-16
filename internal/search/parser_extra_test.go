package search

import (
	"testing"
	"time"

	"github.com/wesm/msgvault/internal/testutil/ptr"
)

// TestParse_CaseInsensitiveOperators verifies that operator names are
// case-insensitive (e.g. FROM: and FROM: both work).
func TestParse_CaseInsensitiveOperators(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  Query
	}{
		{
			name:  "uppercase FROM",
			query: "FROM:alice@example.com",
			want:  Query{FromAddrs: []string{"alice@example.com"}},
		},
		{
			name:  "mixed case To",
			query: "To:bob@example.com",
			want:  Query{ToAddrs: []string{"bob@example.com"}},
		},
		{
			name:  "uppercase SUBJECT",
			query: "SUBJECT:urgent",
			want:  Query{SubjectTerms: []string{"urgent"}},
		},
		{
			name:  "uppercase LABEL",
			query: "LABEL:INBOX",
			want:  Query{Labels: []string{"INBOX"}},
		},
		{
			name:  "uppercase HAS:ATTACHMENT",
			query: "HAS:attachment",
			want:  Query{HasAttachment: ptr.Bool(true)},
		},
		{
			name:  "uppercase BEFORE",
			query: "BEFORE:2024-06-01",
			want:  Query{BeforeDate: ptr.Time(ptr.Date(2024, 6, 1))},
		},
		{
			name:  "uppercase AFTER",
			query: "AFTER:2024-01-01",
			want:  Query{AfterDate: ptr.Time(ptr.Date(2024, 1, 1))},
		},
		{
			name:  "uppercase LARGER",
			query: "LARGER:5M",
			want:  Query{LargerThan: ptr.Int64(5 * 1024 * 1024)},
		},
		{
			name:  "uppercase SMALLER",
			query: "SMALLER:100K",
			want:  Query{SmallerThan: ptr.Int64(100 * 1024)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, tt.want)
		})
	}
}

// TestParse_WhitespaceEdgeCases tests queries with unusual whitespace.
func TestParse_WhitespaceEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantEmpty bool
		wantTerms []string
	}{
		{
			name:      "empty string",
			query:     "",
			wantEmpty: true,
		},
		{
			name:      "only spaces",
			query:     "   ",
			wantEmpty: true,
		},
		{
			name:      "only tabs",
			query:     "\t\t",
			wantTerms: []string{"\t\t"}, // tabs are not whitespace delimiters
		},
		{
			name:      "leading and trailing spaces",
			query:     "  hello  ",
			wantTerms: []string{"hello"},
		},
		{
			name:      "multiple spaces between terms",
			query:     "hello   world",
			wantTerms: []string{"hello", "world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if tt.wantEmpty {
				if !got.IsEmpty() {
					t.Errorf("expected empty query for %q, got %+v", tt.query, got)
				}
				return
			}
			assertQueryEqual(t, *got, Query{TextTerms: tt.wantTerms})
		})
	}
}

// TestParse_UnicodeTerms verifies that unicode text is preserved as-is in TextTerms.
func TestParse_UnicodeTerms(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantTerms []string
	}{
		{
			name:      "accented characters",
			query:     "café résumé",
			wantTerms: []string{"café", "résumé"},
		},
		{
			name:      "chinese characters",
			query:     "你好 世界",
			wantTerms: []string{"你好", "世界"},
		},
		{
			name:      "emoji",
			query:     "hello 🎉",
			wantTerms: []string{"hello", "🎉"},
		},
		{
			name:      "mixed unicode and ascii",
			query:     "from:alice@example.com привет",
			wantTerms: []string{"привет"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if len(tt.query) > 0 {
				if len(tt.wantTerms) == 1 && got.FromAddrs != nil {
					// mixed case — just check text terms
					if len(got.TextTerms) != 1 || got.TextTerms[0] != tt.wantTerms[0] {
						t.Errorf("TextTerms = %v, want %v", got.TextTerms, tt.wantTerms)
					}
					return
				}
			}
			assertQueryEqual(t, *got, Query{TextTerms: tt.wantTerms})
		})
	}
}

// TestParse_UnclosedQuote verifies that an unclosed quote does not panic
// and results in reasonable output.
func TestParse_UnclosedQuote(t *testing.T) {
	// The parser should not panic on malformed input.
	// An unclosed quote means everything inside is consumed as one token.
	tests := []struct {
		name  string
		query string
	}{
		{name: "unclosed double quote", query: `"hello world`},
		{name: "unclosed single quote", query: `'hello world`},
		{name: "operator with unclosed quote", query: `from:"alice@example.com`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Must not panic
			got := Parse(tt.query)
			if got == nil {
				t.Error("Parse returned nil")
			}
		})
	}
}

// TestParse_AccountIDNotSetByParser verifies that AccountID is never set
// by the parser (it is set externally by the TUI/UI layer, not by query parsing).
func TestParse_AccountIDNotSetByParser(t *testing.T) {
	queries := []string{
		"in:account",
		"account:1",
		"from:alice@example.com",
		`"hello world"`,
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			got := Parse(q)
			if got.AccountID != nil {
				t.Errorf("AccountID should never be set by parser, got %d", *got.AccountID)
			}
		})
	}
}

// TestQuery_IsEmpty_AccountIDNotChecked documents that IsEmpty does NOT check
// AccountID — a query with only AccountID set is considered empty by IsEmpty.
// This is intentional: AccountID is injected by the UI, not the query string.
func TestQuery_IsEmpty_AccountIDNotChecked(t *testing.T) {
	id := int64(42)
	q := Query{AccountID: &id}
	if !q.IsEmpty() {
		t.Error("IsEmpty() should return true when only AccountID is set (AccountID is not a query filter)")
	}
}

// TestParse_RelativeDates_AllUnits verifies newer_than supports all units
// by checking that each returns a non-nil AfterDate relative to the fixed time.
func TestParse_RelativeDates_AllUnits(t *testing.T) {
	fixedNow := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	tests := []struct {
		name      string
		query     string
		checkDate func(q *Query) *time.Time
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		{
			name:      "newer_than 30 days",
			query:     "newer_than:30d",
			checkDate: func(q *Query) *time.Time { return q.AfterDate },
			wantYear:  2025,
			wantMonth: time.February,
			wantDay:   13,
		},
		{
			name:      "newer_than 4 weeks",
			query:     "newer_than:4w",
			checkDate: func(q *Query) *time.Time { return q.AfterDate },
			wantYear:  2025,
			wantMonth: time.February,
			wantDay:   15,
		},
		{
			name:      "older_than 3 months",
			query:     "older_than:3m",
			checkDate: func(q *Query) *time.Time { return q.BeforeDate },
			wantYear:  2024,
			wantMonth: time.December,
			wantDay:   15,
		},
		{
			name:      "newer_than 2 years",
			query:     "newer_than:2y",
			checkDate: func(q *Query) *time.Time { return q.AfterDate },
			wantYear:  2023,
			wantMonth: time.March,
			wantDay:   15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.query)
			date := tt.checkDate(got)
			if date == nil {
				t.Fatalf("expected date to be set, got nil")
			}
			if date.Year() != tt.wantYear || date.Month() != tt.wantMonth || date.Day() != tt.wantDay {
				t.Errorf("date = %v, want %04d-%02d-%02d", date, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
		})
	}
}
