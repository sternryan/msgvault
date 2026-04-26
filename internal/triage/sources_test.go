package triage

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// U2 (full RO path): build a fixture sources.db with one URL pre-ingested
// and assert ScoreURLGold returns 0 for that URL while a fresh URL gets 1.
func TestSourcesDB_OpenAndLookup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sources.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sources (id INTEGER PRIMARY KEY, url TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sources(url) VALUES ('https://arxiv.org/abs/2403.00001')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	src, err := OpenSources(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	// Already-ingested URL → 0
	m1 := &Message{ExtractedURLs: []string{"https://arxiv.org/abs/2403.00001"}}
	if got := ScoreURLGold(m1, src, nil); got != 0.0 {
		t.Fatalf("ingested url got %v, want 0.0", got)
	}
	// Fresh high-signal URL → 1
	m2 := &Message{ExtractedURLs: []string{"https://github.com/foo/bar"}}
	if got := ScoreURLGold(m2, src, nil); got != 1.0 {
		t.Fatalf("fresh url got %v, want 1.0", got)
	}
}

// Read-only enforcement on sources.db
func TestSourcesDB_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sources.db")
	db, _ := sql.Open("sqlite3", dbPath)
	_, _ = db.Exec(`CREATE TABLE sources (id INTEGER PRIMARY KEY, url TEXT)`)
	_ = db.Close()
	src, err := OpenSources(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.db.Exec(`INSERT INTO sources(url) VALUES ('x')`); err == nil {
		t.Fatal("expected RO error on INSERT")
	}
}
