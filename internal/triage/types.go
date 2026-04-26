// Package triage implements the msgvault triage pipeline: composite scoring,
// hard filters, and JSONL emission for weekly digest candidates.
//
// Per Phase 14 (TRIAGE-01..06,08) the package is read-only against forge state
// (graph.db, sources.db) and pure-lexical/SQL — NO LLM/encoder calls in the
// scoring hot path (D-04).
package triage

import "time"

// Message is the in-memory representation of a candidate message pulled from
// the msgvault store and prepared for scoring. RawMIME is the zlib-decompressed
// MIME bytes; Headers is populated lazily by filters.loadHeaders.
type Message struct {
	MessageID     string
	ThreadID      string
	ThreadSubject string
	Subject       string
	Sender        string // RFC822 mailbox address
	Date          time.Time
	Body          string // plaintext body
	RawMIME       []byte // zlib-decompressed MIME bytes for header parsing
	Headers       map[string]string
	headersLoaded bool
	Recipients    Recipients
	ContentType   string
	WordCount     int
	ExtractedURLs []string
}

// Recipients holds the To/Cc/Bcc address lists for a message.
type Recipients struct {
	To  []string
	Cc  []string
	Bcc []string
}

// Candidate is the JSONL row schema (TRIAGE-05). All 11 fields are required;
// SuggestedTopic is a *string so it renders as JSON null when absent.
type Candidate struct {
	MessageID       string   `json:"message_id"`
	ThreadID        string   `json:"thread_id"`
	ThreadSubject   string   `json:"thread_subject"`
	Sender          string   `json:"sender"`
	Date            string   `json:"date"` // RFC3339
	Score           float64  `json:"score"`
	Scores          Scores   `json:"scores"`
	Snippet         string   `json:"snippet"`
	ExtractedURLs   []string `json:"extracted_urls"`
	MatchedEntities []string `json:"matched_entities"`
	SuggestedTopic  *string  `json:"suggested_topic"`
}

// Weights holds per-criterion weights for the composite scorer (TRIAGE-02).
// Sum must equal 1.0 (tested in scorer_test.go).
type Weights struct {
	Vocab      float64
	URLGold    float64
	Curiosity  float64
	Recurrence float64
	Bridge     float64
	Decision   float64
	Expert     float64
}

// DefaultWeights are the locked Phase 14 weights:
// vocab 0.25, url_gold 0.20, curiosity 0.15, recurrence 0.15,
// bridge 0.10, decision 0.10, expert 0.05 (sum = 1.00).
var DefaultWeights = Weights{
	Vocab:      0.25,
	URLGold:    0.20,
	Curiosity:  0.15,
	Recurrence: 0.15,
	Bridge:     0.10,
	Decision:   0.10,
	Expert:     0.05,
}

// Scores holds the per-criterion scores in the JSONL row.
type Scores struct {
	Vocab      float64 `json:"vocab"`
	URLGold    float64 `json:"url_gold"`
	Curiosity  float64 `json:"curiosity"`
	Recurrence float64 `json:"recurrence"`
	Bridge     float64 `json:"bridge"`
	Decision   float64 `json:"decision"`
	Expert     float64 `json:"expert"`
}
