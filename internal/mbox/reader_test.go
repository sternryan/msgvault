package mbox

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

const singleMessageMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <msg1@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Test message 1
Date: Mon, 01 Jan 2024 00:00:00 +0000

This is the body of message 1.
`

const multipleMessagesMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <msg1@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Test message 1
Date: Mon, 01 Jan 2024 00:00:00 +0000

This is the body of message 1.

From sender2@example.com Tue Jan  2 00:00:00 2024
Message-ID: <msg2@example.com>
From: sender2@example.com
To: recipient@example.com
Subject: Test message 2
Date: Tue, 02 Jan 2024 00:00:00 +0000

This is the body of message 2.

From sender3@example.com Wed Jan  3 00:00:00 2024
Message-ID: <msg3@example.com>
From: sender3@example.com
To: recipient@example.com
Subject: Test message 3
Date: Wed, 03 Jan 2024 00:00:00 +0000

This is the body of message 3.
`

const escapedFromMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <esc1@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Escaped from line test
Date: Mon, 01 Jan 2024 00:00:00 +0000

This message has escaped From lines:
>From the beginning of time.
>>From even more escaped.
Normal line here.
`

func writeTempMbox(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.mbox")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReaderSingleMessage(t *testing.T) {
	path := writeTempMbox(t, singleMessageMbox)
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	entry, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}

	if entry.Offset != 0 {
		t.Errorf("expected offset 0, got %d", entry.Offset)
	}

	body := string(entry.Raw)
	if !contains(body, "Message-ID: <msg1@example.com>") {
		t.Errorf("expected Message-ID header in raw, got:\n%s", body)
	}
	if !contains(body, "This is the body of message 1.") {
		t.Errorf("expected body in raw, got:\n%s", body)
	}

	// Should get EOF on next call.
	_, err = r.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReaderMultipleMessages(t *testing.T) {
	path := writeTempMbox(t, multipleMessagesMbox)
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var entries []*RawEntry
	for {
		entry, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(entries))
	}

	// Verify each message has its own Message-ID.
	for i, expected := range []string{"<msg1@example.com>", "<msg2@example.com>", "<msg3@example.com>"} {
		if !contains(string(entries[i].Raw), "Message-ID: "+expected) {
			t.Errorf("message %d: expected Message-ID %s", i, expected)
		}
	}

	// Verify offsets are increasing.
	for i := 1; i < len(entries); i++ {
		if entries[i].Offset <= entries[i-1].Offset {
			t.Errorf("offsets not increasing: [%d]=%d, [%d]=%d",
				i-1, entries[i-1].Offset, i, entries[i].Offset)
		}
	}
}

func TestReaderFromEscaping(t *testing.T) {
	path := writeTempMbox(t, escapedFromMbox)
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	entry, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}

	body := string(entry.Raw)
	// ">From " should be unescaped to "From ".
	if !contains(body, "From the beginning of time.") {
		t.Errorf("expected unescaped 'From ' line, got:\n%s", body)
	}
	// ">>From " should be unescaped to ">From ".
	if !contains(body, ">From even more escaped.") {
		t.Errorf("expected unescaped '>From ' line, got:\n%s", body)
	}
}

func TestReaderSeekResume(t *testing.T) {
	path := writeTempMbox(t, multipleMessagesMbox)

	// First pass: read all messages and record offsets.
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}

	var offsets []int64
	for {
		entry, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		offsets = append(offsets, entry.Offset)
	}
	r.Close()

	if len(offsets) != 3 {
		t.Fatalf("expected 3 offsets, got %d", len(offsets))
	}

	// Resume from the second message's offset.
	r2, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()

	if err := r2.SeekTo(offsets[1]); err != nil {
		t.Fatal(err)
	}

	entry, err := r2.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(entry.Raw), "Message-ID: <msg2@example.com>") {
		t.Error("expected to resume at message 2")
	}

	entry, err = r2.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(entry.Raw), "Message-ID: <msg3@example.com>") {
		t.Error("expected message 3 after resume")
	}

	_, err = r2.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF after last message, got %v", err)
	}
}

func TestReaderEmptyMbox(t *testing.T) {
	path := writeTempMbox(t, "")
	r, err := NewReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	_, err = r.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF for empty mbox, got %v", err)
	}
}

func TestCountMessages(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int64
	}{
		{"empty", "", 0},
		{"single", singleMessageMbox, 1},
		{"multiple", multipleMessagesMbox, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempMbox(t, tt.content)
			count, err := CountMessages(path)
			if err != nil {
				t.Fatal(err)
			}
			if count != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, count)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstr(s, substr)
}

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
