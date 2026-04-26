package triage

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// RunOptions configures a single triage run. Code paths share this struct so
// the cobra command and the integration test exercise the same logic.
type RunOptions struct {
	// Messages is the candidate set to score. The cobra command populates it
	// from the msgvault store; tests pass fixtures directly.
	Messages []*Message

	// Lexicon is the forge entity index. May be nil → ScoreVocab/ScoreBridge
	// degrade to 0 for that criterion.
	Lexicon *Lexicon

	// TopicPairs is the forge bridge cache. May be nil.
	TopicPairs *TopicPairs

	// Sources is the forge URL ingest history. May be nil.
	Sources *SourcesDB

	// Recurrence index built over a recent corpus (typically the same window
	// as Messages or a longer lookback). May be nil → criterion #4 returns 0.
	Recurrence *Recurrence

	// TrustedContacts is the curiosity allowlist (criterion #3).
	TrustedContacts []string

	// ExpertAllowlist is the expert-domain allowlist (criterion #7).
	ExpertAllowlist []string

	// HighSignalDomains overrides DefaultHighSignalDomains (used by both the
	// short-no-URL filter and ScoreURLGold).
	HighSignalDomains []string

	// RyanEmail biases curiosity/decision scores when the message sender
	// matches the user's primary address.
	RyanEmail string

	// Threshold is the cutoff for inclusion in the JSONL output.
	Threshold float64

	// MaxN truncates after sorting (typically 25).
	MaxN int

	// Out is where the JSONL is written.
	Out io.Writer
}

// RunTriage scores the messages in opts, applies filters/threshold/sort, and
// writes JSONL to opts.Out. Returns the number of emitted candidates and any
// error. Pure logic — no msgvault store knowledge — so tests can exercise it
// without standing up a full DB.
func RunTriage(opts RunOptions) (int, error) {
	if opts.Out == nil {
		return 0, fmt.Errorf("RunTriage: nil Out")
	}
	threshold := opts.Threshold
	if threshold <= 0 {
		threshold = 0.55
	}
	maxN := opts.MaxN
	if maxN <= 0 {
		maxN = 25
	}

	cands := make([]Candidate, 0, len(opts.Messages))
	for _, m := range opts.Messages {
		if m == nil {
			continue
		}
		// Apply hard filters first (TRIAGE-03).
		drop, _, err := ShouldDrop(m, opts.RyanEmail, opts.HighSignalDomains)
		if err != nil {
			return 0, fmt.Errorf("filter %s: %w", m.MessageID, err)
		}
		if drop {
			continue
		}
		// Score per-criterion.
		s := Scores{
			Vocab:      ScoreVocab(m, opts.Lexicon),
			URLGold:    ScoreURLGold(m, opts.Sources, opts.HighSignalDomains),
			Curiosity:  ScoreCuriosity(m, opts.TrustedContacts, opts.RyanEmail),
			Recurrence: ScoreRecurrence(m, opts.Recurrence),
			Bridge:     ScoreBridge(m, opts.Lexicon, opts.TopicPairs),
			Decision:   ScoreDecision(m, opts.RyanEmail),
			Expert:     ScoreExpert(m, opts.ExpertAllowlist),
		}
		composite := Composite(s, DefaultWeights)
		if composite < threshold {
			continue
		}
		var matched []string
		if opts.Lexicon != nil {
			matched, _ = opts.Lexicon.MatchedEntities(m.Body)
		}
		urls := m.ExtractedURLs
		if len(urls) == 0 {
			urls = ExtractURLs(m.Body)
		}
		snippet := m.Body
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		cands = append(cands, Candidate{
			MessageID:       m.MessageID,
			ThreadID:        m.ThreadID,
			ThreadSubject:   firstNonEmpty(m.ThreadSubject, m.Subject),
			Sender:          m.Sender,
			Date:            m.Date.UTC().Format(time.RFC3339),
			Score:           composite,
			Scores:          s,
			Snippet:         strings.TrimSpace(snippet),
			ExtractedURLs:   urls,
			MatchedEntities: matched,
			SuggestedTopic:  nil,
		})
	}

	out := SelectAndSort(cands, threshold, maxN)
	if err := Emit(opts.Out, out); err != nil {
		return 0, err
	}
	return len(out), nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ParseSinceDuration accepts "Nd" (days) in addition to time.ParseDuration's
// h/m/s units. Returns time.Duration.
func ParseSinceDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err != nil {
			return 0, fmt.Errorf("parse days %q: %w", s, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
