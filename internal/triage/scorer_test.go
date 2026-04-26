package triage

import (
	"math"
	"testing"
	"time"
)

func almostEqual(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// S1, S2, S3, S5: composite arithmetic.
func TestComposite_AllOnes(t *testing.T) {
	got := Composite(Scores{1, 1, 1, 1, 1, 1, 1}, DefaultWeights)
	if !almostEqual(got, 1.0, 1e-9) {
		t.Fatalf("Composite all-ones = %v, want 1.0", got)
	}
}

func TestComposite_AllZeros(t *testing.T) {
	got := Composite(Scores{}, DefaultWeights)
	if got != 0.0 {
		t.Fatalf("Composite zeros = %v, want 0.0", got)
	}
}

func TestComposite_HalfFixture(t *testing.T) {
	s := Scores{Vocab: 0.5, URLGold: 0.5, Curiosity: 0.5, Recurrence: 0.5, Bridge: 0.5, Decision: 0.5, Expert: 0.5}
	got := Composite(s, DefaultWeights)
	if !almostEqual(got, 0.5, 1e-9) {
		t.Fatalf("Composite half = %v, want 0.5", got)
	}
}

// S4: weights sum to 1.0
func TestDefaultWeights_SumToOne(t *testing.T) {
	w := DefaultWeights
	sum := w.Vocab + w.URLGold + w.Curiosity + w.Recurrence + w.Bridge + w.Decision + w.Expert
	if !almostEqual(sum, 1.0, 1e-9) {
		t.Fatalf("default weights sum = %v, want 1.0", sum)
	}
}

// TRIAGE-02 fixture: hand-labeled per-criterion -> composite within 1e-3
func TestComposite_HandLabeledFixture(t *testing.T) {
	// Hand-labeled: vocab=0.8, url_gold=1.0, curiosity=0.6, recurrence=0.3,
	// bridge=0.5, decision=0.0, expert=1.0
	// Expected: 0.25*0.8 + 0.20*1.0 + 0.15*0.6 + 0.15*0.3 + 0.10*0.5 + 0.10*0.0 + 0.05*1.0
	//        = 0.20 + 0.20 + 0.09 + 0.045 + 0.05 + 0.0 + 0.05 = 0.635
	s := Scores{Vocab: 0.8, URLGold: 1.0, Curiosity: 0.6, Recurrence: 0.3, Bridge: 0.5, Decision: 0.0, Expert: 1.0}
	got := Composite(s, DefaultWeights)
	if !almostEqual(got, 0.635, 1e-3) {
		t.Fatalf("Composite fixture = %v, want 0.635 (±0.001)", got)
	}
}

// C1: curiosity, ryan as sender
func TestScoreCuriosity_Ryan(t *testing.T) {
	m := &Message{Sender: "ryan@example.com", Body: "I'm trying to figure out how vaults work."}
	got := ScoreCuriosity(m, nil, "ryan@example.com")
	if got != 1.0 {
		t.Fatalf("curiosity ryan = %v, want 1.0", got)
	}
}

// C2: curiosity, trusted contact
func TestScoreCuriosity_Trusted(t *testing.T) {
	m := &Message{Sender: "alex@example.com", Body: "anyone know a good lawyer?"}
	got := ScoreCuriosity(m, []string{"alex@example.com"}, "ryan@example.com")
	if !almostEqual(got, 0.6, 1e-9) {
		t.Fatalf("curiosity trusted = %v, want 0.6", got)
	}
}

// C3: marker, unknown sender
func TestScoreCuriosity_Unknown(t *testing.T) {
	m := &Message{Sender: "stranger@x.com", Body: "looking into RAG eval"}
	got := ScoreCuriosity(m, nil, "ryan@example.com")
	if !almostEqual(got, 0.2, 1e-9) {
		t.Fatalf("curiosity unknown = %v, want 0.2", got)
	}
}

// C4: no marker
func TestScoreCuriosity_NoMarker(t *testing.T) {
	m := &Message{Sender: "stranger@x.com", Body: "Hi, here's the report."}
	got := ScoreCuriosity(m, nil, "ryan@example.com")
	if got != 0.0 {
		t.Fatalf("curiosity nomarker = %v, want 0.0", got)
	}
}

// D1: decision marker
func TestScoreDecision_Marker(t *testing.T) {
	m := &Message{Sender: "alex@x.com", Body: "We decided to ship next week."}
	got := ScoreDecision(m, "ryan@example.com")
	if got < 0.5 {
		t.Fatalf("decision marker = %v, want >= 0.5", got)
	}
}

// D2: ryan-as-sender weighted higher
func TestScoreDecision_Ryan(t *testing.T) {
	m := &Message{Sender: "ryan@example.com", Body: "We decided to ship."}
	got := ScoreDecision(m, "ryan@example.com")
	if got != 1.0 {
		t.Fatalf("decision ryan = %v, want 1.0", got)
	}
}

// D3: no marker
func TestScoreDecision_NoMarker(t *testing.T) {
	m := &Message{Sender: "x@y.com", Body: "Hi."}
	got := ScoreDecision(m, "")
	if got != 0.0 {
		t.Fatalf("decision nomarker = %v", got)
	}
}

// E1: expert allowlist hit
func TestScoreExpert_Allow(t *testing.T) {
	m := &Message{Sender: "prof@stanford.edu"}
	got := ScoreExpert(m, []string{"stanford.edu"})
	if got != 1.0 {
		t.Fatalf("expert allow = %v, want 1.0", got)
	}
}

// E2: empty allowlist (graceful degrade)
func TestScoreExpert_Empty(t *testing.T) {
	m := &Message{Sender: "x@y.com"}
	got := ScoreExpert(m, nil)
	if got != 0.0 {
		t.Fatalf("expert empty = %v", got)
	}
}

// U-prefix tests for URL gold (TRIAGE-02 criterion #2)
func TestScoreURLGold_NewArxiv(t *testing.T) {
	m := &Message{ExtractedURLs: []string{"https://arxiv.org/abs/2403.00001"}}
	got := ScoreURLGold(m, nil, nil)
	if got != 1.0 {
		t.Fatalf("urlgold new arxiv = %v, want 1.0", got)
	}
}

func TestScoreURLGold_AlreadyIngested(t *testing.T) {
	s := &SourcesDB{hashes: map[string]bool{}}
	url := "https://arxiv.org/abs/2403.00001"
	s.AddURLForTest(url)
	m := &Message{ExtractedURLs: []string{url}}
	got := ScoreURLGold(m, s, nil)
	if got != 0.0 {
		t.Fatalf("urlgold ingested = %v, want 0.0", got)
	}
}

func TestScoreURLGold_RandomDomain(t *testing.T) {
	m := &Message{ExtractedURLs: []string{"https://random.example.com/x"}}
	got := ScoreURLGold(m, nil, nil)
	if got != 0.0 {
		t.Fatalf("urlgold random = %v, want 0.0", got)
	}
}

func TestScoreURLGold_MultipleMaxWins(t *testing.T) {
	m := &Message{ExtractedURLs: []string{
		"https://random.example.com/x",
		"https://github.com/foo/bar",
	}}
	got := ScoreURLGold(m, nil, nil)
	if got != 1.0 {
		t.Fatalf("urlgold mixed = %v, want 1.0", got)
	}
}

// R1-R4 recurrence buckets
func TestScoreRecurrence_Buckets(t *testing.T) {
	now := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	corpus := []*Message{
		{MessageID: "1", Date: now.AddDate(0, 0, -10), Body: "Vaults are a Vanguard concept."},
		{MessageID: "2", Date: now.AddDate(0, 0, -20), Body: "Vaults discussion continued."},
		{MessageID: "3", Date: now.AddDate(0, 0, -25), Body: "Vaults as a savings vehicle."},
	}
	r := NewRecurrence(corpus, now)
	target := &Message{MessageID: "x", Date: now, Body: "More on Vaults."}
	got := ScoreRecurrence(target, r)
	if got != 0.3 {
		t.Fatalf("recurrence 3x = %v, want 0.3", got)
	}
	// 1 occurrence:
	r2 := NewRecurrence([]*Message{{MessageID: "1", Date: now, Body: "Solo Vaults."}}, now)
	got = ScoreRecurrence(target, r2)
	if got != 0.0 {
		t.Fatalf("recurrence 1x = %v, want 0.0", got)
	}
}
