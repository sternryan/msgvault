package search

import (
	"testing"
	"time"
)

// TestTokenize directly tests the tokenize function with specific inputs.
func TestTokenize(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTokens []string
	}{
		{
			name:       "empty string",
			input:      "",
			wantTokens: nil,
		},
		{
			name:       "single bare word",
			input:      "hello",
			wantTokens: []string{"hello"},
		},
		{
			name:       "two bare words",
			input:      "hello world",
			wantTokens: []string{"hello", "world"},
		},
		{
			name:       "operator:value",
			input:      "from:alice@example.com",
			wantTokens: []string{"from:alice@example.com"},
		},
		{
			name:       "operator:\"quoted value\"",
			input:      `subject:"meeting notes"`,
			wantTokens: []string{`subject:"meeting notes"`},
		},
		{
			name:       "standalone quoted phrase",
			input:      `"hello world"`,
			wantTokens: []string{`"hello world"`},
		},
		{
			name:       "multiple operators",
			input:      "from:alice@example.com to:bob@example.com",
			wantTokens: []string{"from:alice@example.com", "to:bob@example.com"},
		},
		{
			name:       "quoted phrase with colon inside",
			input:      `"time: 10:30"`,
			wantTokens: []string{`"time: 10:30"`},
		},
		{
			name:       "single-quoted phrase",
			input:      `'hello world'`,
			wantTokens: []string{`"hello world"`},
		},
		{
			name:       "mixed operator and quoted phrase",
			input:      `from:alice "project report"`,
			wantTokens: []string{"from:alice", `"project report"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.wantTokens) {
				t.Errorf("tokenize(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.wantTokens, len(tt.wantTokens))
				return
			}
			for i := range got {
				if got[i] != tt.wantTokens[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.wantTokens[i])
				}
			}
		})
	}
}

// TestIsQuotedPhrase directly tests the isQuotedPhrase helper boundary cases.
// Logic: len(token) > 2 && token[0] == '"' && token[len(token)-1] == '"'
func TestIsQuotedPhrase(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{`"hello world"`, true},
		{`"ab"`, true},           // len=4, 4>2=true
		{`"x"`, true},            // len=3, 3>2=true
		{`""`, false},            // len=2, 2>2=false
		{"hello", false},         // no quotes
		{`"hello`, false},        // only opening quote, last char != '"'
		{`hello"`, false},        // first char != '"'
		{`'hello world'`, false}, // single quotes — first char is not '"'
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			got := isQuotedPhrase(tt.token)
			if got != tt.want {
				t.Errorf("isQuotedPhrase(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

// TestUnquote directly tests the unquote helper.
func TestUnquote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"hello"`, "hello"},
		{`"hello world"`, "hello world"},
		{"hello", "hello"},               // no quotes — returned as-is
		{`""`, ""},                       // empty quoted string — returns inner ""[1:1] = ""
		{`"`, `"`},                       // single quote — len < 2, returned as-is
		{"", ""},                         // empty string
		{`"hello world`, `"hello world`}, // unclosed — last char != '"', returned as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := unquote(tt.input)
			if got != tt.want {
				t.Errorf("unquote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseSize_Decimal verifies that parseSize handles decimal values like 1.5M.
func TestParseSize_Decimal(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int64
	}{
		{
			name:  "1.5M",
			query: "larger:1.5M",
			want:  int64(1.5 * 1024 * 1024),
		},
		{
			name:  "2.5K",
			query: "larger:2.5K",
			want:  int64(2.5 * 1024),
		},
		{
			name:  "0.5G",
			query: "smaller:0.5G",
			want:  int64(0.5 * 1024 * 1024 * 1024),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.query)
			var actual *int64
			if got.LargerThan != nil {
				actual = got.LargerThan
			} else {
				actual = got.SmallerThan
			}
			if actual == nil {
				t.Fatalf("expected size to be set, got nil")
			}
			if *actual != tt.want {
				t.Errorf("size = %d, want %d", *actual, tt.want)
			}
		})
	}
}

// TestParseDate_Whitespace verifies that parseDate trims surrounding whitespace.
func TestParseDate_Whitespace(t *testing.T) {
	// parseDate calls strings.TrimSpace, so whitespace-padded values should parse.
	// However, the tokenizer splits on spaces, so in practice this would only matter
	// if called directly. We call parseDate indirectly through the parser with
	// an operator that has the value already extracted by the tokenizer.
	// We can verify the trim by constructing a date using the internal parseDate func.
	input := "  2024-06-15  "
	got := parseDate(input)
	if got == nil {
		t.Fatal("parseDate with padded whitespace should succeed, got nil")
	}
	if got.Year() != 2024 || got.Month() != time.June || got.Day() != 15 {
		t.Errorf("parseDate(%q) = %v, want 2024-06-15", input, got)
	}
}

// TestParseRelativeDate_Whitespace verifies parseRelativeDate trims surrounding whitespace.
func TestParseRelativeDate_Whitespace(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	got := parseRelativeDate("  7d  ", fixedNow)
	if got == nil {
		t.Fatal("parseRelativeDate with padded whitespace should succeed, got nil")
	}
	want := time.Date(2025, 6, 8, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseRelativeDate = %v, want %v", got, want)
	}
}

// TestParse_LabelShorthand verifies that both "label:" and "l:" add to Labels.
func TestParse_LabelShorthand(t *testing.T) {
	got := Parse("l:INBOX label:work")
	if len(got.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(got.Labels), got.Labels)
	}
	if got.Labels[0] != "INBOX" || got.Labels[1] != "work" {
		t.Errorf("Labels = %v, want [INBOX work]", got.Labels)
	}
}

// TestParse_SubjectPreservesCase verifies that subject values are NOT lowercased
// (unlike from/to/cc/bcc which are lowercased).
func TestParse_SubjectPreservesCase(t *testing.T) {
	got := Parse("subject:ImportantMeeting")
	if len(got.SubjectTerms) != 1 {
		t.Fatalf("expected 1 subject term, got %d", len(got.SubjectTerms))
	}
	if got.SubjectTerms[0] != "ImportantMeeting" {
		t.Errorf("SubjectTerms[0] = %q, want %q", got.SubjectTerms[0], "ImportantMeeting")
	}
}

// TestParse_FromLowercases verifies that from: values are lowercased.
func TestParse_FromLowercases(t *testing.T) {
	got := Parse("from:Alice@EXAMPLE.COM")
	if len(got.FromAddrs) != 1 {
		t.Fatalf("expected 1 from addr, got %d", len(got.FromAddrs))
	}
	if got.FromAddrs[0] != "alice@example.com" {
		t.Errorf("FromAddrs[0] = %q, want %q", got.FromAddrs[0], "alice@example.com")
	}
}
