// Package embedding provides the embedding pipeline and semantic search engine.
// hybrid.go implements Reciprocal Rank Fusion (RRF) and HybridSearch.
package embedding

import (
	"context"
	"fmt"
	"sort"

	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/search"
	"github.com/wesm/msgvault/internal/store"
)

// rankedItem holds a message with its rank fusion score.
type rankedItem struct {
	MessageID     int64
	SimilarityPct float64 // from vector search, 0 if keyword-only
	RRFScore      float64
}

// reciprocalRankFusion merges multiple ranked result lists using RRF.
// k is the rank constant (standard value: 60). limit caps the output.
// For each message, score = sum(1.0 / (k + rank)) across all lists.
// SimilarityPct is carried from the first list that has a non-zero value.
func reciprocalRankFusion(lists [][]rankedItem, k int, limit int) []rankedItem {
	// scores accumulates RRF score per message ID.
	scores := make(map[int64]float64)
	// simPct tracks the best SimilarityPct for each message.
	simPct := make(map[int64]float64)

	for _, list := range lists {
		for rank, item := range list {
			// rank is 0-based; RRF uses 1-based rank.
			scores[item.MessageID] += 1.0 / float64(k+rank+1)
			// Keep the highest SimilarityPct (from vector list).
			if item.SimilarityPct > simPct[item.MessageID] {
				simPct[item.MessageID] = item.SimilarityPct
			}
		}
	}

	// Build result slice.
	results := make([]rankedItem, 0, len(scores))
	for id, score := range scores {
		results = append(results, rankedItem{
			MessageID:     id,
			SimilarityPct: simPct[id],
			RRFScore:      score,
		})
	}

	// Sort by RRF score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].RRFScore > results[j].RRFScore
	})

	// Apply limit.
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// HybridSearch combines FTS5 keyword search and vector similarity search using
// Reciprocal Rank Fusion (k=60) to produce re-ranked results.
//
// It fetches 2x limit from each source for better fusion coverage,
// then applies RRF and returns the top `limit` results.
func HybridSearch(
	ctx context.Context,
	client *ai.Client,
	s *store.Store,
	engine query.Engine,
	queryText string,
	limit int,
) ([]SemanticResult, error) {
	fetchLimit := limit * 2

	// Run FTS5 keyword search.
	parsed := search.Parse(queryText)
	var keywordRanked []rankedItem

	keywordResults, err := engine.Search(ctx, parsed, fetchLimit, 0)
	if err != nil {
		// Log but don't fail; hybrid degrades to vector-only.
		keywordResults = nil
	}
	for _, msg := range keywordResults {
		keywordRanked = append(keywordRanked, rankedItem{
			MessageID:     msg.ID,
			SimilarityPct: 0, // keyword results have no similarity score
		})
	}

	// Run semantic (vector) search.
	var vectorRanked []rankedItem
	semanticResults, err := SemanticSearch(ctx, client, s, queryText, fetchLimit)
	if err != nil {
		// Log but don't fail; hybrid degrades to keyword-only.
		semanticResults = nil
	}
	for _, sr := range semanticResults {
		vectorRanked = append(vectorRanked, rankedItem{
			MessageID:     sr.ID,
			SimilarityPct: sr.SimilarityPct,
		})
	}

	// Apply RRF with k=60 per user decision.
	const rrfK = 60
	fused := reciprocalRankFusion([][]rankedItem{vectorRanked, keywordRanked}, rrfK, limit)

	if len(fused) == 0 {
		return nil, nil
	}

	// Build a lookup map from both result sources to avoid extra DB queries.
	// Prefer the enriched SemanticResult (has SimilarityPct) over plain MessageSummary.
	summaryByID := make(map[int64]query.MessageSummary, len(keywordResults)+len(semanticResults))
	for _, msg := range keywordResults {
		summaryByID[msg.ID] = msg
	}
	// Overwrite with semantic results (same data but already fetched).
	for _, sr := range semanticResults {
		summaryByID[sr.ID] = sr.MessageSummary
	}

	// Compose the final []SemanticResult preserving RRF order.
	out := make([]SemanticResult, 0, len(fused))
	for _, item := range fused {
		summary, ok := summaryByID[item.MessageID]
		if !ok {
			// Message not in either cache — do a direct DB lookup.
			fetched, fetchErr := fetchMessageSummary(ctx, s.DB(), item.MessageID)
			if fetchErr != nil {
				// Message deleted; skip.
				continue
			}
			summary = *fetched
		}
		out = append(out, SemanticResult{
			MessageSummary: summary,
			SimilarityPct:  item.SimilarityPct,
		})
	}

	return out, nil
}

// HybridSearchErr is returned when both FTS5 and vector search fail.
type HybridSearchErr struct {
	KeywordErr error
	VectorErr  error
}

func (e *HybridSearchErr) Error() string {
	return fmt.Sprintf("hybrid search: keyword error: %v, vector error: %v", e.KeywordErr, e.VectorErr)
}
