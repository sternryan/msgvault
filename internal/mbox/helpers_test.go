package mbox

import (
	"os"
	"path/filepath"
	"testing"
)

const multipleMessagesMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <msg1@example.com>
From: sender@example.com
To: recipient@example.com
Subject: First message
Date: Mon, 01 Jan 2024 00:00:00 +0000

This is the body of message 1.

From sender@example.com Tue Jan  2 00:00:00 2024
Message-ID: <msg2@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Second message
Date: Tue, 02 Jan 2024 00:00:00 +0000

This is the body of message 2.

From sender@example.com Wed Jan  3 00:00:00 2024
Message-ID: <msg3@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Third message
Date: Wed, 03 Jan 2024 00:00:00 +0000

This is the body of message 3.
`

const singleMessageMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <msg1@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Single message
Date: Mon, 01 Jan 2024 00:00:00 +0000

This is the body of message 1.
`

// writeTempMbox writes mbox data to a temporary file and returns the file path.
func writeTempMbox(t *testing.T, data string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.mbox")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("writeTempMbox: %v", err)
	}
	return path
}
