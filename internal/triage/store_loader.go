package triage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	mvmime "github.com/wesm/msgvault/internal/mime"
)

// LoadMessages queries the msgvault SQLite store for messages whose
// sent_at is within `since` of now and returns []*Message ready for
// scoring. Body text is taken from message_bodies.body_text; raw MIME
// from message_raw.raw_data (zlib-compressed); recipients from
// message_recipients joined to participants.
//
// This loader is read-only against msgvault.db. It does NOT touch forge.
func LoadMessages(ctx context.Context, db *sql.DB, since time.Duration, limit int) ([]*Message, error) {
	if db == nil {
		return nil, fmt.Errorf("LoadMessages: nil db")
	}
	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)
	if limit <= 0 {
		limit = 5000
	}
	q := `
		SELECT m.id,
		       COALESCE(m.source_message_id, ''),
		       COALESCE(m.subject, ''),
		       COALESCE(m.sent_at, m.received_at, m.archived_at) AS dt,
		       COALESCE(sp.email_address, ''),
		       COALESCE(c.source_conversation_id, ''),
		       COALESCE(c.title, m.subject),
		       COALESCE(b.body_text, ''),
		       COALESCE(r.raw_data, X''),
		       COALESCE(r.compression, 'zlib')
		FROM messages m
		LEFT JOIN participants sp ON sp.id = m.sender_id
		LEFT JOIN conversations c ON c.id = m.conversation_id
		LEFT JOIN message_bodies b ON b.message_id = m.id
		LEFT JOIN message_raw r ON r.message_id = m.id
		WHERE COALESCE(m.sent_at, m.received_at, m.archived_at) >= ?
		ORDER BY dt DESC
		LIMIT ?
	`
	rows, err := db.QueryContext(ctx, q, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var out []*Message
	for rows.Next() {
		var (
			id             int64
			srcMsgID       string
			subject        string
			dtStr          string
			senderEmail    string
			threadID       string
			threadSubject  string
			bodyText       string
			rawData        []byte
			rawCompression string
		)
		if err := rows.Scan(&id, &srcMsgID, &subject, &dtStr, &senderEmail, &threadID, &threadSubject, &bodyText, &rawData, &rawCompression); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		dt := parseStoredTime(dtStr)
		messageID := srcMsgID
		if messageID == "" {
			messageID = fmt.Sprintf("%d", id)
		}
		m := &Message{
			MessageID:     messageID,
			ThreadID:      threadID,
			ThreadSubject: threadSubject,
			Subject:       subject,
			Sender:        strings.ToLower(strings.TrimSpace(senderEmail)),
			Date:          dt,
			Body:          bodyText,
			RawMIME:       rawData,
			ExtractedURLs: ExtractURLs(bodyText),
			WordCount:     len(strings.Fields(bodyText)),
		}
		// Load recipients per-message (small N).
		if err := loadRecipients(ctx, db, id, &m.Recipients); err != nil {
			return nil, fmt.Errorf("recipients %d: %w", id, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadRecipients(ctx context.Context, db *sql.DB, messageID int64, r *Recipients) error {
	rows, err := db.QueryContext(ctx, `
		SELECT mr.recipient_type, COALESCE(p.email_address, '')
		FROM message_recipients mr
		JOIN participants p ON p.id = mr.participant_id
		WHERE mr.message_id = ?
	`, messageID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rtype, email string
		if err := rows.Scan(&rtype, &email); err != nil {
			return err
		}
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		switch strings.ToLower(rtype) {
		case "to":
			r.To = append(r.To, email)
		case "cc":
			r.Cc = append(r.Cc, email)
		case "bcc":
			r.Bcc = append(r.Bcc, email)
		}
	}
	return rows.Err()
}

func parseStoredTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ParseHeadersIfNeeded ensures that any subsequent ShouldDrop call has
// access to headers parsed from m.RawMIME using the msgvault internal/mime
// package — used as the canonical decoder for production runs (tests use
// the lightweight parser baked into filters.go).
//
// This is a no-op if RawMIME is empty or already-parsed headers exist.
func ParseHeadersIfNeeded(m *Message) error {
	if m == nil || m.headersLoaded {
		return nil
	}
	if len(m.RawMIME) == 0 {
		return loadHeaders(m)
	}
	parsed, err := mvmime.Parse(m.RawMIME)
	if err != nil {
		// Fall back to filters.loadHeaders' best-effort parser.
		return loadHeaders(m)
	}
	if m.Headers == nil {
		m.Headers = map[string]string{}
	}
	// Copy subject if missing and a few core headers we care about.
	if m.Subject == "" && parsed.Subject != "" {
		m.Subject = parsed.Subject
	}
	m.headersLoaded = true
	return nil
}
