package search

import (
	"testing"
	"time"

	"github.com/wesm/msgvault/internal/testutil/ptr"
)

// TestParse_LastWriterWins_AfterDate verifies that when newer_than: and after:
// both set AfterDate, the last one in the query string wins (no merging).
func TestParse_LastWriterWins_AfterDate(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	// after:2024-01-01 appears last — it should overwrite newer_than:7d's value.
	q := p.Parse("newer_than:7d after:2024-01-01")
	want := ptr.Date(2024, 1, 1)
	if q.AfterDate == nil {
		t.Fatal("AfterDate should be set")
	}
	if q.AfterDate.Year() != want.Year() || q.AfterDate.Month() != want.Month() || q.AfterDate.Day() != want.Day() {
		t.Errorf("AfterDate = %v, want 2024-01-01 (after: overwrites newer_than:)", q.AfterDate)
	}
}

// TestParse_LastWriterWins_BeforeDate verifies that when older_than: and before:
// both set BeforeDate, the last one in the query string wins.
func TestParse_LastWriterWins_BeforeDate(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	// before:2024-01-01 appears last — it should overwrite older_than:7d's value.
	q := p.Parse("older_than:7d before:2024-01-01")
	want := ptr.Date(2024, 1, 1)
	if q.BeforeDate == nil {
		t.Fatal("BeforeDate should be set")
	}
	if q.BeforeDate.Year() != want.Year() || q.BeforeDate.Month() != want.Month() || q.BeforeDate.Day() != want.Day() {
		t.Errorf("BeforeDate = %v, want 2024-01-01 (before: overwrites older_than:)", q.BeforeDate)
	}
}

// TestParse_LastWriterWins_AfterFirst verifies the reverse ordering:
// when newer_than: appears AFTER after:, newer_than: wins.
func TestParse_LastWriterWins_AfterFirst(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	// newer_than:7d appears last — AfterDate should be 2025-06-08, not 2020-01-01.
	q := p.Parse("after:2020-01-01 newer_than:7d")
	expected := time.Date(2025, 6, 8, 0, 0, 0, 0, time.UTC)
	if q.AfterDate == nil {
		t.Fatal("AfterDate should be set")
	}
	if !q.AfterDate.Equal(expected) {
		t.Errorf("AfterDate = %v, want %v (newer_than: overwrites after: when last)", q.AfterDate, expected)
	}
}

// TestParse_AllOperatorsTogether verifies that a query using every supported operator
// produces a correctly populated Query with all fields set.
func TestParse_AllOperatorsTogether(t *testing.T) {
	p := &Parser{Now: func() time.Time { return time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC) }}

	q := p.Parse(`from:alice@example.com to:bob@example.com cc:carol@example.com bcc:dave@example.com ` +
		`subject:meeting label:INBOX l:work has:attachment ` +
		`before:2025-12-31 after:2025-01-01 larger:1M smaller:50M hello "world peace"`)

	checks := []struct {
		desc string
		ok   bool
	}{
		{"FromAddrs[0] = alice@example.com", len(q.FromAddrs) == 1 && q.FromAddrs[0] == "alice@example.com"},
		{"ToAddrs[0] = bob@example.com", len(q.ToAddrs) == 1 && q.ToAddrs[0] == "bob@example.com"},
		{"CcAddrs[0] = carol@example.com", len(q.CcAddrs) == 1 && q.CcAddrs[0] == "carol@example.com"},
		{"BccAddrs[0] = dave@example.com", len(q.BccAddrs) == 1 && q.BccAddrs[0] == "dave@example.com"},
		{"SubjectTerms[0] = meeting", len(q.SubjectTerms) == 1 && q.SubjectTerms[0] == "meeting"},
		{"Labels has 2 entries (INBOX, work)", len(q.Labels) == 2},
		{"HasAttachment = true", q.HasAttachment != nil && *q.HasAttachment},
		{"BeforeDate = 2025-12-31", q.BeforeDate != nil && q.BeforeDate.Year() == 2025 && q.BeforeDate.Month() == 12 && q.BeforeDate.Day() == 31},
		{"AfterDate = 2025-01-01", q.AfterDate != nil && q.AfterDate.Year() == 2025 && q.AfterDate.Month() == 1 && q.AfterDate.Day() == 1},
		{"LargerThan = 1M bytes", q.LargerThan != nil && *q.LargerThan == 1*1024*1024},
		{"SmallerThan = 50M bytes", q.SmallerThan != nil && *q.SmallerThan == 50*1024*1024},
		{"TextTerms = [hello, world peace]", len(q.TextTerms) == 2 && q.TextTerms[0] == "hello" && q.TextTerms[1] == "world peace"},
		{"IsEmpty = false", !q.IsEmpty()},
	}

	for _, c := range checks {
		if !c.ok {
			t.Errorf("FAIL: %s", c.desc)
		}
	}
}

// TestParse_NowFunctionCalledOnEveryParse verifies that the Parser.Now function
// is called on every Parse invocation, even for queries with no relative dates.
func TestParse_NowFunctionCalledOnEveryParse(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"relative date query", "newer_than:1d"},
		{"absolute date query", "after:2024-01-01"},
		{"no date query", "from:alice@example.com"},
		{"empty query", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			p := &Parser{Now: func() time.Time {
				callCount++
				return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			}}
			_ = p.Parse(tt.query)
			if callCount != 1 {
				t.Errorf("Parser.Now called %d times for %q, want 1", callCount, tt.query)
			}
		})
	}
}

// TestParse_HasAttachmentIdempotent verifies that repeating has:attachment
// does not clear or toggle the flag — it remains true.
func TestParse_HasAttachmentIdempotent(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"once", "has:attachment"},
		{"twice", "has:attachment has:attachment"},
		{"three times", "has:attachment has:attachment has:attachment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Parse(tt.query)
			if q.HasAttachment == nil || !*q.HasAttachment {
				t.Errorf("HasAttachment should be true for query %q", tt.query)
			}
		})
	}
}

// TestParse_LabelAndShorthandEquivalent verifies that label:X and l:X produce
// the same value in the Labels slice for various label values.
func TestParse_LabelAndShorthandEquivalent(t *testing.T) {
	labels := []string{"INBOX", "important", "sent", "starred", "My Label"}
	for _, label := range labels {
		t.Run(label, func(t *testing.T) {
			q1 := Parse("label:" + label)
			q2 := Parse("l:" + label)
			if len(q1.Labels) != 1 || len(q2.Labels) != 1 {
				t.Fatalf("expected 1 label each, got %v and %v", q1.Labels, q2.Labels)
			}
			if q1.Labels[0] != q2.Labels[0] {
				t.Errorf("label:%s produced %q, l:%s produced %q", label, q1.Labels[0], label, q2.Labels[0])
			}
		})
	}
}

// TestParse_TextTermsPreserveOrder verifies that TextTerms appear in the same
// order as the bare words and quoted phrases in the input query string.
func TestParse_TextTermsPreserveOrder(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{"alpha bravo charlie", []string{"alpha", "bravo", "charlie"}},
		{`"first" "second" "third"`, []string{"first", "second", "third"}},
		{`a "bc" d "ef"`, []string{"a", "bc", "d", "ef"}},
		{"z y x w", []string{"z", "y", "x", "w"}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q := Parse(tt.query)
			if len(q.TextTerms) != len(tt.want) {
				t.Fatalf("TextTerms = %v, want %v", q.TextTerms, tt.want)
			}
			for i, w := range tt.want {
				if q.TextTerms[i] != w {
					t.Errorf("TextTerms[%d] = %q, want %q", i, q.TextTerms[i], w)
				}
			}
		})
	}
}

// TestParse_AddressFieldsPreserveInputOrder verifies that all address fields
// preserve the order in which addresses appear in the query.
func TestParse_AddressFieldsPreserveInputOrder(t *testing.T) {
	q := Parse("from:z@example.com from:a@example.com to:y@example.com to:b@example.com cc:x@example.com bcc:w@example.com")

	checks := []struct {
		name  string
		got   []string
		want0 string
		want1 string
	}{
		{"from", q.FromAddrs, "z@example.com", "a@example.com"},
		{"to", q.ToAddrs, "y@example.com", "b@example.com"},
	}
	for _, c := range checks {
		if len(c.got) < 2 {
			t.Errorf("%s: got %v, want at least 2 entries", c.name, c.got)
			continue
		}
		if c.got[0] != c.want0 || c.got[1] != c.want1 {
			t.Errorf("%s: got [%q, %q], want [%q, %q]", c.name, c.got[0], c.got[1], c.want0, c.want1)
		}
	}
}

// TestParse_DateResultsAreUTC verifies that all parsed dates have the UTC location.
func TestParse_DateResultsAreUTC(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		getDate func(q *Query) *time.Time
	}{
		{"after date", "after:2024-06-15", func(q *Query) *time.Time { return q.AfterDate }},
		{"before date", "before:2024-12-31", func(q *Query) *time.Time { return q.BeforeDate }},
		{"after YYYY/MM/DD", "after:2024/06/15", func(q *Query) *time.Time { return q.AfterDate }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Parse(tt.query)
			date := tt.getDate(q)
			if date == nil {
				t.Fatal("expected date to be set, got nil")
			}
			if date.Location() != time.UTC {
				t.Errorf("date location = %v, want UTC", date.Location())
			}
		})
	}
}

// TestParse_InterleaveOperatorsAndText verifies that bare text interleaved between
// operators is correctly captured in TextTerms.
func TestParse_InterleaveOperatorsAndText(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantTerms []string
		wantFrom  int
	}{
		{
			"text before operator",
			"hello from:alice@example.com",
			[]string{"hello"},
			1,
		},
		{
			"text after operator",
			"from:alice@example.com world",
			[]string{"world"},
			1,
		},
		{
			"text between multiple operators",
			"a from:alice@example.com b from:bob@example.com c",
			[]string{"a", "b", "c"},
			2,
		},
		{
			"quoted phrases interleaved",
			`"first" from:alice@example.com "second"`,
			[]string{"first", "second"},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Parse(tt.query)
			if len(q.TextTerms) != len(tt.wantTerms) {
				t.Fatalf("TextTerms = %v, want %v", q.TextTerms, tt.wantTerms)
			}
			for i, term := range tt.wantTerms {
				if q.TextTerms[i] != term {
					t.Errorf("TextTerms[%d] = %q, want %q", i, q.TextTerms[i], term)
				}
			}
			if len(q.FromAddrs) != tt.wantFrom {
				t.Errorf("FromAddrs count = %d, want %d", len(q.FromAddrs), tt.wantFrom)
			}
		})
	}
}

// TestTokenize_SingleQuoteAfterColon verifies that a single-quoted value
// immediately after a colon is treated as an op-quoted token (same as double-quote).
func TestTokenize_SingleQuoteAfterColon(t *testing.T) {
	// When afterColon=true and we see a single quote, opQuoted is set, so
	// the token includes the quotes: from:'alice'
	tokens := tokenize("from:'alice'")
	if len(tokens) != 1 {
		t.Fatalf("tokenize(%q) = %v, want 1 token", "from:'alice'", tokens)
	}
	if tokens[0] != "from:'alice'" {
		t.Errorf("token = %q, want %q", tokens[0], "from:'alice'")
	}
}

// TestParse_RelativeDateBeforeAndAfterAbsolute verifies both orderings of
// relative vs absolute date operators for both AfterDate and BeforeDate.
func TestParse_RelativeDateBeforeAndAfterAbsolute(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	relDate := time.Date(2025, 6, 8, 0, 0, 0, 0, time.UTC)   // newer_than:7d
	absDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)   // after:2024-01-01
	relBefore := time.Date(2025, 6, 8, 0, 0, 0, 0, time.UTC) // older_than:7d
	absBefore := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC) // before:2023-01-01

	p := &Parser{Now: func() time.Time { return fixedNow }}

	tests := []struct {
		name     string
		query    string
		wantDate time.Time
		getDate  func(q *Query) *time.Time
	}{
		{"newer_than then after: abs wins", "newer_than:7d after:2024-01-01", absDate, func(q *Query) *time.Time { return q.AfterDate }},
		{"after: then newer_than: rel wins", "after:2024-01-01 newer_than:7d", relDate, func(q *Query) *time.Time { return q.AfterDate }},
		{"older_than then before: abs wins", "older_than:7d before:2023-01-01", absBefore, func(q *Query) *time.Time { return q.BeforeDate }},
		{"before: then older_than: rel wins", "before:2023-01-01 older_than:7d", relBefore, func(q *Query) *time.Time { return q.BeforeDate }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := p.Parse(tt.query)
			date := tt.getDate(q)
			if date == nil {
				t.Fatal("expected date to be set, got nil")
			}
			if !date.Equal(tt.wantDate) {
				t.Errorf("date = %v, want %v", date, tt.wantDate)
			}
		})
	}
}

// TestParse_SizeLastWriterWins verifies that repeated larger: or smaller:
// operators result in the last value winning.
func TestParse_SizeLastWriterWins(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantVal int64
		getSize func(q *Query) *int64
	}{
		{"larger: second wins", "larger:1M larger:5M", 5 * 1024 * 1024, func(q *Query) *int64 { return q.LargerThan }},
		{"smaller: second wins", "smaller:10M smaller:1M", 1 * 1024 * 1024, func(q *Query) *int64 { return q.SmallerThan }},
		{"larger: first wins if second invalid", "larger:5M larger:xyz", 5 * 1024 * 1024, func(q *Query) *int64 { return q.LargerThan }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Parse(tt.query)
			size := tt.getSize(q)
			if size == nil {
				t.Fatal("expected size to be set, got nil")
			}
			if *size != tt.wantVal {
				t.Errorf("size = %d, want %d", *size, tt.wantVal)
			}
		})
	}
}

// TestParse_FieldsAreIndependent verifies that each operator sets only its own
// field and leaves all other fields at zero values.
func TestParse_FieldsAreIndependent(t *testing.T) {
	tests := []struct {
		name  string
		query string
		check func(q *Query) bool
		desc  string
	}{
		{
			"from sets only FromAddrs",
			"from:a@example.com",
			func(q *Query) bool {
				return len(q.ToAddrs) == 0 && len(q.CcAddrs) == 0 && len(q.BccAddrs) == 0 &&
					len(q.TextTerms) == 0 && len(q.SubjectTerms) == 0 && len(q.Labels) == 0 &&
					q.HasAttachment == nil && q.BeforeDate == nil && q.AfterDate == nil &&
					q.LargerThan == nil && q.SmallerThan == nil
			},
			"from: should not set any other field",
		},
		{
			"to sets only ToAddrs",
			"to:a@example.com",
			func(q *Query) bool {
				return len(q.FromAddrs) == 0 && len(q.CcAddrs) == 0 && len(q.BccAddrs) == 0 &&
					len(q.TextTerms) == 0 && len(q.SubjectTerms) == 0
			},
			"to: should not set from/cc/bcc/text/subject",
		},
		{
			"has:attachment sets only HasAttachment",
			"has:attachment",
			func(q *Query) bool {
				return len(q.FromAddrs) == 0 && len(q.ToAddrs) == 0 &&
					len(q.TextTerms) == 0 && q.BeforeDate == nil && q.AfterDate == nil &&
					q.LargerThan == nil && q.SmallerThan == nil
			},
			"has:attachment should not set address/date/size fields",
		},
		{
			"larger: sets only LargerThan",
			"larger:5M",
			func(q *Query) bool {
				return len(q.FromAddrs) == 0 && q.SmallerThan == nil &&
					q.HasAttachment == nil && q.BeforeDate == nil
			},
			"larger: should not set other fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Parse(tt.query)
			if !tt.check(q) {
				t.Errorf("field isolation failed for %q: %s", tt.query, tt.desc)
			}
		})
	}
}
