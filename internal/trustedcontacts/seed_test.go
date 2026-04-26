package trustedcontacts

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	_ "github.com/mattn/go-sqlite3"
)

// Schema matching internal/store/schema.sql for the columns we touch.
const seedSchema = `
CREATE TABLE participants (
	id INTEGER PRIMARY KEY,
	email_address TEXT,
	display_name TEXT
);
CREATE TABLE sources (id INTEGER PRIMARY KEY);
CREATE TABLE conversations (id INTEGER PRIMARY KEY);
CREATE TABLE messages (
	id INTEGER PRIMARY KEY,
	conversation_id INTEGER,
	source_id INTEGER,
	source_message_id TEXT,
	message_type TEXT,
	sent_at DATETIME,
	received_at DATETIME,
	archived_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	sender_id INTEGER,
	subject TEXT,
	is_from_me BOOLEAN
);
CREATE TABLE message_recipients (
	id INTEGER PRIMARY KEY,
	message_id INTEGER,
	participant_id INTEGER,
	recipient_type TEXT
);
`

func openSeedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(seedSchema); err != nil {
		t.Fatal(err)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec: %v\n%s", err, q)
	}
}

// B1+B2+B3+B4: top-10, score desc, noise excluded, bidirectional weighting.
func TestBootstrap_Top10AndOrdering(t *testing.T) {
	db := openSeedDB(t)
	defer db.Close()
	now := time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)

	// 12 participants: ryan + 11 others (one noise).
	mustExec(t, db, `INSERT INTO participants(id, email_address, display_name) VALUES (?, ?, ?)`, 1, "ryan@example.com", "Ryan")
	for i := 2; i <= 12; i++ {
		email := ""
		if i == 12 {
			email = "noreply@notifications.com"
		} else {
			email = "user" + string(rune('a'+i)) + "@example.com"
		}
		mustExec(t, db, `INSERT INTO participants(id, email_address, display_name) VALUES (?, ?, ?)`, i, email, "")
	}
	// 30 messages: each non-ryan sends N messages to ryan (varying counts).
	mid := 1
	for i := 2; i <= 12; i++ {
		count := 14 - i // user 'b' sends 12, then 11, ..., noise sends 2
		for j := 0; j < count; j++ {
			mustExec(t, db, `INSERT INTO messages(id, conversation_id, source_id, message_type, sent_at, sender_id) VALUES (?, 1, 1, 'email', ?, ?)`, mid, now, i)
			mid++
		}
	}

	var buf bytes.Buffer
	err := Bootstrap(context.Background(), db, "ryan@example.com", 10, 365*24*time.Hour, nil, &buf)
	if err != nil {
		t.Fatal(err)
	}
	var seed SeedFile
	if _, err := toml.Decode(buf.String(), &seed); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if len(seed.Contacts) != 10 {
		t.Fatalf("got %d contacts, want 10\n%s", len(seed.Contacts), buf.String())
	}
	// Volumes must be sorted desc.
	for i := 1; i < len(seed.Contacts); i++ {
		if seed.Contacts[i].Volume > seed.Contacts[i-1].Volume {
			t.Fatalf("not sorted desc at %d: %v", i, seed.Contacts)
		}
	}
	// Noise excluded.
	for _, c := range seed.Contacts {
		if strings.Contains(c.Email, "noreply") || strings.Contains(c.Email, "notifications") {
			t.Fatalf("noise contact slipped through: %s", c.Email)
		}
	}
	// Self excluded.
	for _, c := range seed.Contacts {
		if c.Email == "ryan@example.com" {
			t.Fatalf("self contact in seed")
		}
	}
}

// B5: default lookback respected (older messages excluded)
func TestBootstrap_LookbackFiltersOld(t *testing.T) {
	db := openSeedDB(t)
	defer db.Close()
	mustExec(t, db, `INSERT INTO participants(id, email_address) VALUES (1, 'ryan@example.com')`)
	mustExec(t, db, `INSERT INTO participants(id, email_address) VALUES (2, 'old@example.com')`)
	mustExec(t, db, `INSERT INTO participants(id, email_address) VALUES (3, 'recent@example.com')`)
	old := time.Now().AddDate(-2, 0, 0).Format(time.RFC3339)
	recent := time.Now().AddDate(0, 0, -10).Format(time.RFC3339)
	for i := 0; i < 5; i++ {
		mustExec(t, db, `INSERT INTO messages(id, conversation_id, source_id, message_type, sent_at, sender_id) VALUES (?, 1, 1, 'email', ?, 2)`, i+1, old)
		mustExec(t, db, `INSERT INTO messages(id, conversation_id, source_id, message_type, sent_at, sender_id) VALUES (?, 1, 1, 'email', ?, 3)`, 10+i, recent)
	}
	var buf bytes.Buffer
	if err := Bootstrap(context.Background(), db, "ryan@example.com", 10, 30*24*time.Hour, nil, &buf); err != nil {
		t.Fatal(err)
	}
	var seed SeedFile
	if _, err := toml.Decode(buf.String(), &seed); err != nil {
		t.Fatal(err)
	}
	for _, c := range seed.Contacts {
		if c.Email == "old@example.com" {
			t.Fatalf("old contact leaked through 30d window")
		}
	}
}

// B6: parses as TOML.
func TestBootstrap_TOMLParseable(t *testing.T) {
	db := openSeedDB(t)
	defer db.Close()
	mustExec(t, db, `INSERT INTO participants(id, email_address) VALUES (1, 'a@example.com')`)
	mustExec(t, db, `INSERT INTO messages(id, conversation_id, source_id, message_type, sent_at, sender_id) VALUES (1, 1, 1, 'email', '2026-04-01T00:00:00Z', 1)`)
	var buf bytes.Buffer
	if err := Bootstrap(context.Background(), db, "ryan@example.com", 10, 365*24*time.Hour, nil, &buf); err != nil {
		t.Fatal(err)
	}
	var seed SeedFile
	if _, err := toml.Decode(buf.String(), &seed); err != nil {
		t.Fatalf("not parseable: %v\n%s", err, buf.String())
	}
}
