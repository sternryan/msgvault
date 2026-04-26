package triage

import (
	"net/url"
	"regexp"
	"strings"
)

// Composite computes the 7-criterion composite score per TRIAGE-02:
//
//	Σ(weight_i × score_i)
//
// Inputs are 0..1 per-criterion scores; output is 0..1 (when weights sum to 1).
func Composite(s Scores, w Weights) float64 {
	return s.Vocab*w.Vocab +
		s.URLGold*w.URLGold +
		s.Curiosity*w.Curiosity +
		s.Recurrence*w.Recurrence +
		s.Bridge*w.Bridge +
		s.Decision*w.Decision +
		s.Expert*w.Expert
}

// DefaultHighSignalDomains is the canonical short list of domains whose URLs
// score 1.0 on URL gold criterion (#2) when not already in sources.db.
var DefaultHighSignalDomains = []string{
	"arxiv.org",
	"substack.com",
	"news.ycombinator.com",
	"github.com",
	"ssrn.com",
}

// ScoreVocab implements criterion #1 (entity lexicon match).
// Returns 0..1 based on how many forge entities appear in m.Body and the
// long-tail-boosted weight returned by the lexicon. lex may be nil; in that
// case the criterion returns 0.0 (graceful degradation when graph.db absent).
func ScoreVocab(m *Message, lex *Lexicon) float64 {
	if lex == nil || m == nil {
		return 0.0
	}
	matched, weight := lex.MatchedEntities(m.Body)
	count := len(matched)
	switch {
	case count == 0:
		return 0.0
	case count <= 2:
		// Long-tail-boosted partial credit.
		score := 0.5 + 0.1*weight
		if score > 1.0 {
			score = 1.0
		}
		return score
	default:
		score := 0.8 + 0.1*weight
		if score > 1.0 {
			score = 1.0
		}
		return score
	}
}

// ScoreURLGold implements criterion #2 (URL gold).
// For each URL in m.ExtractedURLs, score 1.0 if its host is in
// highSignalDomains AND its hash is NOT yet in sources.db (sources may be
// nil; treated as "no prior ingest"). Returns max across URLs.
func ScoreURLGold(m *Message, sources *SourcesDB, highSignalDomains []string) float64 {
	if m == nil || len(m.ExtractedURLs) == 0 {
		return 0.0
	}
	if highSignalDomains == nil {
		highSignalDomains = DefaultHighSignalDomains
	}
	hostSet := make(map[string]bool, len(highSignalDomains))
	for _, d := range highSignalDomains {
		hostSet[strings.ToLower(d)] = true
	}
	max := 0.0
	for _, raw := range m.ExtractedURLs {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		host := strings.ToLower(u.Host)
		// strip leading www.
		host = strings.TrimPrefix(host, "www.")
		if !hostSet[host] {
			continue
		}
		// High-signal host. If sources.db has it, it's already ingested → 0.
		if sources != nil && sources.HasURL(raw) {
			// Already ingested; this URL contributes 0.
			continue
		}
		// New high-signal URL → 1.0
		if 1.0 > max {
			max = 1.0
		}
	}
	return max
}

var (
	curiosityRe = regexp.MustCompile(`(?i)(trying to figure out|anyone know|considering|rabbit hole|deep dive|looking into)`)
	decisionRe  = regexp.MustCompile(`(?i)(we decided|shipping|pivoting|passed on|going with)`)
)

// ScoreCuriosity implements criterion #3.
// 1.0 if Ryan-as-sender, 0.6 if sender is in trustedContacts, 0.2 otherwise,
// 0.0 if no curiosity marker present.
func ScoreCuriosity(m *Message, trustedContacts []string, ryanEmail string) float64 {
	if m == nil || !curiosityRe.MatchString(m.Body) {
		return 0.0
	}
	sender := strings.ToLower(strings.TrimSpace(m.Sender))
	if ryanEmail != "" && sender == strings.ToLower(strings.TrimSpace(ryanEmail)) {
		return 1.0
	}
	for _, c := range trustedContacts {
		if strings.ToLower(strings.TrimSpace(c)) == sender {
			return 0.6
		}
	}
	return 0.2
}

// ScoreRecurrence implements criterion #4 (proper-noun recurrence).
// Buckets: 1 occurrence → 0.0; 2-3 in 30d → 0.3; 4-5 in 60d → 0.7; 6+ in 90d → 1.0.
// rec may be nil → returns 0.0.
func ScoreRecurrence(m *Message, rec *Recurrence) float64 {
	if rec == nil || m == nil {
		return 0.0
	}
	count := rec.Count(m)
	switch {
	case count >= 6:
		return 1.0
	case count >= 4:
		return 0.7
	case count >= 2:
		return 0.3
	default:
		return 0.0
	}
}

// ScoreBridge implements criterion #5 (bridge potential).
// Reads forge topic_pairs and counts pairs where (cosine_similarity < 0.3 AND
// shared_entities < 3) for the topics matched by lex on m.Body.
// 0 → 0.0; 1 → 0.5; 2+ → 1.0.
func ScoreBridge(m *Message, lex *Lexicon, pairs *TopicPairs) float64 {
	if lex == nil || pairs == nil || m == nil {
		return 0.0
	}
	_, _ = lex.MatchedEntities(m.Body)
	matchedTopics := lex.MatchedTopics(m.Body)
	if len(matchedTopics) == 0 {
		return 0.0
	}
	bridgeCount := 0
	for _, p := range pairs.BridgePairsFor(matchedTopics) {
		if p.CosineSimilarity < 0.3 && p.SharedEntities < 3 {
			bridgeCount++
		}
	}
	switch {
	case bridgeCount >= 2:
		return 1.0
	case bridgeCount == 1:
		return 0.5
	default:
		return 0.0
	}
}

// ScoreDecision implements criterion #6.
// 1.0 if Ryan-as-sender + decision marker; 0.5 if any other sender + decision marker; 0.0 otherwise.
func ScoreDecision(m *Message, ryanEmail string) float64 {
	if m == nil || !decisionRe.MatchString(m.Body) {
		return 0.0
	}
	sender := strings.ToLower(strings.TrimSpace(m.Sender))
	if ryanEmail != "" && sender == strings.ToLower(strings.TrimSpace(ryanEmail)) {
		return 1.0
	}
	return 0.5
}

// ScoreExpert implements criterion #7.
// 1.0 if sender domain is in expertAllowlist; 0.0 otherwise (including nil/empty).
func ScoreExpert(m *Message, expertAllowlist []string) float64 {
	if m == nil || len(expertAllowlist) == 0 {
		return 0.0
	}
	sender := strings.ToLower(strings.TrimSpace(m.Sender))
	at := strings.LastIndex(sender, "@")
	if at < 0 || at == len(sender)-1 {
		return 0.0
	}
	domain := sender[at+1:]
	for _, allowed := range expertAllowlist {
		if strings.EqualFold(strings.TrimSpace(allowed), domain) {
			return 1.0
		}
	}
	return 0.0
}
