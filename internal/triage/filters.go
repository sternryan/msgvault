package triage

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

// DropReason is the categorical tag returned by ShouldDrop for the first
// matching hard filter. The empty string means "do not drop".
type DropReason string

const (
	ReasonListUnsub  DropReason = "list_unsubscribe"
	ReasonCalendar   DropReason = "calendar"
	ReasonReceipt    DropReason = "receipt"
	Reason2FA        DropReason = "2fa"
	ReasonBccThread  DropReason = "bcc_thread"
	ReasonShortNoURL DropReason = "short_no_url"
)

var (
	receiptRe   = regexp.MustCompile(`(?i)(receipt|invoice|order #|payment confirm)`)
	twoFactorRe = regexp.MustCompile(`(?i)(verification code|security code|one[- ]time)`)
	urlRe       = regexp.MustCompile(`https?://[^\s)>"']+`)
)

// loadHeaders decompresses m.RawMIME and parses its headers into m.Headers
// using a case-insensitive lower-cased key convention. Idempotent: a second
// call is a no-op (Pitfall 4: parse-on-read is the only path; results cached).
//
// If RawMIME is empty, m.Headers is initialized to an empty map and the
// function returns nil. If decompression fails, returns the zlib error.
func loadHeaders(m *Message) error {
	if m == nil {
		return fmt.Errorf("loadHeaders: nil message")
	}
	if m.headersLoaded {
		return nil
	}
	if len(m.RawMIME) == 0 {
		m.Headers = map[string]string{}
		m.headersLoaded = true
		return nil
	}
	// Try zlib decompress. RawMIME may be either still-compressed or
	// already-decompressed (test fixtures); detect by trying zlib first
	// and falling back to raw bytes.
	var decoded []byte
	if r, err := zlib.NewReader(bytes.NewReader(m.RawMIME)); err == nil {
		buf, rerr := io.ReadAll(r)
		_ = r.Close()
		if rerr == nil {
			decoded = buf
		}
	}
	if decoded == nil {
		decoded = m.RawMIME
	}
	headers := parseMIMEHeaders(decoded)
	m.Headers = headers
	if ct, ok := headers["content-type"]; ok && m.ContentType == "" {
		m.ContentType = ct
	}
	m.headersLoaded = true
	return nil
}

// parseMIMEHeaders extracts headers from the leading block of an RFC822 MIME
// message. Returns lowercased keys.
func parseMIMEHeaders(raw []byte) map[string]string {
	headers := make(map[string]string)
	// MIME headers end at first blank line.
	idx := bytes.Index(raw, []byte("\r\n\r\n"))
	if idx < 0 {
		idx = bytes.Index(raw, []byte("\n\n"))
	}
	headerBlock := raw
	if idx >= 0 {
		headerBlock = raw[:idx]
	}
	scanner := bufio.NewScanner(bytes.NewReader(headerBlock))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	var currentKey, currentValue string
	flush := func() {
		if currentKey != "" {
			headers[strings.ToLower(currentKey)] = strings.TrimSpace(currentValue)
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			// Header continuation (folded).
			currentValue += " " + strings.TrimSpace(line)
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		flush()
		currentKey = line[:colon]
		currentValue = strings.TrimSpace(line[colon+1:])
	}
	flush()
	return headers
}

// ShouldDrop applies the six hard filters (TRIAGE-03) to m. Returns the first
// matching reason. Headers are loaded lazily (parse-on-read).
//
// ryanEmail is the user's primary address (used by the bcc-thread filter).
// highSignalDomains is the URL-gold allowlist used by the short-no-URL filter.
func ShouldDrop(m *Message, ryanEmail string, highSignalDomains []string) (bool, DropReason, error) {
	if m == nil {
		return false, "", fmt.Errorf("ShouldDrop: nil message")
	}
	if err := loadHeaders(m); err != nil {
		return false, "", fmt.Errorf("load headers for %s: %w", m.MessageID, err)
	}

	// 1. List-Unsubscribe header present
	if _, ok := m.Headers["list-unsubscribe"]; ok {
		return true, ReasonListUnsub, nil
	}

	// 2. Content-Type: text/calendar
	ct := m.ContentType
	if ct == "" {
		ct = m.Headers["content-type"]
	}
	if ct != "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)), "text/calendar") {
		return true, ReasonCalendar, nil
	}

	// 3. Receipt subject regex
	if receiptRe.MatchString(m.Subject) {
		return true, ReasonReceipt, nil
	}

	// 4. 2FA subject regex
	if twoFactorRe.MatchString(m.Subject) {
		return true, Reason2FA, nil
	}

	// 5. Big bcc thread with Ryan in Bcc/Cc
	totalRecipients := len(m.Recipients.To) + len(m.Recipients.Cc) + len(m.Recipients.Bcc)
	if totalRecipients > 6 {
		ryanLower := strings.ToLower(strings.TrimSpace(ryanEmail))
		inBccCc := false
		if ryanLower != "" {
			for _, addr := range m.Recipients.Bcc {
				if strings.ToLower(strings.TrimSpace(addr)) == ryanLower {
					inBccCc = true
					break
				}
			}
			if !inBccCc {
				for _, addr := range m.Recipients.Cc {
					if strings.ToLower(strings.TrimSpace(addr)) == ryanLower {
						inBccCc = true
						break
					}
				}
			}
		}
		if inBccCc {
			return true, ReasonBccThread, nil
		}
	}

	// 6. Short body without high-signal URL
	wc := m.WordCount
	if wc == 0 {
		wc = len(strings.Fields(m.Body))
	}
	if wc < 40 {
		if !hasHighSignalURL(m.Body, highSignalDomains) {
			return true, ReasonShortNoURL, nil
		}
	}

	return false, "", nil
}

// hasHighSignalURL reports whether body contains a URL whose host is in the
// high-signal allowlist. Used by TRIAGE-03 short_no_url filter.
func hasHighSignalURL(body string, allowlist []string) bool {
	if allowlist == nil {
		allowlist = DefaultHighSignalDomains
	}
	hosts := make(map[string]bool, len(allowlist))
	for _, d := range allowlist {
		hosts[strings.ToLower(d)] = true
	}
	for _, raw := range urlRe.FindAllString(body, -1) {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
		if hosts[host] {
			return true
		}
	}
	return false
}

// ExtractURLs returns the deduplicated URLs found in body in source order.
func ExtractURLs(body string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, raw := range urlRe.FindAllString(body, -1) {
		clean := strings.TrimRight(raw, ".,;)")
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out
}
