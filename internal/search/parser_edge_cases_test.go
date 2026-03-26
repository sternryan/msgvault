package search

import (
	"testing"
	"time"

	"github.com/wesm/msgvault/internal/testutil/ptr"
)

// TestParse_CcBcc covers the cc: and bcc: operators which had zero test coverage.
func TestParse_CcBcc(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  Query
	}{
		{
			name:  "cc operator",
			query: "cc:charlie@example.com",
			want:  Query{CcAddrs: []string{"charlie@example.com"}},
		},
		{
			name:  "bcc operator",
			query: "bcc:dave@example.com",
			want:  Query{BccAddrs: []string{"dave@example.com"}},
		},
		{
			name:  "cc lowercases address",
			query: "cc:Charlie@EXAMPLE.COM",
			want:  Query{CcAddrs: []string{"charlie@example.com"}},
		},
		{
			name:  "bcc lowercases address",
			query: "bcc:DAVE@EXAMPLE.COM",
			want:  Query{BccAddrs: []string{"dave@example.com"}},
		},
		{
			name:  "multiple cc and bcc",
			query: "cc:alice@example.com cc:bob@example.com bcc:carol@example.com",
			want: Query{
				CcAddrs:  []string{"alice@example.com", "bob@example.com"},
				BccAddrs: []string{"carol@example.com"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, tt.want)
		})
	}
}

// TestParse_UnknownOperator covers the else branch when an unrecognized
// operator:value token is treated as a bare text term.
func TestParse_UnknownOperator(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  Query
	}{
		{
			name:  "unknown operator becomes text term",
			query: "unknown:value",
			want:  Query{TextTerms: []string{"unknown:value"}},
		},
		{
			name:  "unknown operator mixed with real operator",
			query: "from:alice@example.com foo:bar",
			want: Query{
				FromAddrs: []string{"alice@example.com"},
				TextTerms: []string{"foo:bar"},
			},
		},
		{
			name:  "multiple unknown operators",
			query: "xyz:a abc:b",
			want:  Query{TextTerms: []string{"xyz:a", "abc:b"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, tt.want)
		})
	}
}

// TestParseSize_EdgeCases covers the uncovered parseSize branches:
// plain integer bytes (no suffix), KB/MB/GB suffixes, and invalid inputs.
func TestParseSize_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  Query
	}{
		// Plain bytes (no suffix) - this is the ParseInt path that was uncovered.
		{
			name:  "larger plain bytes",
			query: "larger:1024",
			want:  Query{LargerThan: ptr.Int64(1024)},
		},
		{
			name:  "smaller plain bytes",
			query: "smaller:500",
			want:  Query{SmallerThan: ptr.Int64(500)},
		},
		// MB and GB suffix variants (KB and K were already tested).
		{
			name:  "larger MB suffix",
			query: "larger:5MB",
			want:  Query{LargerThan: ptr.Int64(5 * 1024 * 1024)},
		},
		{
			name:  "larger KB suffix",
			query: "larger:100KB",
			want:  Query{LargerThan: ptr.Int64(100 * 1024)},
		},
		{
			name:  "larger GB suffix",
			query: "larger:2GB",
			want:  Query{LargerThan: ptr.Int64(2 * 1024 * 1024 * 1024)},
		},
		{
			name:  "smaller GB suffix",
			query: "smaller:1GB",
			want:  Query{SmallerThan: ptr.Int64(1024 * 1024 * 1024)},
		},
		// Invalid size inputs - should leave field nil.
		{
			name:  "larger with invalid size string",
			query: "larger:xyz",
			want:  Query{},
		},
		{
			name:  "smaller with empty-suffix but non-numeric",
			query: "smaller:abc",
			want:  Query{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, tt.want)
		})
	}
}

// TestParseDate_EdgeCases covers alternate date formats and invalid dates.
func TestParseDate_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantNilDate bool
		wantYear    int
		wantMonth   time.Month
		wantDay     int
	}{
		{
			name:      "YYYY/MM/DD format",
			query:     "before:2024/06/30",
			wantYear:  2024,
			wantMonth: time.June,
			wantDay:   30,
		},
		{
			name:        "invalid date string",
			query:       "before:not-a-date",
			wantNilDate: true,
		},
		{
			name:        "invalid date with numbers",
			query:       "after:99-99-9999",
			wantNilDate: true,
		},
		{
			name:        "empty date value",
			query:       "before:",
			wantNilDate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			// Determine which date field we set based on operator
			var date *time.Time
			if got.BeforeDate != nil {
				date = got.BeforeDate
			} else {
				date = got.AfterDate
			}
			if tt.wantNilDate {
				if date != nil {
					t.Errorf("expected nil date, got %v", date)
				}
				return
			}
			if date == nil {
				t.Fatalf("expected date to be set, got nil")
			}
			if date.Year() != tt.wantYear || date.Month() != tt.wantMonth || date.Day() != tt.wantDay {
				t.Errorf("date = %v, want %04d-%02d-%02d", date, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
		})
	}
}

// TestParseRelativeDate_Invalid covers the nil return path when the
// relative date string doesn't match the expected pattern.
func TestParseRelativeDate_Invalid(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	tests := []struct {
		name  string
		query string
		field string // "after" or "before"
	}{
		{"invalid newer_than: alphabetic", "newer_than:abc", "after"},
		{"invalid older_than: alphabetic", "older_than:xyz", "before"},
		{"invalid newer_than: no unit", "newer_than:7", "after"},
		{"invalid older_than: wrong unit", "older_than:2z", "before"},
		{"invalid newer_than: empty", "newer_than:", "after"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.query)
			if tt.field == "after" && got.AfterDate != nil {
				t.Errorf("%s: AfterDate should be nil for invalid input, got %v", tt.name, got.AfterDate)
			}
			if tt.field == "before" && got.BeforeDate != nil {
				t.Errorf("%s: BeforeDate should be nil for invalid input, got %v", tt.name, got.BeforeDate)
			}
		})
	}
}

// TestParse_HasVariants covers the has: operator edge cases:
// plural "attachments", and unknown has: values.
func TestParse_HasVariants(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		wantAttachment *bool
	}{
		{
			name:           "has:attachments (plural)",
			query:          "has:attachments",
			wantAttachment: ptr.Bool(true),
		},
		{
			name:           "has:ATTACHMENT (uppercase)",
			query:          "has:ATTACHMENT",
			wantAttachment: ptr.Bool(true),
		},
		{
			name:           "has:unknown does not set attachment",
			query:          "has:unknown",
			wantAttachment: nil,
		},
		{
			name:           "has:label does not set attachment",
			query:          "has:label",
			wantAttachment: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if tt.wantAttachment == nil {
				if got.HasAttachment != nil {
					t.Errorf("HasAttachment should be nil, got %v", got.HasAttachment)
				}
			} else {
				if got.HasAttachment == nil {
					t.Errorf("HasAttachment should be %v, got nil", *tt.wantAttachment)
				} else if *got.HasAttachment != *tt.wantAttachment {
					t.Errorf("HasAttachment = %v, want %v", *got.HasAttachment, *tt.wantAttachment)
				}
			}
		})
	}
}

// TestTokenize_TextBeforeQuote covers the tokenizer branch where accumulated
// text appears immediately before a quoted phrase (no space separator), e.g.
// `hello"world"`. The tokenizer must flush the preceding text as a separate token.
func TestTokenize_TextBeforeQuote(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantTerms []string
	}{
		{
			// "hello" is flushed, then "world" is the quoted phrase content.
			name:      "text immediately before quoted phrase",
			query:     `hello"world"`,
			wantTerms: []string{"hello", "world"},
		},
		{
			// Multiple words accumulated before the quote.
			name:      "two words then quoted phrase",
			query:     `foo bar"baz"`,
			wantTerms: []string{"foo", "bar", "baz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, Query{TextTerms: tt.wantTerms})
		})
	}
}

// TestQuery_IsEmpty_AllFields verifies that IsEmpty returns false when each
// optional pointer field is set (these are not tested by the basic IsEmpty tests).
func TestQuery_IsEmpty_AllFields(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name  string
		query Query
	}{
		{"LargerThan set", Query{LargerThan: ptr.Int64(1024)}},
		{"SmallerThan set", Query{SmallerThan: ptr.Int64(1024)}},
		{"BeforeDate set", Query{BeforeDate: &now}},
		{"AfterDate set", Query{AfterDate: &now}},
		{"CcAddrs set", Query{CcAddrs: []string{"x@example.com"}}},
		{"BccAddrs set", Query{BccAddrs: []string{"x@example.com"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.query.IsEmpty() {
				t.Errorf("IsEmpty() = true, want false for query: %+v", tt.query)
			}
		})
	}
}
