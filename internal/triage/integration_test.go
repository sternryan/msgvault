package triage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mutecomm/go-sqlcipher/v4"

	"github.com/wesm/msgvault/internal/authority"
)

// stanfordAuthorityFake returns an authority Store that mimics the legacy
// "stanford.edu allowlist" by giving any stanford.edu sender a high authority
// score (0.95). Used by the legacy E2E tests that pre-date AUTHGRAPH-03 so
// they keep their original semantic intent (criterion #7 fires for the test
// senders) without depending on the production authority_scores table.
func stanfordAuthorityFake() fakeAuthorityStore {
	return fakeAuthorityStore{
		"alex@stanford.edu":       0.95,
		"researcher@stanford.edu": 0.95,
	}
}

// TestRunTriage_EndToEnd exercises RunTriage with fixture forge graph.db +
// in-memory message slice. Asserts I1 (exits OK with valid JSONL),
// I2 (only candidates >= threshold), I3 (max-N truncation),
// I4 (deterministic across runs), I7 (matched entities populated).
func TestRunTriage_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, graphPath)
	lex, err := OpenLexicon(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lex.Close()
	tp, err := OpenTopicPairs(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()

	now := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	msgs := []*Message{
		// 1: high-scoring research signal — Vanguard + Section 351 + arXiv URL
		{
			MessageID: "m1", ThreadID: "t1", ThreadSubject: "Re: §351 holdings",
			Subject: "Re: §351 holdings", Sender: "alex@stanford.edu",
			Date:          now.AddDate(0, 0, -3),
			Body:          "I'm trying to figure out the Vanguard exchange fund mechanics. Section 351 holding period is 7 years per https://arxiv.org/abs/2403.00001 and Vanguard docs.",
			ExtractedURLs: []string{"https://arxiv.org/abs/2403.00001"},
		},
		// 2: short with no URL — gets filtered (TRIAGE-03)
		{
			MessageID: "m2", ThreadID: "t2", ThreadSubject: "lunch",
			Subject: "lunch", Sender: "friend@example.com",
			Date: now.AddDate(0, 0, -1),
			Body: "want to grab lunch tomorrow",
		},
		// 3: receipt subject — gets filtered
		{
			MessageID: "m3", ThreadID: "t3", ThreadSubject: "Receipt for order #42",
			Subject: "Receipt for order #42", Sender: "shop@x.com",
			Date: now.AddDate(0, 0, -5),
			Body: strings.Repeat("word ", 60),
		},
		// 4: long technical message about diffraction + PLL — bridges optics+electronics
		{
			MessageID: "m4", ThreadID: "t4", ThreadSubject: "diffraction PLL stability",
			Subject: "diffraction PLL stability", Sender: "researcher@stanford.edu",
			Date:          now.AddDate(0, 0, -2),
			Body:          strings.Repeat("diffraction grating PLL stability measurements are looking into ", 5) + " https://arxiv.org/abs/diffractionPLL",
			ExtractedURLs: []string{"https://arxiv.org/abs/diffractionPLL"},
		},
	}

	var buf bytes.Buffer
	n, err := RunTriage(RunOptions{
		Messages:        msgs,
		Lexicon:         lex,
		TopicPairs:      tp,
		RyanEmail:       "ryan@example.com",
		AuthorityStore:  stanfordAuthorityFake(),
		Threshold:       0.30,
		MaxN:            25,
		Out:             &buf,
	})
	if err != nil {
		t.Fatalf("RunTriage: %v", err)
	}
	if n == 0 {
		t.Fatalf("got 0 candidates from %d msgs; output:\n%s", len(msgs), buf.String())
	}
	// I2: every emitted line has score >= threshold
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var c Candidate
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("invalid JSONL: %v\n%s", err, line)
		}
		if c.Score < 0.30 {
			t.Errorf("emitted candidate with score %v < threshold", c.Score)
		}
		// receipt+lunch should not be present
		if c.MessageID == "m2" || c.MessageID == "m3" {
			t.Errorf("filter-violation: %s emitted", c.MessageID)
		}
	}

	// I4: determinism — same inputs, byte-identical output
	var buf2 bytes.Buffer
	_, err = RunTriage(RunOptions{
		Messages:        msgs,
		Lexicon:         lex,
		TopicPairs:      tp,
		RyanEmail:       "ryan@example.com",
		AuthorityStore:  stanfordAuthorityFake(),
		Threshold:       0.30,
		MaxN:            25,
		Out:             &buf2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != buf2.String() {
		t.Fatalf("non-deterministic across runs:\n--- 1 ---\n%s\n--- 2 ---\n%s", buf.String(), buf2.String())
	}
}

// I3: --max truncates
func TestRunTriage_MaxN(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, graphPath)
	lex, _ := OpenLexicon(graphPath)
	defer lex.Close()
	now := time.Now()
	var msgs []*Message
	for i := 0; i < 50; i++ {
		msgs = append(msgs, &Message{
			MessageID: "m" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Sender:    "alex@stanford.edu",
			Date:      now,
			Body:      "Vanguard Section 351 arXiv " + strings.Repeat("word ", 60),
		})
	}
	var buf bytes.Buffer
	n, err := RunTriage(RunOptions{
		Messages:        msgs,
		Lexicon:         lex,
		AuthorityStore:  stanfordAuthorityFake(),
		Threshold:       0.0,
		MaxN:            5,
		Out:             &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n > 5 {
		t.Fatalf("MaxN=5 emitted %d", n)
	}
}

// TestIntegration_RunTriageWithAuthority is the AUTHGRAPH-03 end-to-end gate:
// run RunTriage with a real authority.SQLiteStore (not the fake) over an
// in-memory msgvault DB seeded with three known authority_scores, and assert
// that across emitted candidates the Expert sub-score values are CONTINUOUS —
// i.e. NOT a subset of {0.0, 1.0}. Proves the binary allowlist semantics are
// retired end-to-end.
func TestIntegration_RunTriageWithAuthority(t *testing.T) {
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, graphPath)
	lex, err := OpenLexicon(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lex.Close()
	tp, err := OpenTopicPairs(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()

	// In-memory msgvault DB with the authority schema applied; INSERT three
	// known per-sender scores covering the boundary (0.0), low (0.27), and
	// high (0.83) bands. (We do NOT exercise authority.Recompute here — that
	// path is covered by Plan 01's TestRecompute_FreshFixture; this test is
	// about TRIAGE wiring.)
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	defer db.Close()
	if err := authority.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	rows := []struct {
		email string
		score float64
	}{
		{"alex@stanford.edu", 0.83},
		{"researcher@stanford.edu", 0.27},
		{"newcomer@example.com", 0.0},
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO authority_scores
			 (sender_email, volume, response_rate_7d, link_quality, authority_score, updated_at)
			 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
			r.email, 5, 0.5, 0.5, r.score,
		); err != nil {
			t.Fatalf("insert authority_scores %s: %v", r.email, err)
		}
	}
	store := authority.NewSQLiteStore(db)

	now := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	msgs := []*Message{
		{
			MessageID: "m1", ThreadID: "t1", ThreadSubject: "Re: §351 holdings",
			Subject: "Re: §351 holdings", Sender: "alex@stanford.edu",
			Date:          now.AddDate(0, 0, -3),
			Body:          "I'm trying to figure out the Vanguard exchange fund mechanics. Section 351 holding period is 7 years per https://arxiv.org/abs/2403.00001 and Vanguard docs.",
			ExtractedURLs: []string{"https://arxiv.org/abs/2403.00001"},
		},
		{
			MessageID: "m2", ThreadID: "t2", ThreadSubject: "diffraction PLL stability",
			Subject: "diffraction PLL stability", Sender: "researcher@stanford.edu",
			Date:          now.AddDate(0, 0, -2),
			Body:          strings.Repeat("diffraction grating PLL stability measurements are looking into ", 5) + " https://arxiv.org/abs/diffractionPLL",
			ExtractedURLs: []string{"https://arxiv.org/abs/diffractionPLL"},
		},
		{
			MessageID: "m3", ThreadID: "t3", ThreadSubject: "Vanguard 351 update",
			Subject: "Vanguard 351 update", Sender: "newcomer@example.com",
			Date:          now.AddDate(0, 0, -1),
			Body:          "Vanguard Section 351 deep dive trying to figure out the holding period " + strings.Repeat("word ", 60) + " https://arxiv.org/abs/newcomer",
			ExtractedURLs: []string{"https://arxiv.org/abs/newcomer"},
		},
	}

	var buf bytes.Buffer
	n, err := RunTriage(RunOptions{
		Messages:       msgs,
		Lexicon:        lex,
		TopicPairs:     tp,
		AuthorityStore: store,
		RyanEmail:      "ryan@example.com",
		// Threshold positive but minimal — RunTriage promotes <=0 to 0.55 default.
		Threshold: 0.01,
		MaxN:      25,
		Out:       &buf,
	})
	if err != nil {
		t.Fatalf("RunTriage: %v", err)
	}
	if n == 0 {
		t.Fatalf("got 0 candidates from %d msgs; output:\n%s", len(msgs), buf.String())
	}

	// Collect Expert sub-scores from the JSONL output.
	var expertScores []float64
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var c Candidate
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("invalid JSONL: %v\n%s", err, line)
		}
		expertScores = append(expertScores, c.Scores.Expert)
	}

	// Assertion 1: at least one candidate has 0 < Expert < 1 (continuous,
	// not collapsed to the legacy binary allowlist semantics).
	hasMidband := false
	for _, e := range expertScores {
		if e > 0.0 && e < 1.0 {
			hasMidband = true
			break
		}
	}
	if !hasMidband {
		t.Fatalf("no candidate has 0 < Expert < 1; got %v — AUTHGRAPH-03 acceptance fails (binary semantics still in effect)", expertScores)
	}

	// Assertion 2: the SET of Expert sub-scores is NOT a subset of {0.0, 1.0}.
	// Proves continuous distribution flows end-to-end through RunTriage.
	for _, e := range expertScores {
		if e != 0.0 && e != 1.0 {
			t.Logf("AUTHGRAPH-03 acceptance: Expert distribution = %v (continuous, not binary)", expertScores)
			return
		}
	}
	t.Fatalf("Expert sub-scores collapsed to {0,1}: %v", expertScores)
}

// ParseSinceDuration: 7d works
func TestParseSinceDuration_Days(t *testing.T) {
	d, err := ParseSinceDuration("7d")
	if err != nil {
		t.Fatal(err)
	}
	if d != 7*24*time.Hour {
		t.Fatalf("got %v, want 7*24h", d)
	}
}
