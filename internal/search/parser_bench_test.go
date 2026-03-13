package search

import "testing"

// BenchmarkParse measures the cost of parsing common query patterns.
// Run with: go test -tags fts5 -bench=. ./internal/search/...
func BenchmarkParse_BareWords(b *testing.B) {
	for b.Loop() {
		Parse("hello world golang")
	}
}

func BenchmarkParse_MultipleOperators(b *testing.B) {
	for b.Loop() {
		Parse(`from:alice@example.com to:bob@example.com subject:"project update" has:attachment after:2024-01-01`)
	}
}

func BenchmarkParse_QuotedPhrase(b *testing.B) {
	for b.Loop() {
		Parse(`"the quick brown fox jumps over the lazy dog"`)
	}
}

func BenchmarkParse_SizeFilters(b *testing.B) {
	for b.Loop() {
		Parse("larger:5M smaller:10M")
	}
}

func BenchmarkParse_RelativeDates(b *testing.B) {
	for b.Loop() {
		Parse("newer_than:7d older_than:1y")
	}
}

func BenchmarkParse_Complex(b *testing.B) {
	for b.Loop() {
		Parse(`from:alice@example.com to:bob@example.com cc:carol@example.com subject:"quarterly review" label:INBOX has:attachment after:2024-01-01 before:2024-12-31 larger:1M smaller:50M "budget report"`)
	}
}

// TestParseSize_InvalidNumericPrefix covers the return nil path inside
// parseSize when a recognised size suffix is found but the preceding numeric
// part cannot be parsed as a float (e.g. "abcM").
func TestParseSize_InvalidNumericPrefix(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "larger with letters before M suffix",
			query: "larger:abcM",
		},
		{
			name:  "smaller with letters before K suffix",
			query: "smaller:xyzK",
		},
		{
			name:  "larger with letters before MB suffix",
			query: "larger:fooMB",
		},
		{
			name:  "smaller with letters before GB suffix",
			query: "smaller:barGB",
		},
		{
			name:  "larger with letters before G suffix",
			query: "larger:bazG",
		},
		{
			name:  "smaller with letters before KB suffix",
			query: "smaller:quxKB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if got.LargerThan != nil {
				t.Errorf("%s: LargerThan should be nil for invalid size, got %d", tt.name, *got.LargerThan)
			}
			if got.SmallerThan != nil {
				t.Errorf("%s: SmallerThan should be nil for invalid size, got %d", tt.name, *got.SmallerThan)
			}
		})
	}
}

// TestParse_SingleQuotedPhrases covers the single-quote path in tokenize
// (the tokenizer accepts both " and ' as quote characters).
func TestParse_SingleQuotedPhrases(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantTerms []string
	}{
		{
			name:      "single-quoted phrase becomes text term",
			query:     "'hello world'",
			wantTerms: []string{"hello world"},
		},
		{
			name:      "single-quoted phrase with multiple words",
			query:     "'the quick brown fox'",
			wantTerms: []string{"the quick brown fox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			assertQueryEqual(t, *got, Query{TextTerms: tt.wantTerms})
		})
	}
}

// TestParseDate_AltFormats covers the MM/DD/YYYY and DD/MM/YYYY date formats
// accepted by parseDate (the last two format entries in the formats slice).
func TestParseDate_AltFormats(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantYear  int
		wantMonth int
		wantDay   int
	}{
		{
			// 01/02/2006 layout: month=06, day=15, year=2024
			name:      "MM/DD/YYYY format (before)",
			query:     "before:06/15/2024",
			wantYear:  2024,
			wantMonth: 6,
			wantDay:   15,
		},
		{
			// 02/01/2006 layout: day=25, month=12, year=2024
			// Day 25 > 12 so MM/DD/YYYY would fail, falling through to DD/MM/YYYY.
			name:      "DD/MM/YYYY format (after)",
			query:     "after:25/12/2024",
			wantYear:  2024,
			wantMonth: 12,
			wantDay:   25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			var date interface{ Year() int }
			if got.BeforeDate != nil {
				d := got.BeforeDate
				if d.Year() != tt.wantYear || int(d.Month()) != tt.wantMonth || d.Day() != tt.wantDay {
					t.Errorf("BeforeDate = %v, want %04d-%02d-%02d", d, tt.wantYear, tt.wantMonth, tt.wantDay)
				}
				return
			}
			if got.AfterDate != nil {
				d := got.AfterDate
				if d.Year() != tt.wantYear || int(d.Month()) != tt.wantMonth || d.Day() != tt.wantDay {
					t.Errorf("AfterDate = %v, want %04d-%02d-%02d", d, tt.wantYear, tt.wantMonth, tt.wantDay)
				}
				return
			}
			_ = date
			t.Errorf("expected date to be set, but both BeforeDate and AfterDate are nil")
		})
	}
}
