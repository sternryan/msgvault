package triage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// buildFixtureGraphDB materializes a forge-shaped graph.db at the given path
// from testdata/forge_graph.sql.
func buildFixtureGraphDB(t *testing.T, path string) {
	t.Helper()
	sqlPath := filepath.Join("testdata", "forge_graph.sql")
	sqlBytes, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatalf("read forge_graph.sql: %v", err)
	}
	_ = os.Remove(path)
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	_ = db.Close()
}

// L1: 3 entity matches → vocab >= 0.8
func TestLexicon_MatchesMany(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, dbPath)
	lex, err := OpenLexicon(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lex.Close()
	body := "The Vanguard exchange fund uses a Section 351 contribution and arXiv papers."
	matched, weight := lex.MatchedEntities(body)
	if len(matched) < 3 {
		t.Fatalf("got %d matches (%v), want >=3 (idx size %d)", len(matched), matched, lex.Size())
	}
	if weight <= 0 {
		t.Fatalf("zero weight, want >0")
	}
}

// L3: 0 matches → empty
func TestLexicon_NoMatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, dbPath)
	lex, _ := OpenLexicon(dbPath)
	defer lex.Close()
	matched, _ := lex.MatchedEntities("Quick lunch tomorrow at noon.")
	if len(matched) != 0 {
		t.Fatalf("got %v, want none", matched)
	}
}

// L4: long-tail boost — rare entity yields higher weight than dense
func TestLexicon_LongTailBoost(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, dbPath)
	lex, _ := OpenLexicon(dbPath)
	defer lex.Close()
	_, rareWeight := lex.MatchedEntities("Section 351 sec 351 transactions are rare.")
	_, denseWeight := lex.MatchedEntities("Vanguard Vanguard Vanguard funds.")
	if rareWeight <= denseWeight {
		t.Fatalf("expected rare weight (%v) > dense weight (%v)", rareWeight, denseWeight)
	}
}

// L5: read-only enforcement — INSERT fails
func TestLexicon_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, dbPath)
	lex, err := OpenLexicon(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lex.Close()
	_, err = lex.db.Exec(`INSERT INTO entities(id, canonical_name) VALUES (99, 'shouldfail')`)
	if err == nil {
		t.Fatal("expected RO error on INSERT, got nil")
	}
}

// TP1+TP2+TP3: topic_pairs read + bridge classification
func TestTopicPairs_Read(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, dbPath)
	tp, err := OpenTopicPairs(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	if tp.Size() != 3 {
		t.Fatalf("got %d pairs, want 3", tp.Size())
	}
	pairs := tp.BridgePairsFor([]string{"optics"})
	if len(pairs) != 2 {
		t.Fatalf("got %d optics pairs, want 2", len(pairs))
	}
}

// TP4: empty pairs → no panic
func TestTopicPairs_Empty(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	// build minimal DB without topic_pairs
	db, _ := sql.Open("sqlite3", dbPath)
	_, _ = db.Exec(`CREATE TABLE entities (id INTEGER PRIMARY KEY, canonical_name TEXT, aliases TEXT, source_count INT)`)
	_ = db.Close()
	tp, err := OpenTopicPairs(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	pairs := tp.BridgePairsFor([]string{"optics"})
	if len(pairs) != 0 {
		t.Fatalf("got %d, want 0", len(pairs))
	}
}

// ScoreVocab integration: vocab > 0 when entities match
func TestScoreVocab_WithLexicon(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, dbPath)
	lex, _ := OpenLexicon(dbPath)
	defer lex.Close()
	m := &Message{Body: "Vanguard, Section 351, arXiv all in one body."}
	got := ScoreVocab(m, lex)
	if got < 0.8 {
		t.Fatalf("vocab = %v, want >= 0.8", got)
	}
}

// ScoreBridge integration: matched topics + bridge pairs → 1.0
func TestScoreBridge_WithFixture(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")
	buildFixtureGraphDB(t, dbPath)
	lex, _ := OpenLexicon(dbPath)
	defer lex.Close()
	tp, _ := OpenTopicPairs(dbPath)
	defer tp.Close()
	// Body mentions Diffraction (optics topic) — fixture has 2 high-bridge pairs touching optics.
	m := &Message{Body: "diffraction grating measurements and PLL stability."}
	got := ScoreBridge(m, lex, tp)
	if got != 1.0 {
		t.Fatalf("bridge = %v, want 1.0", got)
	}
}
