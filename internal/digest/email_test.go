package digest

import (
	"strings"
	"testing"

	"github.com/wesm/msgvault/internal/triage"
)

func sample() []triage.Candidate {
	return []triage.Candidate{
		{
			MessageID: "m1", ThreadID: "t1", ThreadSubject: "Re: §351 holdings",
			Sender: "alex@example.com", Date: "2026-04-22",
			Score:         0.83,
			Snippet:       "Quick thought on §351 holding period\nthe 7yr rule we discussed isn't actually...",
			ExtractedURLs: []string{"https://vanguard.com/exchange-fund-faq", "https://kitces.com/blog"},
		},
		{
			MessageID: "m2", ThreadID: "t2", ThreadSubject: "Q4 plan",
			Sender: "ryan@example.com", Date: "2026-04-21",
			Score: 0.55,
		},
	}
}

// E1: Format produces D-05 shape
func TestFormat_Shape(t *testing.T) {
	got := Format(sample())
	wants := []string{
		"1. **[0.83]** Re: §351 holdings — alex@example.com · 2026-04-22",
		"   urls: vanguard.com, kitces.com",
		"   approve: 1",
		"2. **[0.55]** Q4 plan — ryan@example.com · 2026-04-21",
		"   approve: 2",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

// E2: score formatted with 2 decimals
func TestFormat_TwoDecimals(t *testing.T) {
	got := Format([]triage.Candidate{{Score: 0.5, ThreadSubject: "x", Sender: "y@z", Date: "2026-04-25"}})
	if !strings.Contains(got, "[0.50]") {
		t.Fatalf("missing [0.50] in %q", got)
	}
}

// E5: index = JSONL row index (1-indexed)
func TestFormat_IndexMatchesRow(t *testing.T) {
	got := Format(sample())
	if !strings.Contains(got, "approve: 1") || !strings.Contains(got, "approve: 2") {
		t.Fatalf("approve indices wrong:\n%s", got)
	}
}

// E6: no HTML
func TestFormat_NoHTML(t *testing.T) {
	got := Format(sample())
	for _, h := range []string{"<table", "<tr", "<td", "<html", "<body"} {
		if strings.Contains(got, h) {
			t.Errorf("found HTML tag %q", h)
		}
	}
}

// E7: no tables (markdown pipe form)
func TestFormat_NoMarkdownTables(t *testing.T) {
	got := Format(sample())
	if strings.Contains(got, "|---") {
		t.Fatalf("got markdown table separator")
	}
}

// E8: header injection sanitization on subject + sender
func TestFormat_Sanitize(t *testing.T) {
	c := []triage.Candidate{{
		ThreadSubject: "evil\r\nBcc: attacker@x.com",
		Sender:        "x@y.com\r\nX-Forge: 1",
		Date:          "2026-04-25",
		Score:         0.6,
	}}
	got := Format(c)
	if strings.Contains(got, "\r") {
		t.Fatalf("CR in output")
	}
	// embedded LF should not produce extra header-style lines
	if strings.Contains(got, "Bcc: attacker") {
		// allowed in body, but must be on the subject line, not on its own line
		if strings.Contains(got, "\nBcc: attacker") {
			t.Fatalf("LF survived sanitize: %q", got)
		}
	}
}

// E9: BuildRFC822 contains required headers
func TestBuildRFC822_Headers(t *testing.T) {
	out := string(BuildRFC822("ryan@example.com", "ryan@example.com", "Weekly digest", "body"))
	for _, h := range []string{
		"From: ryan@example.com",
		"To: ryan@example.com",
		"Subject: Weekly digest",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 7bit",
	} {
		if !strings.Contains(out, h) {
			t.Errorf("missing header %q in:\n%s", h, out)
		}
	}
}

// BuildRFC822 sanitizes \r\n in headers — no NEW header line should appear
// (i.e., no "\nBcc:" or "\nX-Evil:" sequence at the start of a line).
func TestBuildRFC822_NoInjection(t *testing.T) {
	out := string(BuildRFC822(
		"x\r\nBcc: attacker@y.com",
		"ryan@example.com",
		"hello\r\nX-Evil: 1",
		"body",
	))
	// CR must not survive in the header block (only the explicit \r\n line endings).
	headerBlock := out[:strings.Index(out, "\r\n\r\n")]
	if strings.Contains(headerBlock, "\nBcc:") || strings.Contains(headerBlock, "\nX-Evil:") {
		t.Fatalf("header injection produced new header line:\n%s", headerBlock)
	}
}
