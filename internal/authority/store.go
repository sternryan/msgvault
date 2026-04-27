package authority

import (
	"database/sql"
	"strings"
)

// Store is the read-side authority interface consumed by msgvault triage
// (Plan 16-03 rewires triage.ScoreExpert to take a Store). Locked by
// Phase 16 D-06: per-call Score lookup, lower+trim canonicalisation,
// (0, false) on miss OR query error (graceful — triage scoring must not
// panic if the authority backend is unavailable).
type Store interface {
	Score(email string) (float64, bool)
}

// SQLiteStore is the production Store implementation backed by the
// authority_scores table populated by Recompute (see schema.sql).
//
// No LRU cache in v1 (CONTEXT.md A4). Per-call SELECT against the
// PRIMARY KEY is fast enough for triage's 5k-message-per-week ceiling;
// add a cache only if a profile says we need one.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore returns a SQLiteStore backed by db. The caller retains
// ownership of db (NewSQLiteStore does NOT close it).
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// Score returns (authority_score, true) for a known sender or (0, false)
// for an unknown sender OR when the underlying query errors (e.g. closed
// DB connection). Never panics. Email is lower+trim-normalized before
// lookup so triage callers don't have to pre-canonicalize.
func (s *SQLiteStore) Score(email string) (float64, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, false
	}
	if s == nil || s.db == nil {
		return 0, false
	}
	var score float64
	err := s.db.QueryRow(
		`SELECT authority_score FROM authority_scores WHERE sender_email = ?`,
		email,
	).Scan(&score)
	if err != nil {
		// Graceful: ErrNoRows OR any other query error (closed DB,
		// schema missing, etc). Triage degrades to baseline rather
		// than crashing.
		return 0, false
	}
	return score, true
}
