package search

import "testing"

// BenchmarkParse_Empty measures the baseline overhead of parsing an empty
// query — essentially just struct allocation + time.Now().
func BenchmarkParse_Empty(b *testing.B) {
	for b.Loop() {
		Parse("")
	}
}

// BenchmarkParse_SingleBareWord measures parsing a single bare word with
// no operators or quoted phrases.
func BenchmarkParse_SingleBareWord(b *testing.B) {
	for b.Loop() {
		Parse("urgent")
	}
}

// BenchmarkParse_SingleOperator measures parsing one simple operator:value pair.
func BenchmarkParse_SingleOperator(b *testing.B) {
	for b.Loop() {
		Parse("from:alice@example.com")
	}
}

// TestParse_EmptyStringIsEmpty verifies that parsing "" returns an empty query.
func TestParse_EmptyStringIsEmpty(t *testing.T) {
	q := Parse("")
	if !q.IsEmpty() {
		t.Errorf("Parse(\"\").IsEmpty() = false, want true")
	}
}

// TestParse_AllFieldsSetMakesQueryNonEmpty verifies that setting any single
// field makes the query non-empty (belt-and-suspenders for IsEmpty coverage).
func TestParse_AllFieldsSetMakesQueryNonEmpty(t *testing.T) {
	queries := []string{
		"from:x@example.com",
		"to:x@example.com",
		"cc:x@example.com",
		"bcc:x@example.com",
		"subject:test",
		"label:INBOX",
		"has:attachment",
		"before:2024-01-01",
		"after:2024-01-01",
		"larger:1M",
		"smaller:1M",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			got := Parse(q)
			if got.IsEmpty() {
				t.Errorf("Parse(%q).IsEmpty() = true, want false", q)
			}
		})
	}
}
