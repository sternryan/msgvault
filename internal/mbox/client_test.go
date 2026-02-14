package mbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

const threadingMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <root@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Original thread
Date: Mon, 01 Jan 2024 00:00:00 +0000

Root message.

From sender2@example.com Tue Jan  2 00:00:00 2024
Message-ID: <reply1@example.com>
From: sender2@example.com
To: sender@example.com
Subject: Re: Original thread
Date: Tue, 02 Jan 2024 00:00:00 +0000
In-Reply-To: <root@example.com>

Reply to root.

From sender3@example.com Wed Jan  3 00:00:00 2024
Message-ID: <reply2@example.com>
From: sender3@example.com
To: sender@example.com
Subject: Re: Re: Original thread
Date: Wed, 03 Jan 2024 00:00:00 +0000
References: <root@example.com> <reply1@example.com>

Another reply.

From other@example.com Thu Jan  4 00:00:00 2024
Message-ID: <unrelated@example.com>
From: other@example.com
To: recipient@example.com
Subject: Different topic
Date: Thu, 04 Jan 2024 00:00:00 +0000

Unrelated message.
`

const gmThreadMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <gm1@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Gmail thread test
Date: Mon, 01 Jan 2024 00:00:00 +0000
X-GM-THRID: 1234567890

First in Gmail thread.

From sender2@example.com Tue Jan  2 00:00:00 2024
Message-ID: <gm2@example.com>
From: sender2@example.com
To: sender@example.com
Subject: Re: Gmail thread test
Date: Tue, 02 Jan 2024 00:00:00 +0000
X-GM-THRID: 1234567890
In-Reply-To: <gm1@example.com>

Second in Gmail thread.
`

const labelsMbox = `From sender@example.com Mon Jan  1 00:00:00 2024
Message-ID: <lbl1@example.com>
From: sender@example.com
To: recipient@example.com
Subject: Labels test
Date: Mon, 01 Jan 2024 00:00:00 +0000
X-Gmail-Labels: Inbox,Important

Body 1.

From sender@example.com Tue Jan  2 00:00:00 2024
Message-ID: <lbl2@example.com>
From: sender@example.com
To: recipient@example.com
Subject: More labels
Date: Tue, 02 Jan 2024 00:00:00 +0000
X-Gmail-Labels: Sent,"Label with, commas",Starred

Body 2.
`

func TestClientThreadGroupingInReplyTo(t *testing.T) {
	path := writeTempMbox(t, threadingMbox)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	resp, err := client.ListMessages(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(resp.Messages))
	}

	// Messages 0, 1, 2 should share the same thread (via In-Reply-To/References chain).
	thread0 := resp.Messages[0].ThreadID
	thread1 := resp.Messages[1].ThreadID
	thread2 := resp.Messages[2].ThreadID
	thread3 := resp.Messages[3].ThreadID

	if thread0 != thread1 {
		t.Errorf("messages 0 and 1 should be in same thread: %q vs %q", thread0, thread1)
	}
	if thread0 != thread2 {
		t.Errorf("messages 0 and 2 should be in same thread: %q vs %q", thread0, thread2)
	}
	if thread0 == thread3 {
		t.Errorf("message 3 should be in a different thread from 0")
	}
}

func TestClientThreadGroupingGMTHRID(t *testing.T) {
	path := writeTempMbox(t, gmThreadMbox)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	resp, err := client.ListMessages(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(resp.Messages))
	}

	// Both should share the same thread ID (X-GM-THRID).
	if resp.Messages[0].ThreadID != resp.Messages[1].ThreadID {
		t.Errorf("messages should share thread: %q vs %q",
			resp.Messages[0].ThreadID, resp.Messages[1].ThreadID)
	}
	if resp.Messages[0].ThreadID != "1234567890" {
		t.Errorf("expected thread ID 1234567890, got %q", resp.Messages[0].ThreadID)
	}
}

func TestClientThreadGroupingSubjectFallback(t *testing.T) {
	// Messages with the same subject (after normalization) but no In-Reply-To/References
	// should be grouped by subject hash.
	mboxData := `From a@example.com Mon Jan  1 00:00:00 2024
Message-ID: <subj1@example.com>
From: a@example.com
To: b@example.com
Subject: Weekly meeting
Date: Mon, 01 Jan 2024 00:00:00 +0000

First.

From b@example.com Tue Jan  2 00:00:00 2024
Message-ID: <subj2@example.com>
From: b@example.com
To: a@example.com
Subject: Re: Weekly meeting
Date: Tue, 02 Jan 2024 00:00:00 +0000

Reply without In-Reply-To.

From c@example.com Wed Jan  3 00:00:00 2024
Message-ID: <subj3@example.com>
From: c@example.com
To: d@example.com
Subject: Totally different
Date: Wed, 03 Jan 2024 00:00:00 +0000

Different subject.
`
	path := writeTempMbox(t, mboxData)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	resp, err := client.ListMessages(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(resp.Messages))
	}

	// Messages 0 and 1 should share a thread (same normalized subject).
	if resp.Messages[0].ThreadID != resp.Messages[1].ThreadID {
		t.Errorf("messages 0 and 1 should be in same thread: %q vs %q",
			resp.Messages[0].ThreadID, resp.Messages[1].ThreadID)
	}
	// Message 2 should be in a different thread.
	if resp.Messages[0].ThreadID == resp.Messages[2].ThreadID {
		t.Errorf("message 2 should be in a different thread")
	}
}

func TestClientLabelParsing(t *testing.T) {
	path := writeTempMbox(t, labelsMbox)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// Check labels discovered during indexing.
	labels, err := client.ListLabels(ctx)
	if err != nil {
		t.Fatal(err)
	}

	labelNames := make(map[string]bool)
	for _, l := range labels {
		labelNames[l.Name] = true
	}

	expected := []string{"Inbox", "Important", "Sent", "Label with, commas", "Starred"}
	for _, e := range expected {
		if !labelNames[e] {
			t.Errorf("expected label %q not found in %v", e, labelNames)
		}
	}

	// Check per-message labels.
	msg1, err := client.GetMessageRaw(ctx, "<lbl1@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	if len(msg1.LabelIDs) != 2 || msg1.LabelIDs[0] != "Inbox" || msg1.LabelIDs[1] != "Important" {
		t.Errorf("unexpected labels for msg1: %v", msg1.LabelIDs)
	}

	msg2, err := client.GetMessageRaw(ctx, "<lbl2@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	if len(msg2.LabelIDs) != 3 {
		t.Errorf("expected 3 labels for msg2, got %d: %v", len(msg2.LabelIDs), msg2.LabelIDs)
	}
}

func TestClientPagination(t *testing.T) {
	path := writeTempMbox(t, multipleMessagesMbox)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// First page: should get all 3 with no next page token (page size > count).
	resp, err := client.ListMessages(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(resp.Messages))
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected empty next page token, got %q", resp.NextPageToken)
	}
}

func TestClientPaginationWithLimit(t *testing.T) {
	path := writeTempMbox(t, multipleMessagesMbox)
	client, err := NewClient([]string{path}, "test", WithMboxLimit(2))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	resp, err := client.ListMessages(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages (limit), got %d", len(resp.Messages))
	}

	// Next call should return empty (limit reached).
	resp2, err := client.ListMessages(ctx, "", resp.NextPageToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp2.Messages) != 0 {
		t.Errorf("expected 0 messages after limit, got %d", len(resp2.Messages))
	}
}

func TestClientDateFiltering(t *testing.T) {
	path := writeTempMbox(t, multipleMessagesMbox)
	after := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	client, err := NewClient([]string{path}, "test", WithMboxAfterDate(after))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	resp, err := client.ListMessages(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Only messages on or after Jan 2 should be included (msg2, msg3).
	if len(resp.Messages) != 2 {
		t.Errorf("expected 2 messages after filtering, got %d", len(resp.Messages))
	}
}

func TestClientDateFilteringBefore(t *testing.T) {
	path := writeTempMbox(t, multipleMessagesMbox)
	before := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	client, err := NewClient([]string{path}, "test", WithMboxBeforeDate(before))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	resp, err := client.ListMessages(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Only messages before Jan 2 should be included (msg1).
	if len(resp.Messages) != 1 {
		t.Errorf("expected 1 message before Jan 2, got %d", len(resp.Messages))
	}
}

func TestClientGetMessageRaw(t *testing.T) {
	path := writeTempMbox(t, singleMessageMbox)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	msg, err := client.GetMessageRaw(ctx, "<msg1@example.com>")
	if err != nil {
		t.Fatal(err)
	}

	if msg.ID != "<msg1@example.com>" {
		t.Errorf("unexpected message ID: %q", msg.ID)
	}
	if !strings.Contains(string(msg.Raw), "This is the body of message 1.") {
		t.Error("expected body in raw message")
	}
	if msg.InternalDate == 0 {
		t.Error("expected non-zero InternalDate")
	}
}

func TestClientGetMessageRawNotFound(t *testing.T) {
	path := writeTempMbox(t, singleMessageMbox)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	_, err = client.GetMessageRaw(ctx, "<nonexistent@example.com>")
	if err == nil {
		t.Error("expected error for nonexistent message")
	}
}

func TestClientDeletionNotSupported(t *testing.T) {
	path := writeTempMbox(t, singleMessageMbox)
	client, err := NewClient([]string{path}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.TrashMessage(ctx, "x"); err == nil {
		t.Error("expected error for TrashMessage")
	}
	if err := client.DeleteMessage(ctx, "x"); err == nil {
		t.Error("expected error for DeleteMessage")
	}
	if err := client.BatchDeleteMessages(ctx, []string{"x"}); err == nil {
		t.Error("expected error for BatchDeleteMessages")
	}
}

func TestParseGmailLabels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"simple", "Inbox,Sent", []string{"Inbox", "Sent"}},
		{"with spaces", "Inbox, Important, Starred", []string{"Inbox", "Important", "Starred"}},
		{"quoted commas", `Sent,"Label with, commas",Inbox`, []string{"Sent", "Label with, commas", "Inbox"}},
		{"empty", "", nil},
		{"single", "Inbox", []string{"Inbox"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseGmailLabels(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d labels, got %d: %v", len(tt.expected), len(result), result)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("label %d: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestNormalizeSubject(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello world"},
		{"Re: Hello World", "hello world"},
		{"Fwd: Hello World", "hello world"},
		{"Re: Re: Re: Hello World", "hello world"},
		{"Re: Fwd: Hello World", "hello world"},
		{"FW: Hello World", "hello world"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSubject(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestClientGetProfile(t *testing.T) {
	path := writeTempMbox(t, multipleMessagesMbox)
	client, err := NewClient([]string{path}, "test-source")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	profile, err := client.GetProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if profile.EmailAddress != "test-source" {
		t.Errorf("expected email 'test-source', got %q", profile.EmailAddress)
	}
	if profile.MessagesTotal != 3 {
		t.Errorf("expected 3 messages, got %d", profile.MessagesTotal)
	}
}
