package triage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func sampleCandidates() []Candidate {
	topic := "vanguard-funds"
	return []Candidate{
		{
			MessageID: "msg1", ThreadID: "t1", ThreadSubject: "Re: §351 holdings",
			Sender: "alex@example.com", Date: "2026-04-22T10:00:00Z",
			Score:           0.83,
			Scores:          Scores{Vocab: 0.8, URLGold: 1.0, Curiosity: 0.6, Recurrence: 0.3, Bridge: 0.5, Decision: 0.0, Expert: 1.0},
			Snippet:         "Quick thought on §351 holding period",
			ExtractedURLs:   []string{"https://vanguard.com/exchange-fund-faq"},
			MatchedEntities: []string{"§351"},
			SuggestedTopic:  &topic,
		},
		{
			MessageID: "msg2", ThreadID: "t2", ThreadSubject: "Q4 plan",
			Sender: "ryan@example.com", Date: "2026-04-21T08:00:00Z",
			Score:          0.55,
			Scores:         Scores{Vocab: 0.5, URLGold: 0.5, Curiosity: 0.5, Recurrence: 0.5, Bridge: 0.5, Decision: 0.5, Expert: 0.5},
			SuggestedTopic: nil,
		},
	}
}

// J1: each line parses as Candidate
func TestEmit_LineByLine(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, sampleCandidates()); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	for i, ln := range lines {
		var c Candidate
		if err := json.Unmarshal([]byte(ln), &c); err != nil {
			t.Errorf("line %d: %v", i, err)
		}
	}
}

// J2: scores has exactly 7 keys
func TestEmit_ScoresKeyCount(t *testing.T) {
	var buf bytes.Buffer
	_ = Emit(&buf, sampleCandidates()[:1])
	var raw map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &raw); err != nil {
		t.Fatal(err)
	}
	scores, ok := raw["scores"].(map[string]any)
	if !ok {
		t.Fatal("no scores object")
	}
	if len(scores) != 7 {
		t.Fatalf("scores has %d keys, want 7", len(scores))
	}
	for _, want := range []string{"vocab", "url_gold", "curiosity", "recurrence", "bridge", "decision", "expert"} {
		if _, ok := scores[want]; !ok {
			t.Errorf("missing scores.%s", want)
		}
	}
}

// J4: nil SuggestedTopic renders as null
func TestEmit_NilSuggestedTopic(t *testing.T) {
	var buf bytes.Buffer
	_ = Emit(&buf, []Candidate{sampleCandidates()[1]})
	if !strings.Contains(buf.String(), `"suggested_topic":null`) {
		t.Fatalf("missing null suggested_topic in: %s", buf.String())
	}
}

// J5: byte-identical across runs
func TestEmit_Deterministic(t *testing.T) {
	cs := sampleCandidates()
	var a, b bytes.Buffer
	_ = Emit(&a, cs)
	_ = Emit(&b, cs)
	if a.String() != b.String() {
		t.Fatalf("emit not deterministic")
	}
}
