package search

import (
	"testing"
	"time"
)

// TestParse_DisplayNameInAngleBrackets verifies behavior when a display-name
// style address is used: from:"John Doe" <john@example.com>.
// The angle-bracket token has no colon, so it becomes a text term.
func TestParse_DisplayNameInAngleBrackets(t *testing.T) {
	got := Parse(`from:"John Doe" <john@example.com>`)
	if len(got.FromAddrs) != 1 || got.FromAddrs[0] != "john doe" {
		t.Errorf("FromAddrs = %v, want [john doe]", got.FromAddrs)
	}
	if len(got.TextTerms) != 1 || got.TextTerms[0] != "<john@example.com>" {
		t.Errorf("TextTerms = %v, want [<john@example.com>]", got.TextTerms)
	}
}

// TestParse_LabelPreservesCase verifies that label values are NOT lowercased,
// in contrast to from/to/cc/bcc which are always lowercased.
func TestParse_LabelPreservesCase(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"label:ImportantLabel", "ImportantLabel"},
		{"label:MixedCASE", "MixedCASE"},
		{"l:Work-Stuff", "Work-Stuff"},
		{"label:inbox", "inbox"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := Parse(tt.query)
			if len(got.Labels) != 1 {
				t.Fatalf("Labels = %v, want 1 element", got.Labels)
			}
			if got.Labels[0] != tt.want {
				t.Errorf("Labels[0] = %q, want %q (case must be preserved)", got.Labels[0], tt.want)
			}
		})
	}
}

// TestParse_ToLowercases verifies that to: values are lowercased,
// complementing the existing TestParse_FromLowercases in parser_helpers_test.go.
func TestParse_ToLowercases(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"to:Bob@EXAMPLE.COM", "bob@example.com"},
		{"to:ALICE@Example.Org", "alice@example.org"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := Parse(tt.query)
			if len(got.ToAddrs) != 1 {
				t.Fatalf("ToAddrs = %v, want 1 element", got.ToAddrs)
			}
			if got.ToAddrs[0] != tt.want {
				t.Errorf("ToAddrs[0] = %q, want %q", got.ToAddrs[0], tt.want)
			}
		})
	}
}

// TestParse_SubjectWithQuotedOperator verifies that subject:"from:alice" sets
// SubjectTerms to ["from:alice"] — the colon inside the quoted value must not
// trigger operator parsing.
func TestParse_SubjectWithQuotedOperator(t *testing.T) {
	got := Parse(`subject:"from:alice"`)
	if len(got.SubjectTerms) != 1 || got.SubjectTerms[0] != "from:alice" {
		t.Errorf("SubjectTerms = %v, want [from:alice]", got.SubjectTerms)
	}
	// The inner "from:alice" must NOT populate FromAddrs.
	if len(got.FromAddrs) != 0 {
		t.Errorf("FromAddrs should be empty, got %v", got.FromAddrs)
	}
}

// TestParse_NegativeDecimalSize verifies that parseSize handles negative
// decimal values — ParseFloat accepts them, yielding a negative byte count.
func TestParse_NegativeDecimalSize(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantLarger bool
		wantVal  int64
	}{
		{
			name:       "larger:-1M produces negative LargerThan",
			query:      "larger:-1M",
			wantLarger: true,
			wantVal:    int64(-1.0 * 1024 * 1024),
		},
		{
			name:       "smaller:-500K produces negative SmallerThan",
			query:      "smaller:-500K",
			wantLarger: false,
			wantVal:    int64(-500.0 * 1024),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			var actual *int64
			if tt.wantLarger {
				actual = got.LargerThan
			} else {
				actual = got.SmallerThan
			}
			if actual == nil {
				t.Fatalf("expected size to be set, got nil")
			}
			if *actual != tt.wantVal {
				t.Errorf("size = %d, want %d", *actual, tt.wantVal)
			}
		})
	}
}

// TestParse_ZeroWithSuffix verifies that 0K, 0M, 0G produce zero-byte sizes.
func TestParse_ZeroWithSuffix(t *testing.T) {
	tests := []struct {
		query      string
		wantLarger bool
	}{
		{"larger:0K", true},
		{"larger:0M", true},
		{"smaller:0G", false},
		{"larger:0KB", true},
		{"smaller:0MB", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := Parse(tt.query)
			var actual *int64
			if tt.wantLarger {
				actual = got.LargerThan
			} else {
				actual = got.SmallerThan
			}
			if actual == nil {
				t.Fatalf("expected size to be set for %q, got nil", tt.query)
			}
			if *actual != 0 {
				t.Errorf("size = %d, want 0 for %q", *actual, tt.query)
			}
		})
	}
}

// TestParse_ParserEquivalence verifies that the convenience Parse() function
// and NewParser().Parse() produce identical results for queries without
// relative date operators (where time.Now() is not called).
func TestParse_ParserEquivalence(t *testing.T) {
	queries := []string{
		"from:alice@example.com",
		`subject:"meeting notes"`,
		"has:attachment larger:5M",
		"before:2024-01-01 after:2023-01-01",
		`"hello world" from:bob@example.com label:INBOX`,
		"cc:charlie@example.com bcc:dave@example.com",
		"unknown:operator bare word",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			got1 := Parse(q)
			got2 := NewParser().Parse(q)
			assertQueryEqual(t, *got1, *got2)
		})
	}
}

// TestTokenize_DoubleQuoteContainsSingleQuote verifies that a single quote
// inside a double-quoted phrase does not prematurely end the phrase.
func TestTokenize_DoubleQuoteContainsSingleQuote(t *testing.T) {
	tokens := tokenize(`"it's important"`)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(tokens), tokens)
	}
	if tokens[0] != `"it's important"` {
		t.Errorf("token = %q, want %q", tokens[0], `"it's important"`)
	}
}

// TestParse_SingleQuoteInsideDoublePhrase verifies that a single quote inside
// a double-quoted phrase is preserved in the resulting TextTerm.
func TestParse_SingleQuoteInsideDoublePhrase(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{`"it's important"`, "it's important"},
		{`"can't stop won't stop"`, "can't stop won't stop"},
		{`"O'Brien's report"`, "O'Brien's report"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
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

// TestParse_AllOperatorsInOneQuery verifies that a query using all supported
// operators populates every field of the Query struct correctly.
func TestParse_AllOperatorsInOneQuery(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	query := `from:alice@example.com to:bob@example.com cc:carol@example.com bcc:dave@example.com subject:"quarterly report" label:INBOX has:attachment before:2025-06-01 after:2024-01-01 larger:1M smaller:50M "budget analysis"`
	got := p.Parse(query)

	if len(got.FromAddrs) != 1 || got.FromAddrs[0] != "alice@example.com" {
		t.Errorf("FromAddrs = %v, want [alice@example.com]", got.FromAddrs)
	}
	if len(got.ToAddrs) != 1 || got.ToAddrs[0] != "bob@example.com" {
		t.Errorf("ToAddrs = %v, want [bob@example.com]", got.ToAddrs)
	}
	if len(got.CcAddrs) != 1 || got.CcAddrs[0] != "carol@example.com" {
		t.Errorf("CcAddrs = %v, want [carol@example.com]", got.CcAddrs)
	}
	if len(got.BccAddrs) != 1 || got.BccAddrs[0] != "dave@example.com" {
		t.Errorf("BccAddrs = %v, want [dave@example.com]", got.BccAddrs)
	}
	if len(got.SubjectTerms) != 1 || got.SubjectTerms[0] != "quarterly report" {
		t.Errorf("SubjectTerms = %v, want [quarterly report]", got.SubjectTerms)
	}
	if len(got.Labels) != 1 || got.Labels[0] != "INBOX" {
		t.Errorf("Labels = %v, want [INBOX]", got.Labels)
	}
	if got.HasAttachment == nil || !*got.HasAttachment {
		t.Errorf("HasAttachment = %v, want true", got.HasAttachment)
	}
	if got.BeforeDate == nil {
		t.Error("BeforeDate should be set")
	}
	if got.AfterDate == nil {
		t.Error("AfterDate should be set")
	}
	if got.LargerThan == nil || *got.LargerThan != 1*1024*1024 {
		t.Errorf("LargerThan = %v, want %d", got.LargerThan, 1*1024*1024)
	}
	if got.SmallerThan == nil || *got.SmallerThan != 50*1024*1024 {
		t.Errorf("SmallerThan = %v, want %d", got.SmallerThan, 50*1024*1024)
	}
	if len(got.TextTerms) != 1 || got.TextTerms[0] != "budget analysis" {
		t.Errorf("TextTerms = %v, want [budget analysis]", got.TextTerms)
	}
}

// TestParse_AdjacentOperatorsNoSpace verifies that operators written without
// a separating space are treated as a single token split at the first colon.
func TestParse_AdjacentOperatorsNoSpace(t *testing.T) {
	// "from:a@a.comto:b@b.com" is one token; strings.Index finds first colon.
	// op="from", value="a@a.comto:b@b.com" (the second colon is part of value).
	got := Parse("from:a@a.comto:b@b.com")
	if len(got.FromAddrs) != 1 {
		t.Fatalf("FromAddrs = %v, want 1 element", got.FromAddrs)
	}
	if got.FromAddrs[0] != "a@a.comto:b@b.com" {
		t.Errorf("FromAddrs[0] = %q, want %q", got.FromAddrs[0], "a@a.comto:b@b.com")
	}
	// The embedded "to:b@b.com" should NOT create a ToAddr entry.
	if len(got.ToAddrs) != 0 {
		t.Errorf("ToAddrs should be empty, got %v", got.ToAddrs)
	}
}

// TestParse_MixedQuotedAndUnquotedSameOp verifies that the same operator type
// can appear with both quoted and unquoted values in one query.
func TestParse_MixedQuotedAndUnquotedSameOp(t *testing.T) {
	got := Parse(`from:alice@example.com from:"bob@example.com" to:carol@example.com`)
	if len(got.FromAddrs) != 2 {
		t.Fatalf("FromAddrs = %v, want 2 elements", got.FromAddrs)
	}
	if got.FromAddrs[0] != "alice@example.com" {
		t.Errorf("FromAddrs[0] = %q, want alice@example.com", got.FromAddrs[0])
	}
	if got.FromAddrs[1] != "bob@example.com" {
		t.Errorf("FromAddrs[1] = %q, want bob@example.com", got.FromAddrs[1])
	}
	if len(got.ToAddrs) != 1 || got.ToAddrs[0] != "carol@example.com" {
		t.Errorf("ToAddrs = %v, want [carol@example.com]", got.ToAddrs)
	}
}

// TestTokenize_OperatorValueWithMultipleColons verifies that when an
// operator value itself contains colons, only the first colon separates
// operator from value.
func TestTokenize_OperatorValueWithMultipleColons(t *testing.T) {
	tests := []struct {
		input      string
		wantTokens []string
	}{
		{
			// "from:user:extra" → one token (no spaces)
			input:      "from:user:extra",
			wantTokens: []string{"from:user:extra"},
		},
		{
			// "from:a:b:c" → one token
			input:      "from:a:b:c",
			wantTokens: []string{"from:a:b:c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.wantTokens) {
				t.Fatalf("tokenize(%q) = %v, want %v", tt.input, got, tt.wantTokens)
			}
			for i := range got {
				if got[i] != tt.wantTokens[i] {
					t.Errorf("token[%d] = %q, want %q", i, got[i], tt.wantTokens[i])
				}
			}
		})
	}
}

// TestParse_InsertionOrderTextTerms verifies that bare text terms are
// accumulated in the order they appear in the query string.
func TestParse_InsertionOrderTextTerms(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{
			"alpha beta gamma",
			[]string{"alpha", "beta", "gamma"},
		},
		{
			`from:alice@example.com first second third`,
			[]string{"first", "second", "third"},
		},
		{
			`"phrase one" bare "phrase two"`,
			[]string{"phrase one", "bare", "phrase two"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := Parse(tt.query)
			if len(got.TextTerms) != len(tt.want) {
				t.Fatalf("TextTerms = %v, want %v", got.TextTerms, tt.want)
			}
			for i := range tt.want {
				if got.TextTerms[i] != tt.want[i] {
					t.Errorf("TextTerms[%d] = %q, want %q", i, got.TextTerms[i], tt.want[i])
				}
			}
		})
	}
}

// TestParse_RealisticUserQueries verifies realistic Gmail-like search queries
// that users commonly run, testing end-to-end parsing behavior.
func TestParse_RealisticUserQueries(t *testing.T) {
	tests := []struct {
		name  string
		query string
		check func(t *testing.T, q *Query)
	}{
		{
			name:  "find newsletter from mailing list with attachment",
			query: "from:newsletter@company.com has:attachment label:newsletters",
			check: func(t *testing.T, q *Query) {
				if len(q.FromAddrs) != 1 || q.FromAddrs[0] != "newsletter@company.com" {
					t.Errorf("FromAddrs = %v", q.FromAddrs)
				}
				if q.HasAttachment == nil || !*q.HasAttachment {
					t.Error("HasAttachment should be true")
				}
				if len(q.Labels) != 1 || q.Labels[0] != "newsletters" {
					t.Errorf("Labels = %v", q.Labels)
				}
			},
		},
		{
			name:  "find large emails from last year",
			query: "larger:10M after:2024-01-01 before:2025-01-01",
			check: func(t *testing.T, q *Query) {
				if q.LargerThan == nil || *q.LargerThan != 10*1024*1024 {
					t.Errorf("LargerThan = %v, want %d", q.LargerThan, 10*1024*1024)
				}
				if q.AfterDate == nil || q.BeforeDate == nil {
					t.Error("AfterDate and BeforeDate should both be set")
				}
			},
		},
		{
			name:  "find email thread by subject",
			query: `subject:"Re: Q4 Budget Review" from:boss@company.com`,
			check: func(t *testing.T, q *Query) {
				if len(q.SubjectTerms) != 1 || q.SubjectTerms[0] != "Re: Q4 Budget Review" {
					t.Errorf("SubjectTerms = %v", q.SubjectTerms)
				}
				if len(q.FromAddrs) != 1 || q.FromAddrs[0] != "boss@company.com" {
					t.Errorf("FromAddrs = %v", q.FromAddrs)
				}
			},
		},
		{
			name:  "search for email with specific text and no attachment filter",
			query: `"action required" "please review" from:manager@company.com`,
			check: func(t *testing.T, q *Query) {
				if len(q.TextTerms) != 2 {
					t.Fatalf("TextTerms = %v, want 2 items", q.TextTerms)
				}
				if q.TextTerms[0] != "action required" || q.TextTerms[1] != "please review" {
					t.Errorf("TextTerms = %v", q.TextTerms)
				}
			},
		},
		{
			name:  "find all mail cc'd to me",
			query: "cc:me@example.com label:INBOX",
			check: func(t *testing.T, q *Query) {
				if len(q.CcAddrs) != 1 || q.CcAddrs[0] != "me@example.com" {
					t.Errorf("CcAddrs = %v", q.CcAddrs)
				}
				if len(q.Labels) != 1 || q.Labels[0] != "INBOX" {
					t.Errorf("Labels = %v", q.Labels)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			tt.check(t, got)
		})
	}
}

// TestParse_EmailWithPlusAddressing verifies that plus-addressed emails
// (user+tag@domain.com) are treated as a single uninterrupted token.
func TestParse_EmailWithPlusAddressing(t *testing.T) {
	got := Parse("from:user+newsletters@example.com to:alice+work@company.org")
	if len(got.FromAddrs) != 1 || got.FromAddrs[0] != "user+newsletters@example.com" {
		t.Errorf("FromAddrs = %v, want [user+newsletters@example.com]", got.FromAddrs)
	}
	if len(got.ToAddrs) != 1 || got.ToAddrs[0] != "alice+work@company.org" {
		t.Errorf("ToAddrs = %v, want [alice+work@company.org]", got.ToAddrs)
	}
}

// TestParse_TabInQuery verifies that tab characters are NOT treated as
// word separators — only space is a delimiter in the tokenizer.
func TestParse_TabInQuery(t *testing.T) {
	// Tab between two words → one token containing the tab
	got := Parse("hello\tworld")
	if len(got.TextTerms) != 1 {
		t.Fatalf("TextTerms = %v, want 1 element (tab is not a delimiter)", got.TextTerms)
	}
	if got.TextTerms[0] != "hello\tworld" {
		t.Errorf("TextTerms[0] = %q, want %q", got.TextTerms[0], "hello\tworld")
	}
}

// BenchmarkTokenize_BareWords benchmarks the tokenizer with multiple bare words.
func BenchmarkTokenize_BareWords(b *testing.B) {
	for b.Loop() {
		tokenize("alpha beta gamma delta epsilon")
	}
}

// BenchmarkTokenize_MixedContent benchmarks the tokenizer with a mix of
// operators, quoted phrases, and bare words.
func BenchmarkTokenize_MixedContent(b *testing.B) {
	for b.Loop() {
		tokenize(`from:alice@example.com subject:"meeting notes" urgent important`)
	}
}
