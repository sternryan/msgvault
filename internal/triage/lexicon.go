package triage

import (
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strings"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// LexEntry is one row from forge graph.db.entities indexed by lower-cased
// canonical_name and any aliases. SourceCount drives the long-tail boost
// (rare entities weighted higher).
type LexEntry struct {
	CanonicalName string
	SourceCount   int
	TopicSlug     string
}

// Lexicon is a read-only view over forge graph.db.entities (+ entity_edges)
// for entity-name lookup during scoring. Open via OpenLexicon. The underlying
// *sql.DB is opened with ?mode=ro.
type Lexicon struct {
	db    *sql.DB
	index map[string]LexEntry // lowercased name/alias -> entry
}

// OpenLexicon opens the forge graph.db read-only and pre-loads the entity
// index. The mutecomm/go-sqlcipher driver registers itself as "sqlite3" and
// transparently reads plaintext SQLite files when no key is set, which is
// what we need for forge graph.db. (The plan originally specified mattn/
// go-sqlite3 here, but it duplicates SQLite C symbols at link time when both
// drivers ship in one binary — Rule 1 deviation, see SUMMARY.md.)
func OpenLexicon(graphDBPath string) (*Lexicon, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_query_only=true", url.QueryEscape(graphDBPath))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open lexicon %s: %w", graphDBPath, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping lexicon %s: %w", graphDBPath, err)
	}
	idx := make(map[string]LexEntry)
	// Best-effort: tolerate either schema (with or without aliases / topic_slug).
	rows, err := db.Query(`
		SELECT e.canonical_name,
		       COALESCE(e.aliases, ''),
		       COALESCE(e.source_count, 0),
		       COALESCE((SELECT topic_slug FROM entity_edges
		                 WHERE source_entity_id = e.id
		                 ORDER BY rowid LIMIT 1), '')
		FROM entities e
	`)
	if err != nil {
		// graph.db may have a slightly different schema in the field; fall back to
		// just canonical names.
		rows2, err2 := db.Query(`SELECT canonical_name, COALESCE(source_count, 0) FROM entities`)
		if err2 != nil {
			_ = db.Close()
			return nil, fmt.Errorf("lex query: %w", err)
		}
		defer rows2.Close()
		for rows2.Next() {
			var name string
			var sc int
			if err := rows2.Scan(&name, &sc); err != nil {
				continue
			}
			idx[strings.ToLower(name)] = LexEntry{CanonicalName: name, SourceCount: sc}
		}
		return &Lexicon{db: db, index: idx}, nil
	}
	defer rows.Close()
	for rows.Next() {
		var name, aliases, slug string
		var sc int
		if err := rows.Scan(&name, &aliases, &sc, &slug); err != nil {
			continue
		}
		entry := LexEntry{CanonicalName: name, SourceCount: sc, TopicSlug: slug}
		idx[strings.ToLower(name)] = entry
		if aliases != "" {
			for _, alias := range strings.Split(aliases, ",") {
				alias = strings.TrimSpace(alias)
				if alias != "" {
					idx[strings.ToLower(alias)] = entry
				}
			}
		}
	}
	return &Lexicon{db: db, index: idx}, nil
}

// Close closes the underlying DB handle.
func (l *Lexicon) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

// Size returns the number of indexed names/aliases (for tests).
func (l *Lexicon) Size() int {
	if l == nil {
		return 0
	}
	return len(l.index)
}

// MatchedEntities scans body for any indexed entity name (case-insensitive
// substring), returning the de-duplicated canonical names and a "long-tail
// weight" (higher when matched entities have low source_count).
func (l *Lexicon) MatchedEntities(body string) (matched []string, weight float64) {
	if l == nil || len(l.index) == 0 || body == "" {
		return nil, 0.0
	}
	lower := strings.ToLower(body)
	seen := make(map[string]bool)
	var totalRarity float64
	for key, entry := range l.index {
		if len(key) < 3 {
			continue
		}
		if strings.Contains(lower, key) {
			if !seen[entry.CanonicalName] {
				seen[entry.CanonicalName] = true
				matched = append(matched, entry.CanonicalName)
				// rarity: high when source_count is low. Use 1/(1+sc) → (0,1].
				totalRarity += 1.0 / float64(1+entry.SourceCount)
			}
		}
	}
	sort.Strings(matched)
	if len(matched) > 0 {
		weight = totalRarity / float64(len(matched)) // average rarity in [0,1]
	}
	return matched, weight
}

// MatchedTopics returns the unique topic slugs attributed to entities found
// in body. Used by ScoreBridge to query topic_pairs.
func (l *Lexicon) MatchedTopics(body string) []string {
	if l == nil || len(l.index) == 0 || body == "" {
		return nil
	}
	lower := strings.ToLower(body)
	seen := make(map[string]bool)
	for key, entry := range l.index {
		if len(key) < 3 || entry.TopicSlug == "" {
			continue
		}
		if strings.Contains(lower, key) {
			seen[entry.TopicSlug] = true
		}
	}
	out := make([]string, 0, len(seen))
	for slug := range seen {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}
