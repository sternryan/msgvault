package triage

import (
	"regexp"
	"strings"
	"time"
)

// Recurrence tracks proper-noun frequency across a corpus of recent messages.
// ScoreRecurrence buckets the count returned by Count(m).
//
// The simple in-memory implementation tokenizes message bodies into capitalized
// words and counts how many distinct messages within a lookback window contain
// any token also present in the target message. This avoids dependence on
// msgvault's entities table (which is populated by a separate AI pipeline).
type Recurrence struct {
	// Mentions: token -> set of (messageID, date). Built via Add().
	mentions map[string]map[string]time.Time
	now      time.Time
}

// NewRecurrence builds a Recurrence index from a corpus of past messages.
// Pass time.Now() (or a fixed time in tests) as `now`.
func NewRecurrence(corpus []*Message, now time.Time) *Recurrence {
	r := &Recurrence{
		mentions: make(map[string]map[string]time.Time),
		now:      now,
	}
	for _, m := range corpus {
		r.add(m)
	}
	return r
}

var properNounRe = regexp.MustCompile(`\b[A-Z][a-zA-Z]{3,}\b`)

func extractProperNouns(body string) map[string]bool {
	out := make(map[string]bool)
	for _, tok := range properNounRe.FindAllString(body, -1) {
		out[strings.ToLower(tok)] = true
	}
	return out
}

func (r *Recurrence) add(m *Message) {
	if r == nil || m == nil {
		return
	}
	for tok := range extractProperNouns(m.Body) {
		if r.mentions[tok] == nil {
			r.mentions[tok] = make(map[string]time.Time)
		}
		r.mentions[tok][m.MessageID] = m.Date
	}
}

// Count returns the maximum recurrence count of any proper noun in m, looking
// back 90d from r.now (callers map count → bucket via ScoreRecurrence).
func (r *Recurrence) Count(m *Message) int {
	if r == nil || m == nil {
		return 0
	}
	cutoff := r.now.AddDate(0, 0, -90)
	max := 0
	for tok := range extractProperNouns(m.Body) {
		threads, ok := r.mentions[tok]
		if !ok {
			continue
		}
		count := 0
		for _, t := range threads {
			if t.After(cutoff) || t.Equal(cutoff) {
				count++
			}
		}
		if count > max {
			max = count
		}
	}
	return max
}
