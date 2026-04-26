// Package digest formats msgvault triage candidates into the Phase 14 D-05
// markdown digest body and assembles RFC822 envelope ready for Gmail send.
//
// D-06 LOCKED: plain markdown only — no HTML, no tables. Index N is the
// 1-indexed JSONL row index, mapped 1:1 to forge's `--select` flag.
package digest

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/wesm/msgvault/internal/triage"
)

// Format renders []triage.Candidate as the locked D-05 markdown body. The
// output is:
//
//	N. **[score]** Subject — sender · date
//	   snippet (first 2 plaintext lines)
//	   urls: domain1, domain2
//	   approve: N
//
// Index N is 1-indexed and matches the JSONL row index (Pitfall 7).
func Format(cands []triage.Candidate) string {
	var buf bytes.Buffer
	for i, c := range cands {
		idx := i + 1
		fmt.Fprintf(&buf, "%d. **[%.2f]** %s — %s · %s\n",
			idx,
			c.Score,
			sanitize(c.ThreadSubject),
			sanitize(c.Sender),
			sanitize(c.Date),
		)
		if snippet := firstNLines(c.Snippet, 2); snippet != "" {
			fmt.Fprintf(&buf, "   %s\n", sanitize(snippet))
		}
		if len(c.ExtractedURLs) > 0 {
			hosts := make([]string, 0, len(c.ExtractedURLs))
			for _, u := range c.ExtractedURLs {
				hosts = append(hosts, hostOnly(u))
			}
			fmt.Fprintf(&buf, "   urls: %s\n", strings.Join(hosts, ", "))
		}
		fmt.Fprintf(&buf, "   approve: %d\n\n", idx)
	}
	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// BuildRFC822 assembles a minimal RFC822 message ready for Gmail send.
// Per RESEARCH.md §Pattern 4: text/plain; charset=UTF-8; 7bit. Headers
// are sanitized (\r, \n stripped) to prevent injection.
func BuildRFC822(from, to, subject, markdownBody string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", sanitize(from))
	fmt.Fprintf(&buf, "To: %s\r\n", sanitize(to))
	fmt.Fprintf(&buf, "Subject: %s\r\n", sanitize(subject))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&buf, "Content-Transfer-Encoding: 7bit\r\n")
	fmt.Fprintf(&buf, "\r\n")
	buf.WriteString(markdownBody)
	return buf.Bytes()
}

// sanitize strips \r and \n from a string. Used on every header value
// (and snippet text) to defeat RFC822 header injection (T-14-10).
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// hostOnly returns the host portion of a URL (no scheme, no path) for
// the digest "urls:" line. Falls back to the raw string if parsing fails.
func hostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// firstNLines returns the first n non-empty lines of s, joined by spaces
// (so the digest body stays single-line per snippet).
func firstNLines(s string, n int) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, n)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= n {
			break
		}
	}
	return strings.Join(out, " ")
}
