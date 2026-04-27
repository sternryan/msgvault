package authority

import (
	"database/sql"
	"testing"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// freshStoreDB returns a :memory: msgvault DB with InitSchema applied
// (no fixture rows). Tests INSERT into authority_scores directly.
func freshStoreDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return db
}

func insertScore(t *testing.T, db *sql.DB, email string, score float64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO authority_scores (sender_email, volume, response_rate_7d, link_quality, authority_score)
		 VALUES (?, 0, 0, 0, ?)`,
		email, score,
	)
	if err != nil {
		t.Fatalf("insert authority_scores(%s, %v): %v", email, score, err)
	}
}

func TestSQLiteStore_Score_Hit(t *testing.T) {
	db := freshStoreDB(t)
	insertScore(t, db, "alice@example.com", 0.42)
	s := NewSQLiteStore(db)
	got, ok := s.Score("alice@example.com")
	if !ok {
		t.Fatalf("Score: ok=false, want true")
	}
	if got < 0.4199 || got > 0.4201 {
		t.Fatalf("Score = %v, want 0.42 (±0.0001)", got)
	}
}

func TestSQLiteStore_Score_Miss(t *testing.T) {
	db := freshStoreDB(t)
	s := NewSQLiteStore(db)
	got, ok := s.Score("ghost@nowhere")
	if ok {
		t.Fatalf("Score: ok=true, want false")
	}
	if got != 0.0 {
		t.Fatalf("Score = %v, want 0.0", got)
	}
}

func TestSQLiteStore_Score_Normalization(t *testing.T) {
	db := freshStoreDB(t)
	insertScore(t, db, "alice@example.com", 0.42)
	s := NewSQLiteStore(db)
	got, ok := s.Score("  ALICE@Example.COM  ")
	if !ok {
		t.Fatalf("Score (normalized): ok=false, want true")
	}
	if got < 0.4199 || got > 0.4201 {
		t.Fatalf("Score (normalized) = %v, want 0.42 (±0.0001)", got)
	}
}

func TestSQLiteStore_Score_GracefulOnError(t *testing.T) {
	db := freshStoreDB(t)
	s := NewSQLiteStore(db)
	// Close DB → subsequent QueryRow.Scan returns an error (NOT ErrNoRows).
	// Score must return (0, false) without panicking.
	_ = db.Close()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Score panicked on closed DB: %v", r)
		}
	}()
	got, ok := s.Score("anyone@anywhere")
	if ok {
		t.Fatalf("Score: ok=true on closed DB, want false")
	}
	if got != 0.0 {
		t.Fatalf("Score = %v on closed DB, want 0.0", got)
	}
}

// fakeStore is a map-backed Store for tests of downstream consumers
// (Plan 03 triage rewire). Compile-time interface satisfaction proof.
type fakeStore struct {
	scores map[string]float64
}

func (f fakeStore) Score(email string) (float64, bool) {
	v, ok := f.scores[email]
	return v, ok
}

var _ Store = fakeStore{}
var _ Store = (*SQLiteStore)(nil)

func TestStoreInterface_FakeSatisfies(t *testing.T) {
	var s Store = fakeStore{scores: map[string]float64{"x@y.com": 0.7}}
	got, ok := s.Score("x@y.com")
	if !ok || got != 0.7 {
		t.Fatalf("fake.Score = (%v, %v), want (0.7, true)", got, ok)
	}
	if _, ok := s.Score("missing@z"); ok {
		t.Fatalf("fake.Score missing returned ok=true")
	}
}
