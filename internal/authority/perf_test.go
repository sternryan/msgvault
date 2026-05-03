package authority

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// TestRecompute_PerfRegression_100k guards against PERF-16-02 returning. Builds
// a synthetic 100k-message msgvault DB on tmpfs/disk and asserts that an
// incremental Recompute completes in under 5 minutes wall-clock on internal
// SSD.
//
// Laptop baseline (M1 / internal SSD, 2026-04-29, after the N+1 collapse fix
// in commit 695ed037): ~2.3 seconds for 100k messages / ~3,000 senders.
// 5-minute budget is ~130× headroom — a regression that meaningfully reverts
// the single-pass aggregation will trip this immediately.
//
// Skipped under -short or CI=1 to keep CI fast and avoid disk-bound flakes
// on shared runners. Run locally with:
//
//	go test ./internal/authority/ -run TestRecompute_PerfRegression_100k -v
func TestRecompute_PerfRegression_100k(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf regression test under -short")
	}
	if os.Getenv("CI") != "" {
		t.Skip("skipping perf regression test in CI (set CI= to force-run)")
	}

	const (
		nMessages   = 100_000
		nSenders    = 3_000  // ~33 messages per sender
		nReplies    = 5_000  // 5% of inbound are followed by a reply
		budget      = 5 * time.Minute
	)

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/perf.db"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open synthetic db: %v", err)
	}
	defer db.Close()

	// PRAGMAs that mirror production msgvault.db (WAL, default cache).
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatalf("%s: %v", pragma, err)
		}
	}

	loadFixture(t, db, "testdata/msgvault_fixture.sql")
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	t.Logf("seeding %d senders, %d messages, %d replies...", nSenders, nMessages, nReplies)
	seedStart := time.Now()
	if err := seedSyntheticCorpus(db, nSenders, nMessages, nReplies); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Logf("seed complete in %.1fs", time.Since(seedStart).Seconds())

	// Empty sources DB so BuildURLHashCache is a no-op, link_quality = 0.
	srcDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sources: %v", err)
	}
	defer srcDB.Close()
	loadFixture(t, srcDB, "testdata/sources_fixture.sql")

	t.Setenv("MSGVAULT_USER_EMAIL", "ryan@example.com")

	runStart := time.Now()
	res, err := Recompute(context.Background(), db, srcDB, "", "ryan@example.com")
	elapsed := time.Since(runStart)
	if err != nil {
		t.Fatalf("Recompute on 100k synthetic: %v", err)
	}

	t.Logf("Recompute done in %v: senders_updated=%d max_v=%d reply_mode=%s",
		elapsed, res.SendersUpdated, res.MaxVolume, res.ReplyMode)

	if elapsed > budget {
		t.Fatalf("PERF-16-02 REGRESSION: Recompute on 100k synthetic took %v (budget %v)",
			elapsed, budget)
	}

	// Sanity: at least one row written.
	var rowCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM authority_scores`).Scan(&rowCount); err != nil {
		t.Fatalf("count authority_scores: %v", err)
	}
	if rowCount < nSenders/2 {
		t.Fatalf("authority_scores rows = %d, want >= %d (synthetic senders not scored)",
			rowCount, nSenders/2)
	}
}

// seedSyntheticCorpus writes nSenders participants, nMessages inbound messages
// distributed round-robin across them, and nReplies "from-me" replies. IDs
// start at 1000 (above the fixture's 1-121 range) to avoid PK collisions.
func seedSyntheticCorpus(db *sql.DB, nSenders, nMessages, nReplies int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Bulk participants.
	pStmt, err := tx.Prepare(`INSERT INTO participants (id, email_address, display_name) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	for i := 0; i < nSenders; i++ {
		pid := 1000 + i
		email := fmt.Sprintf("sender%05d@perf.test", i)
		if _, err := pStmt.Exec(pid, email, ""); err != nil {
			pStmt.Close()
			return fmt.Errorf("insert participant %d: %w", i, err)
		}
	}
	pStmt.Close()

	// One conversation per sender (keeps schema simple, mirrors realistic
	// "one thread per correspondent" pattern).
	cStmt, err := tx.Prepare(`INSERT INTO conversations (id, source_conversation_id, title) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	for i := 0; i < nSenders; i++ {
		cid := 1000 + i
		if _, err := cStmt.Exec(cid, fmt.Sprintf("perf-thread-%d", i), ""); err != nil {
			cStmt.Close()
			return fmt.Errorf("insert conversation %d: %w", i, err)
		}
	}
	cStmt.Close()

	// Inbound messages, round-robin across senders. received_at = now - 3 days
	// so all fall inside the 7d response-rate window.
	mStmt, err := tx.Prepare(
		`INSERT INTO messages (id, conversation_id, sender_id, is_from_me, received_at, sent_at, subject)
		 VALUES (?, ?, ?, 0, datetime('now','-3 days'), datetime('now','-3 days'), ?)`,
	)
	if err != nil {
		return err
	}
	for i := 0; i < nMessages; i++ {
		mid := 1000 + i
		senderIdx := i % nSenders
		convID := 1000 + senderIdx
		senderID := 1000 + senderIdx
		if _, err := mStmt.Exec(mid, convID, senderID, fmt.Sprintf("msg %d", i)); err != nil {
			mStmt.Close()
			return fmt.Errorf("insert message %d: %w", i, err)
		}
	}
	mStmt.Close()

	// Reply messages from "me" (participant id=1, ryan@example.com from the
	// loaded fixture). Reply lands in the same conversation as the inbound
	// it follows, ID just above the inbound's. received_at = now - 2 days
	// so it sorts AFTER the inbound and falls inside the 7d window.
	rStmt, err := tx.Prepare(
		`INSERT INTO messages (id, conversation_id, sender_id, is_from_me, received_at, sent_at, subject)
		 VALUES (?, ?, 1, 1, datetime('now','-2 days'), datetime('now','-2 days'), ?)`,
	)
	if err != nil {
		return err
	}
	replyIDBase := 1000 + nMessages
	for i := 0; i < nReplies; i++ {
		// Distribute replies across conversations.
		convID := 1000 + (i % nSenders)
		rid := replyIDBase + i
		if _, err := rStmt.Exec(rid, convID, fmt.Sprintf("Re: %d", i)); err != nil {
			rStmt.Close()
			return fmt.Errorf("insert reply %d: %w", i, err)
		}
	}
	rStmt.Close()

	return tx.Commit()
}
