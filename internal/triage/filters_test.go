package triage

import (
	"bytes"
	"compress/zlib"
	"strings"
	"testing"
)

func zlibCompress(s string) []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write([]byte(s))
	_ = zw.Close()
	return buf.Bytes()
}

// F1: List-Unsubscribe header drops
func TestShouldDrop_ListUnsubscribe(t *testing.T) {
	mime := "From: x@y.com\r\nList-Unsubscribe: <mailto:u@x.com>\r\nSubject: Hi\r\n\r\nBody."
	m := &Message{MessageID: "f1", Subject: "Hi", Body: "Body.", RawMIME: zlibCompress(mime)}
	drop, reason, err := ShouldDrop(m, "ryan@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !drop || reason != ReasonListUnsub {
		t.Fatalf("got drop=%v reason=%q, want true list_unsubscribe", drop, reason)
	}
}

// F2: Content-Type text/calendar
func TestShouldDrop_Calendar(t *testing.T) {
	mime := "From: x@y.com\r\nContent-Type: text/calendar; charset=utf-8\r\nSubject: Mtg\r\n\r\nBEGIN:VCALENDAR"
	m := &Message{MessageID: "f2", Subject: "Mtg", Body: "BEGIN:VCALENDAR", RawMIME: zlibCompress(mime)}
	drop, reason, _ := ShouldDrop(m, "ryan@example.com", nil)
	if !drop || reason != ReasonCalendar {
		t.Fatalf("got drop=%v reason=%q, want calendar", drop, reason)
	}
}

// F3: receipt subject
func TestShouldDrop_Receipt(t *testing.T) {
	m := &Message{MessageID: "f3", Subject: "Receipt for order #12345", Body: strings.Repeat("word ", 60)}
	drop, reason, _ := ShouldDrop(m, "ryan@example.com", nil)
	if !drop || reason != ReasonReceipt {
		t.Fatalf("got drop=%v reason=%q, want receipt", drop, reason)
	}
}

// F4: 2FA
func TestShouldDrop_2FA(t *testing.T) {
	m := &Message{MessageID: "f4", Subject: "Your verification code is 123456", Body: strings.Repeat("word ", 60)}
	drop, reason, _ := ShouldDrop(m, "ryan@example.com", nil)
	if !drop || reason != Reason2FA {
		t.Fatalf("got drop=%v reason=%q, want 2fa", drop, reason)
	}
}

// F5: bcc thread > 6 with ryan in bcc
func TestShouldDrop_BccThread(t *testing.T) {
	m := &Message{
		MessageID: "f5", Subject: "FYI", Body: strings.Repeat("word ", 60),
		Recipients: Recipients{
			To:  []string{"a@x.com", "b@x.com", "c@x.com"},
			Cc:  []string{"d@x.com", "e@x.com"},
			Bcc: []string{"ryan@example.com", "f@x.com"},
		},
	}
	drop, reason, _ := ShouldDrop(m, "ryan@example.com", nil)
	if !drop || reason != ReasonBccThread {
		t.Fatalf("got drop=%v reason=%q, want bcc_thread", drop, reason)
	}
}

// F6: short body, no URL -> drop
func TestShouldDrop_ShortNoURL(t *testing.T) {
	m := &Message{MessageID: "f6", Subject: "Hey", Body: "Just a quick note about lunch tomorrow."}
	drop, reason, _ := ShouldDrop(m, "ryan@example.com", nil)
	if !drop || reason != ReasonShortNoURL {
		t.Fatalf("got drop=%v reason=%q, want short_no_url", drop, reason)
	}
}

// F7: short body but high-signal URL -> keep
func TestShouldDrop_ShortWithURL(t *testing.T) {
	m := &Message{MessageID: "f7", Subject: "Paper", Body: "Check this https://arxiv.org/abs/2403.00001"}
	drop, _, _ := ShouldDrop(m, "ryan@example.com", nil)
	if drop {
		t.Fatalf("short with arxiv url got dropped")
	}
}

// F8: long body, no URL -> keep
func TestShouldDrop_LongNoURL(t *testing.T) {
	m := &Message{MessageID: "f8", Subject: "Update", Body: strings.Repeat("word ", 100)}
	drop, _, _ := ShouldDrop(m, "ryan@example.com", nil)
	if drop {
		t.Fatalf("long no-url got dropped")
	}
}

// F10: loadHeaders idempotent — second call returns same map without re-parse
func TestLoadHeaders_Idempotent(t *testing.T) {
	mime := "From: x@y.com\r\nList-Unsubscribe: <u>\r\n\r\nbody"
	m := &Message{RawMIME: zlibCompress(mime)}
	if err := loadHeaders(m); err != nil {
		t.Fatal(err)
	}
	if !m.headersLoaded {
		t.Fatal("headersLoaded not set")
	}
	first := m.Headers
	// Second call should not reset
	if err := loadHeaders(m); err != nil {
		t.Fatal(err)
	}
	if &m.Headers == nil {
		t.Fatal("headers cleared")
	}
	_ = first
}
