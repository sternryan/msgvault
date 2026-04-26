package triage

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// Pair is one row from forge graph.db.topic_pairs.
type Pair struct {
	TopicA           string
	TopicB           string
	SharedEntities   int
	CosineSimilarity float64
	EdgeCount        int
}

// TopicPairs is the read-only cache of forge graph.db.topic_pairs loaded once
// at startup. ScoreBridge filters pairs by matched topic slugs.
type TopicPairs struct {
	db    *sql.DB
	pairs []Pair
}

// OpenTopicPairs opens forge graph.db read-only and loads all topic_pairs
// rows. If the table doesn't exist (forge ran an older audit-bridges build)
// returns an empty TopicPairs, not an error.
func OpenTopicPairs(graphDBPath string) (*TopicPairs, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_query_only=true", url.QueryEscape(graphDBPath))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open topic_pairs %s: %w", graphDBPath, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping topic_pairs %s: %w", graphDBPath, err)
	}
	tp := &TopicPairs{db: db}
	rows, err := db.Query(`SELECT topic_a, topic_b, shared_entities, cosine_similarity, edge_count FROM topic_pairs`)
	if err != nil {
		// Table may not exist yet (forge hasn't written it) — return empty.
		return tp, nil
	}
	defer rows.Close()
	for rows.Next() {
		var p Pair
		if err := rows.Scan(&p.TopicA, &p.TopicB, &p.SharedEntities, &p.CosineSimilarity, &p.EdgeCount); err != nil {
			continue
		}
		tp.pairs = append(tp.pairs, p)
	}
	return tp, nil
}

// Close closes the underlying DB handle.
func (tp *TopicPairs) Close() error {
	if tp == nil || tp.db == nil {
		return nil
	}
	return tp.db.Close()
}

// Size returns the number of cached pairs (for tests).
func (tp *TopicPairs) Size() int {
	if tp == nil {
		return 0
	}
	return len(tp.pairs)
}

// BridgePairsFor returns all topic_pairs that include any topic in the
// matchedTopics set. Caller filters further by cosine/shared thresholds.
func (tp *TopicPairs) BridgePairsFor(matchedTopics []string) []Pair {
	if tp == nil || len(tp.pairs) == 0 || len(matchedTopics) == 0 {
		return nil
	}
	want := make(map[string]bool, len(matchedTopics))
	for _, t := range matchedTopics {
		want[t] = true
	}
	out := make([]Pair, 0)
	for _, p := range tp.pairs {
		if want[p.TopicA] || want[p.TopicB] {
			out = append(out, p)
		}
	}
	return out
}
