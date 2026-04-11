package embedding

import (
	"testing"
)

// rankedItemForTest is a copy of the internal type for test comparison.
// We use the package-internal reciprocalRankFusion directly since tests are in the same package.

// TestRRFMath verifies the RRF scoring formula with known inputs.
func TestRRFMath(t *testing.T) {
	// k=60. Message ranked #1 in list1 and #5 in list2:
	// score = 1/(60+1) + 1/(60+5) = 1/61 + 1/65
	want := 1.0/61.0 + 1.0/65.0

	list1 := []rankedItem{{MessageID: 10, SimilarityPct: 90.0}}
	list2 := []rankedItem{
		{MessageID: 99},
		{MessageID: 98},
		{MessageID: 97},
		{MessageID: 96},
		{MessageID: 10, SimilarityPct: 0}, // same message, rank 5
	}

	results := reciprocalRankFusion([][]rankedItem{list1, list2}, 60, 10)

	// Find message ID 10
	var found *rankedItem
	for i := range results {
		if results[i].MessageID == 10 {
			found = &results[i]
			break
		}
	}

	if found == nil {
		t.Fatal("expected message ID 10 in RRF results")
	}

	if abs(found.RRFScore-want) > 1e-9 {
		t.Errorf("RRF score for msg 10: got %.10f, want %.10f", found.RRFScore, want)
	}
}

// TestRRFDeduplicate verifies that a message appearing in both lists is merged.
func TestRRFDeduplicate(t *testing.T) {
	list1 := []rankedItem{
		{MessageID: 1},
		{MessageID: 2},
	}
	list2 := []rankedItem{
		{MessageID: 2}, // duplicate
		{MessageID: 3},
	}

	results := reciprocalRankFusion([][]rankedItem{list1, list2}, 60, 10)

	// Should have 3 unique messages, not 4
	if len(results) != 3 {
		t.Errorf("expected 3 unique results, got %d", len(results))
	}

	seen := map[int64]int{}
	for _, r := range results {
		seen[r.MessageID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("message %d appeared %d times, want 1", id, count)
		}
	}
}

// TestRRFSortedDescending verifies results are sorted by RRF score descending.
func TestRRFSortedDescending(t *testing.T) {
	// Message 1 appears in both lists (higher combined score)
	// Message 2 appears only in list1
	// Message 3 appears only in list2
	list1 := []rankedItem{{MessageID: 1}, {MessageID: 2}}
	list2 := []rankedItem{{MessageID: 1}, {MessageID: 3}}

	results := reciprocalRankFusion([][]rankedItem{list1, list2}, 60, 10)

	// Message 1 should be first (appears in both lists)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].MessageID != 1 {
		t.Errorf("expected message 1 first (highest RRF), got %d", results[0].MessageID)
	}

	// Verify descending order
	for i := 1; i < len(results); i++ {
		if results[i].RRFScore > results[i-1].RRFScore {
			t.Errorf("results not sorted: results[%d].RRFScore=%f > results[%d].RRFScore=%f",
				i, results[i].RRFScore, i-1, results[i-1].RRFScore)
		}
	}
}

// TestRRFLimit verifies that the limit parameter is respected.
func TestRRFLimit(t *testing.T) {
	var list1 []rankedItem
	for i := int64(1); i <= 20; i++ {
		list1 = append(list1, rankedItem{MessageID: i})
	}

	results := reciprocalRankFusion([][]rankedItem{list1}, 60, 5)

	if len(results) != 5 {
		t.Errorf("expected 5 results (limit), got %d", len(results))
	}
}

// TestRRFk60Constant verifies the k=60 constant is used in scoring.
func TestRRFk60Constant(t *testing.T) {
	// With k=60, rank 1 score = 1/61
	list1 := []rankedItem{{MessageID: 1}}

	results := reciprocalRankFusion([][]rankedItem{list1}, 60, 10)

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	want := 1.0 / 61.0
	if abs(results[0].RRFScore-want) > 1e-9 {
		t.Errorf("RRF score for rank-1 item: got %.10f, want %.10f", results[0].RRFScore, want)
	}
}

// TestRRFSimilarityCarried verifies that SimilarityPct from the vector list is carried through.
func TestRRFSimilarityCarried(t *testing.T) {
	// Message 5 appears in both lists; vector list has SimilarityPct=87.3
	vectorList := []rankedItem{{MessageID: 5, SimilarityPct: 87.3}}
	keywordList := []rankedItem{{MessageID: 5, SimilarityPct: 0}}

	results := reciprocalRankFusion([][]rankedItem{vectorList, keywordList}, 60, 10)

	var found *rankedItem
	for i := range results {
		if results[i].MessageID == 5 {
			found = &results[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected message 5 in results")
	}
	if found.SimilarityPct != 87.3 {
		t.Errorf("expected SimilarityPct=87.3, got %f", found.SimilarityPct)
	}
}

// TestRRFFallbackFTS5Only verifies that when vector list is empty, RRF still ranks FTS5 results.
func TestRRFFallbackFTS5Only(t *testing.T) {
	keywordList := []rankedItem{
		{MessageID: 1},
		{MessageID: 2},
		{MessageID: 3},
	}
	vectorList := []rankedItem{} // empty

	results := reciprocalRankFusion([][]rankedItem{keywordList, vectorList}, 60, 10)

	if len(results) != 3 {
		t.Errorf("expected 3 results from FTS5-only fallback, got %d", len(results))
	}
	if results[0].MessageID != 1 {
		t.Errorf("expected message 1 first (rank 1 in keyword list), got %d", results[0].MessageID)
	}
}

// TestRRFFallbackVectorOnly verifies that when FTS5 list is empty, RRF uses vector ranking.
func TestRRFFallbackVectorOnly(t *testing.T) {
	keywordList := []rankedItem{} // empty
	vectorList := []rankedItem{
		{MessageID: 10, SimilarityPct: 95.0},
		{MessageID: 20, SimilarityPct: 80.0},
	}

	results := reciprocalRankFusion([][]rankedItem{keywordList, vectorList}, 60, 10)

	if len(results) != 2 {
		t.Errorf("expected 2 results from vector-only fallback, got %d", len(results))
	}
	if results[0].MessageID != 10 {
		t.Errorf("expected message 10 first (rank 1 in vector list), got %d", results[0].MessageID)
	}
	if results[0].SimilarityPct != 95.0 {
		t.Errorf("expected SimilarityPct=95.0, got %f", results[0].SimilarityPct)
	}
}

// TestRRFBothEmpty verifies that two empty lists produce empty results.
func TestRRFBothEmpty(t *testing.T) {
	results := reciprocalRankFusion([][]rankedItem{{}, {}}, 60, 10)
	if len(results) != 0 {
		t.Errorf("expected empty results for empty inputs, got %d", len(results))
	}
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
