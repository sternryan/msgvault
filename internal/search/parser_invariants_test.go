package search

import (
	"testing"
	"time"
)

// TestParse_AlwaysReturnsNonNil verifies that Parse never returns nil,
// regardless of input (including empty, whitespace-only, or garbage).
func TestParse_AlwaysReturnsNonNil(t *testing.T) {
	inputs := []string{
		"",
		"   ",
		"\t",
		"from:",
		"::::",
		"\"",
		"'",
		"a",
		"from:alice@example.com",
		`"quoted"`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			got := Parse(input)
			if got == nil {
				t.Errorf("Parse(%q) returned nil, want non-nil", input)
			}
		})
	}
}

// TestTokenize_EmptyQuotedPhrase verifies that an empty quoted phrase ""
// produces no tokens (empty current buffer when closing quote fires).
func TestTokenize_EmptyQuotedPhrase(t *testing.T) {
	tokens := tokenize(`""`)
	// The quoted phrase is empty — current.Len()=0 when closing quote fires,
	// so nothing is appended.
	if len(tokens) != 0 {
		t.Errorf(`tokenize(`+"`"+"\"\""+"`"+`) = %v, want []`, tokens)
	}
}

// TestTokenize_OperatorWithEmptyQuotedValue verifies that op:"" produces one
// token (the full op:"" string), which unquotes to an empty value.
func TestTokenize_OperatorWithEmptyQuotedValue(t *testing.T) {
	tokens := tokenize(`from:""`)
	if len(tokens) != 1 {
		t.Fatalf(`tokenize(`+"`from:\"\"`"+`) = %v, want 1 token`, tokens)
	}
	if tokens[0] != `from:""` {
		t.Errorf("token = %q, want %q", tokens[0], `from:""`)
	}
}

// TestParse_OperatorWithEmptyQuotedValue verifies that from:"" appends an
// empty string to FromAddrs (the double-unquote of "" = "").
func TestParse_OperatorWithEmptyQuotedValue(t *testing.T) {
	got := Parse(`from:""`)
	if len(got.FromAddrs) != 1 {
		t.Fatalf("expected 1 FromAddr, got %d: %v", len(got.FromAddrs), got.FromAddrs)
	}
	if got.FromAddrs[0] != "" {
		t.Errorf("FromAddrs[0] = %q, want %q", got.FromAddrs[0], "")
	}
}

// TestTokenize_TextThenEmptyQuote verifies that text immediately before an
// empty quoted phrase ("") flushes the text as a token, then produces nothing
// from the empty quote.
func TestTokenize_TextThenEmptyQuote(t *testing.T) {
	tokens := tokenize(`hello""`)
	// "hello" is flushed when the opening " is seen (current.Len()>0 && !afterColon).
	// Then the empty quoted phrase produces nothing (current.Len()=0 at close).
	if len(tokens) != 1 || tokens[0] != "hello" {
		t.Errorf(`tokenize("hello\"\"") = %v, want ["hello"]`, tokens)
	}
}

// TestTokenize_QuoteAtEndOfOperatorValue verifies that a query like
// `from:alice"` (unmatched quote after operator value) doesn't crash.
func TestTokenize_UnmatchedQuoteAfterValue(t *testing.T) {
	// Should not panic.
	tokens := tokenize(`from:alice"`)
	if tokens == nil {
		t.Error("tokenize returned nil")
	}
}

// TestParse_MultipleSubjectsAccumulate verifies that multiple subject: operators
// accumulate into the SubjectTerms slice in order.
func TestParse_MultipleSubjectsAccumulate(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "two subject terms",
			query: "subject:alpha subject:beta",
			want:  []string{"alpha", "beta"},
		},
		{
			name:  "three subject terms mixed with text",
			query: "subject:first foo subject:second bar subject:third",
			want:  []string{"first", "second", "third"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if len(got.SubjectTerms) != len(tt.want) {
				t.Fatalf("SubjectTerms = %v, want %v", got.SubjectTerms, tt.want)
			}
			for i := range tt.want {
				if got.SubjectTerms[i] != tt.want[i] {
					t.Errorf("SubjectTerms[%d] = %q, want %q", i, got.SubjectTerms[i], tt.want[i])
				}
			}
		})
	}
}

// TestParse_ManyFromAddrs verifies that the parser accumulates a large number
// of from: operators into FromAddrs without loss or reordering.
func TestParse_ManyFromAddrs(t *testing.T) {
	addrs := []string{
		"a@example.com", "b@example.com", "c@example.com",
		"d@example.com", "e@example.com", "f@example.com",
	}
	query := ""
	for _, a := range addrs {
		query += "from:" + a + " "
	}
	got := Parse(query)
	if len(got.FromAddrs) != len(addrs) {
		t.Fatalf("FromAddrs = %v, want %v", got.FromAddrs, addrs)
	}
	for i, want := range addrs {
		if got.FromAddrs[i] != want {
			t.Errorf("FromAddrs[%d] = %q, want %q", i, got.FromAddrs[i], want)
		}
	}
}

// TestParse_TextTermsDoNotContainOperatorFields verifies that when operators
// are used, their values do NOT appear in TextTerms.
func TestParse_OperatorValuesNotInTextTerms(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"from not in text", "from:alice@example.com"},
		{"to not in text", "to:bob@example.com"},
		{"subject not in text", "subject:urgent"},
		{"label not in text", "label:INBOX"},
		{"has not in text", "has:attachment"},
		{"before not in text", "before:2024-01-01"},
		{"after not in text", "after:2024-01-01"},
		{"larger not in text", "larger:5M"},
		{"smaller not in text", "smaller:100K"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if len(got.TextTerms) != 0 {
				t.Errorf("TextTerms should be empty, got %v", got.TextTerms)
			}
		})
	}
}

// TestParse_OperatorsCaseInsensitiveOperatorName verifies the cc and bcc
// operators are case-insensitive, and the address value is lowercased.
func TestParse_CcBccCaseInsensitiveOperator(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantCc   []string
		wantBcc  []string
	}{
		{
			name:    "CC uppercase",
			query:   "CC:alice@example.com",
			wantCc:  []string{"alice@example.com"},
		},
		{
			name:    "BCC uppercase",
			query:   "BCC:bob@example.com",
			wantBcc: []string{"bob@example.com"},
		},
		{
			name:    "Cc mixed case",
			query:   "Cc:carol@example.com",
			wantCc:  []string{"carol@example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if len(tt.wantCc) > 0 {
				if len(got.CcAddrs) != len(tt.wantCc) || got.CcAddrs[0] != tt.wantCc[0] {
					t.Errorf("CcAddrs = %v, want %v", got.CcAddrs, tt.wantCc)
				}
			}
			if len(tt.wantBcc) > 0 {
				if len(got.BccAddrs) != len(tt.wantBcc) || got.BccAddrs[0] != tt.wantBcc[0] {
					t.Errorf("BccAddrs = %v, want %v", got.BccAddrs, tt.wantBcc)
				}
			}
		})
	}
}

// TestParse_LabelShorthandAccumulates verifies that l: (shorthand) and label:
// both accumulate into the same Labels slice in insertion order.
func TestParse_LabelShorthandMixedWithFull(t *testing.T) {
	got := Parse("l:INBOX label:important l:sent")
	want := []string{"INBOX", "important", "sent"}
	if len(got.Labels) != len(want) {
		t.Fatalf("Labels = %v, want %v", got.Labels, want)
	}
	for i, w := range want {
		if got.Labels[i] != w {
			t.Errorf("Labels[%d] = %q, want %q", i, got.Labels[i], w)
		}
	}
}

// TestParseRelativeDate_LargeAmounts verifies that parseRelativeDate handles
// large numeric amounts without overflow issues in practice.
func TestParseRelativeDate_LargeAmounts(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		value    string
		wantYear int
	}{
		{"100d", 2025},
		{"52w", 2024},
		{"24m", 2023},
		{"10y", 2015},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got := parseRelativeDate(tt.value, fixedNow)
			if got == nil {
				t.Fatalf("parseRelativeDate(%q) = nil, want non-nil", tt.value)
			}
			if got.Year() != tt.wantYear {
				t.Errorf("parseRelativeDate(%q).Year() = %d, want %d", tt.value, got.Year(), tt.wantYear)
			}
			// Result must be in the past relative to now
			if !got.Before(fixedNow) {
				t.Errorf("parseRelativeDate(%q) = %v is not before now (%v)", tt.value, got, fixedNow)
			}
		})
	}
}

// TestParse_BothLargerAndSmaller verifies that larger: and smaller: can both
// be set in the same query without interfering.
func TestParse_BothSizeFilters(t *testing.T) {
	got := Parse("larger:1M smaller:10M")
	if got.LargerThan == nil {
		t.Fatal("LargerThan should be set")
	}
	if got.SmallerThan == nil {
		t.Fatal("SmallerThan should be set")
	}
	if *got.LargerThan != 1*1024*1024 {
		t.Errorf("LargerThan = %d, want %d", *got.LargerThan, 1*1024*1024)
	}
	if *got.SmallerThan != 10*1024*1024 {
		t.Errorf("SmallerThan = %d, want %d", *got.SmallerThan, 10*1024*1024)
	}
}

// TestParse_BothAfterAndBeforeDates verifies that after: and before: can
// both be set simultaneously.
func TestParse_BothDateFilters(t *testing.T) {
	got := Parse("after:2024-01-01 before:2024-12-31")
	if got.AfterDate == nil {
		t.Fatal("AfterDate should be set")
	}
	if got.BeforeDate == nil {
		t.Fatal("BeforeDate should be set")
	}
	if got.AfterDate.Year() != 2024 || got.AfterDate.Month() != 1 || got.AfterDate.Day() != 1 {
		t.Errorf("AfterDate = %v, want 2024-01-01", got.AfterDate)
	}
	if got.BeforeDate.Year() != 2024 || got.BeforeDate.Month() != 12 || got.BeforeDate.Day() != 31 {
		t.Errorf("BeforeDate = %v, want 2024-12-31", got.BeforeDate)
	}
}

// TestParse_SubdomainEmailAddresses verifies that complex email addresses with
// subdomains are preserved exactly (after lowercasing).
func TestParse_SubdomainEmails(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"from:user@mail.example.co.uk", "user@mail.example.co.uk"},
		{"to:admin@sub.domain.org", "admin@sub.domain.org"},
		{"cc:SUPPORT@COMPANY.IO", "support@company.io"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := Parse(tt.query)
			var addr string
			switch {
			case len(got.FromAddrs) > 0:
				addr = got.FromAddrs[0]
			case len(got.ToAddrs) > 0:
				addr = got.ToAddrs[0]
			case len(got.CcAddrs) > 0:
				addr = got.CcAddrs[0]
			}
			if addr != tt.want {
				t.Errorf("addr = %q, want %q", addr, tt.want)
			}
		})
	}
}

// TestParse_QuotedPhraseWithSpecialChars verifies that special characters
// inside quoted phrases are preserved as-is in TextTerms.
func TestParse_QuotedPhraseSpecialChars(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"exclamation", `"hello!"`, "hello!"},
		{"question mark", `"is this working?"`, "is this working?"},
		{"ampersand", `"R&D update"`, "R&D update"},
		{"parentheses", `"(action required)"`, "(action required)"},
		{"slash", `"Q3/Q4 review"`, "Q3/Q4 review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			if len(got.TextTerms) != 1 {
				t.Fatalf("TextTerms = %v, want [%q]", got.TextTerms, tt.want)
			}
			if got.TextTerms[0] != tt.want {
				t.Errorf("TextTerms[0] = %q, want %q", got.TextTerms[0], tt.want)
			}
		})
	}
}
