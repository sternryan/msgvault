package authority

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// setupTestDB returns (msgvaultDB, sourcesDB, manifestsDir) ready for a
// Recompute call. msgvault has InitSchema applied, msgvault_fixture.sql
// loaded, and received_at columns populated to "now - 3 days" so all
// inbound messages and replies fall inside the 7d window.
func setupTestDB(t *testing.T) (*sql.DB, *sql.DB, string) {
	t.Helper()
	mvDB := openMsgvaultFixture(t)
	srcDB := openSourcesFixture(t)
	// Per-message received_at so reply-within-7d windows are unambiguous.
	// Layout (all "now - N days"):
	//   alice msg 100 (inbound)  -3d
	//   alice msg 101 (reply)    -2d   → counts for 100 (within 7d)
	//   alice msg 102 (inbound)  -1d   → no reply after it → 1/2 = 0.5
	//   bob   msg 110 (inbound)  -3d
	//   bob   msg 111 (reply)    -3d   → counts for 110
	//   bob   msg 112 (inbound)  -2d
	//   bob   msg 113 (reply)    -1d   → counts for 112 → 2/2 = 1.0
	//   carol msg 120 (inbound)  -3d   no replies
	//   carol msg 121 (inbound)  -2d   → 0/2 = 0.0
	updates := []struct {
		id  int
		off string
	}{
		{100, "-3 days"}, {101, "-2 days"}, {102, "-1 days"},
		{110, "-3 days"}, {111, "-3 days"}, {112, "-2 days"}, {113, "-1 days"},
		{120, "-3 days"}, {121, "-2 days"},
	}
	for _, u := range updates {
		if _, err := mvDB.Exec(
			`UPDATE messages SET received_at = datetime('now', ?),
			                     sent_at     = datetime('now', ?) WHERE id = ?`,
			u.off, u.off, u.id,
		); err != nil {
			t.Fatalf("seed received_at id=%d: %v", u.id, err)
		}
	}
	return mvDB, srcDB, "testdata/manifests"
}

// scoreFor returns the (volume, response_rate_7d, link_quality, authority_score)
// row for sender_email. Fails t if no row exists.
func scoreFor(t *testing.T, db *sql.DB, email string) (int, float64, float64, float64) {
	t.Helper()
	var v int
	var r, l, s float64
	err := db.QueryRow(
		`SELECT volume, response_rate_7d, link_quality, authority_score
		   FROM authority_scores WHERE sender_email = ?`,
		email,
	).Scan(&v, &r, &l, &s)
	if err != nil {
		t.Fatalf("scoreFor(%s): %v", email, err)
	}
	return v, r, l, s
}

func TestRecompute_FreshFixture(t *testing.T) {
	mv, src, mfsDir := setupTestDB(t)
	t.Setenv("MSGVAULT_USER_EMAIL", "ryan@example.com")
	res, err := Recompute(context.Background(), mv, src, mfsDir, "ryan@example.com")
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if res.SendersUpdated != 3 {
		t.Fatalf("SendersUpdated = %d, want 3", res.SendersUpdated)
	}
	if res.NewWatermark <= 0 {
		t.Fatalf("NewWatermark = %d, want > 0", res.NewWatermark)
	}
	if res.ReplyMode != "is_from_me" {
		t.Fatalf("ReplyMode = %q, want is_from_me", res.ReplyMode)
	}
	if res.MaxVolume != 2 {
		t.Fatalf("MaxVolume = %d, want 2", res.MaxVolume)
	}

	// Hand-computed expected values:
	//   max_v = 2; VolumeNorm(2,2) = 1.0 for all three senders.
	//   alice: 2 inbound, 1 reply → reply_rate 0.5; URL example.com/post/alpha
	//          (compiled=1) → link_quality 1.0 → composite 0.2+0.2+0.4 = 0.8
	//   bob:   2 inbound, 2 replies → reply_rate 1.0; URL stanford (compiled=1)
	//          → link_quality 1.0 → composite 0.2+0.4+0.4 = 1.0
	//   carol: 2 inbound, 0 replies → reply_rate 0.0; URL newsletter (compiled=0)
	//          → link_quality 0.0 → composite 0.2+0+0 = 0.2
	cases := []struct {
		email    string
		v        int
		r, l, sc float64
	}{
		{"alice@example.com", 2, 0.5, 1.0, 0.8},
		{"bob@stanford.edu", 2, 1.0, 1.0, 1.0},
		{"carol@newsletter.com", 2, 0.0, 0.0, 0.2},
	}
	for _, tc := range cases {
		v, r, l, sc := scoreFor(t, mv, tc.email)
		if v != tc.v {
			t.Errorf("%s volume = %d, want %d", tc.email, v, tc.v)
		}
		if !floatEq(r, tc.r, 0.001) {
			t.Errorf("%s reply_rate = %v, want %v", tc.email, r, tc.r)
		}
		if !floatEq(l, tc.l, 0.001) {
			t.Errorf("%s link_quality = %v, want %v", tc.email, l, tc.l)
		}
		if !floatEq(sc, tc.sc, 0.001) {
			t.Errorf("%s authority_score = %v, want %v", tc.email, sc, tc.sc)
		}
	}

	// Watermark advanced to MAX(rowid).
	var wm int64
	mv.QueryRow(`SELECT last_msg_rowid FROM authority_state WHERE id = 1`).Scan(&wm)
	if wm != res.NewWatermark {
		t.Errorf("persisted watermark = %d, want %d", wm, res.NewWatermark)
	}
}

func TestRecompute_NoOp(t *testing.T) {
	mv, src, mfsDir := setupTestDB(t)
	ctx := context.Background()
	if _, err := Recompute(ctx, mv, src, mfsDir, "ryan@example.com"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Snapshot scores + state.
	type snap struct {
		score      float64
		updatedAt  string
	}
	before := map[string]snap{}
	rows, _ := mv.Query(`SELECT sender_email, authority_score, updated_at FROM authority_scores`)
	for rows.Next() {
		var e, u string
		var s float64
		_ = rows.Scan(&e, &s, &u)
		before[e] = snap{s, u}
	}
	rows.Close()
	var wmBefore int64
	mv.QueryRow(`SELECT last_msg_rowid FROM authority_state WHERE id = 1`).Scan(&wmBefore)

	res, err := Recompute(ctx, mv, src, mfsDir, "ryan@example.com")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.SendersUpdated != 0 {
		t.Errorf("second-run SendersUpdated = %d, want 0", res.SendersUpdated)
	}
	var wmAfter int64
	mv.QueryRow(`SELECT last_msg_rowid FROM authority_state WHERE id = 1`).Scan(&wmAfter)
	if wmAfter != wmBefore {
		t.Errorf("watermark changed: %d → %d (want unchanged)", wmBefore, wmAfter)
	}
	// Per-row updated_at MUST be unchanged (no UPSERT fired).
	rows, _ = mv.Query(`SELECT sender_email, authority_score, updated_at FROM authority_scores`)
	for rows.Next() {
		var e, u string
		var s float64
		_ = rows.Scan(&e, &s, &u)
		if before[e].updatedAt != u {
			t.Errorf("%s updated_at changed: %q → %q (want unchanged)", e, before[e].updatedAt, u)
		}
		if !floatEq(before[e].score, s, 1e-9) {
			t.Errorf("%s score changed: %v → %v", e, before[e].score, s)
		}
	}
	rows.Close()
}

func TestRecompute_FullParity(t *testing.T) {
	mv1, src1, mfs := setupTestDB(t)
	mv2, src2, _ := setupTestDB(t)
	ctx := context.Background()

	// mv1: incremental from scratch.
	if _, err := Recompute(ctx, mv1, src1, mfs, "ryan@example.com"); err != nil {
		t.Fatalf("incremental: %v", err)
	}
	// mv2: --full from scratch.
	if _, err := RecomputeFull(ctx, mv2, src2, mfs, "ryan@example.com"); err != nil {
		t.Fatalf("full: %v", err)
	}

	for _, email := range []string{"alice@example.com", "bob@stanford.edu", "carol@newsletter.com"} {
		_, _, _, s1 := scoreFor(t, mv1, email)
		_, _, _, s2 := scoreFor(t, mv2, email)
		if !floatEq(s1, s2, 0.001) {
			t.Errorf("%s: incremental=%v vs full=%v (drift > 0.001)", email, s1, s2)
		}
	}
}

func TestRecompute_TwoPassUnion(t *testing.T) {
	mv, src, mfs := setupTestDB(t)
	ctx := context.Background()

	// First run: scores baseline (alice, bob, carol).
	if _, err := Recompute(ctx, mv, src, mfs, "ryan@example.com"); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	var wm int64
	mv.QueryRow(`SELECT last_msg_rowid FROM authority_state WHERE id = 1`).Scan(&wm)

	// Touch-detection: mutate every stored authority_score to a sentinel.
	// If a sender is recomputed on the next run, their UPSERT overwrites
	// the sentinel with the real composite. If they are NOT recomputed,
	// the sentinel remains. (1-second updated_at granularity makes a
	// timestamp-based check unreliable in fast tests.)
	const sentinel = 0.987654
	if _, err := mv.Exec(`UPDATE authority_scores SET authority_score = ?`, sentinel); err != nil {
		t.Fatalf("inject sentinel: %v", err)
	}

	// Inject senderX (NEW, post-watermark) and arrange senderY's most-recent
	// inbound to land in the (now-8d, now-6d) drift window. We pick alice
	// as senderY: shift her inbound rowids' received_at to now-7 days. Add
	// senderX with a brand-new conversation, participant, message.
	if _, err := mv.Exec(
		`UPDATE messages SET received_at = datetime('now', '-7 days')
		   WHERE sender_id = (SELECT id FROM participants WHERE email_address = 'alice@example.com')`,
	); err != nil {
		t.Fatalf("shift alice: %v", err)
	}
	// Pin bob's most recent inbound OUTSIDE the drift window so he is NOT
	// touched (no new rowid for bob, no drift).
	if _, err := mv.Exec(
		`UPDATE messages SET received_at = datetime('now', '-3 days')
		   WHERE sender_id = (SELECT id FROM participants WHERE email_address = 'bob@stanford.edu')`,
	); err != nil {
		t.Fatalf("pin bob: %v", err)
	}
	// Pin carol's most recent inbound to now (well inside the window, NOT
	// drift edge), so she also is NOT touched (no new rowid).
	if _, err := mv.Exec(
		`UPDATE messages SET received_at = datetime('now', '-1 day')
		   WHERE sender_id = (SELECT id FROM participants WHERE email_address = 'carol@newsletter.com')`,
	); err != nil {
		t.Fatalf("pin carol: %v", err)
	}
	// Add senderX: new participant + conversation + 1 inbound message
	// post-watermark.
	mv.Exec(`INSERT INTO participants (id, email_address, display_name) VALUES (5, 'newsender@x.com', 'X')`)
	mv.Exec(`INSERT INTO conversations (id, source_conversation_id, title) VALUES (13, 'thread-x', 'X thread')`)
	if _, err := mv.Exec(
		`INSERT INTO messages (id, conversation_id, sender_id, is_from_me, received_at, subject)
		 VALUES (200, 13, 5, 0, datetime('now', '-1 day'), 'hi')`,
	); err != nil {
		t.Fatalf("add senderX msg: %v", err)
	}
	mv.Exec(`INSERT INTO message_bodies (message_id, body_text) VALUES (200, 'plain note no urls')`)

	res, err := Recompute(ctx, mv, src, mfs, "ryan@example.com")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	// senderX (new rowid) AND alice (drift) → 2 senders touched.
	// bob and carol → NOT touched.
	if res.SendersUpdated != 2 {
		t.Fatalf("SendersUpdated = %d, want 2 (senderX + alice drift)", res.SendersUpdated)
	}
	// alice's score should be overwritten (sentinel gone).
	var aliceScore float64
	mv.QueryRow(`SELECT authority_score FROM authority_scores WHERE sender_email = ?`, "alice@example.com").Scan(&aliceScore)
	if floatEq(aliceScore, sentinel, 1e-9) {
		t.Errorf("alice still at sentinel — drift sender NOT picked up by Pass 2")
	}
	// bob and carol should still hold the sentinel (NOT touched).
	for _, e := range []string{"bob@stanford.edu", "carol@newsletter.com"} {
		var s float64
		mv.QueryRow(`SELECT authority_score FROM authority_scores WHERE sender_email = ?`, e).Scan(&s)
		if !floatEq(s, sentinel, 1e-9) {
			t.Errorf("%s overwritten (now %v); should NOT be touched (no new rowid, no drift)", e, s)
		}
	}
	// senderX should now have a row.
	var n int
	mv.QueryRow(`SELECT COUNT(*) FROM authority_scores WHERE sender_email = ?`, "newsender@x.com").Scan(&n)
	if n != 1 {
		t.Errorf("senderX missing from authority_scores — Pass 1 (rowid > watermark) failed")
	}
}

func TestRecompute_LinkQuality_NonZero(t *testing.T) {
	mv, src, mfs := setupTestDB(t)
	if _, err := Recompute(context.Background(), mv, src, mfs, "ryan@example.com"); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	// alice + bob: URLs match compiled rows → link_quality > 0.
	for _, e := range []string{"alice@example.com", "bob@stanford.edu"} {
		_, _, l, _ := scoreFor(t, mv, e)
		if l <= 0 {
			t.Errorf("%s link_quality = %v, want > 0 (resolved §6 BLOCKER)", e, l)
		}
	}
	// carol: URL exists but compiled=0 → link_quality = 0.
	_, _, lc, _ := scoreFor(t, mv, "carol@newsletter.com")
	if lc != 0 {
		t.Errorf("carol link_quality = %v, want 0 (URL not compiled)", lc)
	}
}

func TestRecompute_ReplyDetectionFallback(t *testing.T) {
	mv, src, mfs := setupTestDB(t)
	// Force is_from_me to all-zero to trigger fallback.
	if _, err := mv.Exec(`UPDATE messages SET is_from_me = 0`); err != nil {
		t.Fatalf("zero is_from_me: %v", err)
	}
	res, err := Recompute(context.Background(), mv, src, mfs, "ryan@example.com")
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if res.ReplyMode != "sender_email" {
		t.Fatalf("ReplyMode = %q, want sender_email (A1 fallback)", res.ReplyMode)
	}
	var mode string
	mv.QueryRow(`SELECT reply_detection_mode FROM authority_state WHERE id = 1`).Scan(&mode)
	if mode != "sender_email" {
		t.Errorf("persisted reply_detection_mode = %q, want sender_email", mode)
	}
	// alice still has 1 reply from "ryan@example.com" (msg 101) → reply_rate
	// must be non-zero under the fallback path.
	_, r, _, _ := scoreFor(t, mv, "alice@example.com")
	if r <= 0 {
		t.Errorf("alice reply_rate = %v, want > 0 under sender_email fallback", r)
	}
}

func TestRecompute_MaxVQueriedFromFullCorpus(t *testing.T) {
	mv, src, mfs := setupTestDB(t)
	// Add a heavy-volume sender with 10 inbound messages — much higher than
	// any of the 3 baseline senders (each = 2). max_v MUST reflect this
	// regardless of which senders are in the touched union.
	mv.Exec(`INSERT INTO participants (id, email_address) VALUES (6, 'heavy@news.com')`)
	mv.Exec(`INSERT INTO conversations (id, source_conversation_id) VALUES (14, 'thread-heavy')`)
	for i := 0; i < 10; i++ {
		mv.Exec(`INSERT INTO messages (conversation_id, sender_id, is_from_me, received_at) VALUES (14, 6, 0, datetime('now', '-3 days'))`)
	}

	res, err := Recompute(context.Background(), mv, src, mfs, "ryan@example.com")
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if res.MaxVolume != 10 {
		t.Fatalf("MaxVolume = %d, want 10 (queried from FULL corpus, not subset)", res.MaxVolume)
	}
	// alice has volume=2, max_v=10 → VolumeNorm(2,10) = log10(3)/log10(11)
	// ≈ 0.4594. authority_score must reflect that (NOT 1.0 from a
	// touched-subset max_v=2 bug).
	v, _, _, _ := scoreFor(t, mv, "alice@example.com")
	if v != 2 {
		t.Fatalf("alice volume = %d, want 2", v)
	}
	// VolumeNorm(2,10) ≈ 0.4594; with reply_rate=0.5 and link_quality=1.0:
	//   composite = 0.2*0.4594 + 0.4*0.5 + 0.4*1.0 = 0.0919 + 0.2 + 0.4 = 0.6919
	_, _, _, sc := scoreFor(t, mv, "alice@example.com")
	want := 0.2*VolumeNorm(2, 10) + 0.4*0.5 + 0.4*1.0
	if !floatEq(sc, want, 0.001) {
		t.Errorf("alice composite = %v, want %v (max_v=10 from full corpus)", sc, want)
	}
}
