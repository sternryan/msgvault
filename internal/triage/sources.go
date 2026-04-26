package triage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// SourcesDB is a read-only view over forge sources.db for URL hash lookup
// (criterion #2). Open via OpenSources. Hash of "already-ingested" indicates
// the URL has been processed by forge previously and so contributes 0 to
// URL gold.
type SourcesDB struct {
	db     *sql.DB
	hashes map[string]bool // sha256(normalized URL) -> ingested
}

// OpenSources opens forge sources.db read-only and pre-loads the URL hash set.
// If the table doesn't expose URL hashes (older schema), returns an empty
// SourcesDB rather than failing — criterion #2 will treat all URLs as new.
func OpenSources(sourcesDBPath string) (*SourcesDB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_query_only=true", url.QueryEscape(sourcesDBPath))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sources %s: %w", sourcesDBPath, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sources %s: %w", sourcesDBPath, err)
	}
	s := &SourcesDB{db: db, hashes: make(map[string]bool)}
	// Try the most common forge schema first: sources(url_hash) or sources(url).
	rows, err := db.Query(`SELECT url FROM sources WHERE url IS NOT NULL`)
	if err != nil {
		// Try url_hash column instead.
		rows2, err2 := db.Query(`SELECT url_hash FROM sources WHERE url_hash IS NOT NULL`)
		if err2 != nil {
			// Schema unknown — return empty (graceful degrade).
			return s, nil
		}
		defer rows2.Close()
		for rows2.Next() {
			var h string
			if err := rows2.Scan(&h); err != nil {
				continue
			}
			s.hashes[h] = true
		}
		return s, nil
	}
	defer rows.Close()
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			continue
		}
		s.hashes[hashURL(u)] = true
	}
	return s, nil
}

// Close closes the underlying DB handle.
func (s *SourcesDB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// HasURL returns true if this URL (normalized + sha256) has been ingested.
func (s *SourcesDB) HasURL(rawURL string) bool {
	if s == nil {
		return false
	}
	return s.hashes[hashURL(rawURL)]
}

// hashURL normalizes a URL (lowercased host, no trailing slash, no fragment)
// and returns the hex sha256.
func hashURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		sum := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(sum[:])
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	normalized := strings.TrimRight(u.String(), "/")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// AddURLForTest is a test helper that lets tests pre-populate the hash set
// without writing to a real sources.db file.
func (s *SourcesDB) AddURLForTest(rawURL string) {
	if s.hashes == nil {
		s.hashes = make(map[string]bool)
	}
	s.hashes[hashURL(rawURL)] = true
}
