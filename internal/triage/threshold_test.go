package triage

import (
	"bytes"
	"testing"
)

func mkCands(n int, baseScore float64) []Candidate {
	out := make([]Candidate, n)
	for i := 0; i < n; i++ {
		out[i] = Candidate{MessageID: "id" + string(rune('a'+i%26)), Score: baseScore, Date: "2026-04-25T00:00:00Z"}
	}
	return out
}

// T1: 30 above + 5 below threshold → exactly 25 in output, all >= 0.55
func TestSelectAndSort_TopN(t *testing.T) {
	cs := make([]Candidate, 0, 35)
	for i := 0; i < 30; i++ {
		cs = append(cs, Candidate{MessageID: "good" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Score: 0.6, Date: "2026-04-25T00:00:00Z"})
	}
	for i := 0; i < 5; i++ {
		cs = append(cs, Candidate{MessageID: "bad" + string(rune('a'+i)), Score: 0.4, Date: "2026-04-25T00:00:00Z"})
	}
	out := SelectAndSort(cs, 0.55, 25)
	if len(out) != 25 {
		t.Fatalf("got %d, want 25", len(out))
	}
	for _, c := range out {
		if c.Score < 0.55 {
			t.Fatalf("got score %v < 0.55", c.Score)
		}
	}
}

// T2: sort tuple Score DESC, Date DESC, MessageID ASC
func TestSelectAndSort_Tuple(t *testing.T) {
	cs := []Candidate{
		{MessageID: "b", Score: 0.7, Date: "2026-04-22T00:00:00Z"},
		{MessageID: "a", Score: 0.7, Date: "2026-04-22T00:00:00Z"},
		{MessageID: "c", Score: 0.7, Date: "2026-04-23T00:00:00Z"},
		{MessageID: "d", Score: 0.8, Date: "2026-04-20T00:00:00Z"},
	}
	out := SelectAndSort(cs, 0.0, 10)
	want := []string{"d", "c", "a", "b"}
	for i, c := range out {
		if c.MessageID != want[i] {
			t.Fatalf("pos %d: got %q want %q", i, c.MessageID, want[i])
		}
	}
}

// T3: deterministic across two runs
func TestSelectAndSort_Deterministic(t *testing.T) {
	cs := []Candidate{
		{MessageID: "z", Score: 0.6, Date: "2026-04-25T00:00:00Z"},
		{MessageID: "a", Score: 0.7, Date: "2026-04-25T00:00:00Z"},
	}
	o1 := SelectAndSort(cs, 0.55, 25)
	o2 := SelectAndSort(cs, 0.55, 25)
	var b1, b2 bytes.Buffer
	_ = Emit(&b1, o1)
	_ = Emit(&b2, o2)
	if b1.String() != b2.String() {
		t.Fatalf("non-deterministic: %s vs %s", b1.String(), b2.String())
	}
}

// T4: --threshold raises bar
func TestSelectAndSort_HigherThreshold(t *testing.T) {
	cs := []Candidate{{Score: 0.6}, {Score: 0.8}}
	out := SelectAndSort(cs, 0.7, 10)
	if len(out) != 1 || out[0].Score != 0.8 {
		t.Fatalf("got %v", out)
	}
}

// T5: --max truncates
func TestSelectAndSort_MaxN(t *testing.T) {
	cs := mkCands(20, 0.9)
	out := SelectAndSort(cs, 0.55, 10)
	if len(out) != 10 {
		t.Fatalf("got %d, want 10", len(out))
	}
}
