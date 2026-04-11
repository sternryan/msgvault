---
phase: 13-embeddings-vector-search
plan: 02
subsystem: embedding-web
tags: [rrf, hybrid-search, semantic-search, web-ui, htmx, templ, similarity-badge]
dependency_graph:
  requires:
    - 13-01  # sqlite-vec, embedding pipeline, SemanticSearch, ai.Client
  provides:
    - internal/embedding/hybrid.go (HybridSearch, reciprocalRankFusion)
    - web /search Semantic and Hybrid tabs
    - similarity percentage badges on search results
    - --hybrid flag on msgvault search CLI
  affects:
    - internal/web/handlers.go (aiClient + store fields added)
    - internal/web/server.go (ServerOption/WithAI pattern)
    - internal/web/handlers_search.go (mode routing: keyword/semantic/hybrid)
    - internal/web/templates/search.templ + search_templ.go (new signature + tabs)
    - cmd/msgvault/cmd/web.go (WithAI wired from config)
    - cmd/msgvault/cmd/search.go (--hybrid flag + runHybridSearch)
tech_stack:
  added: []
  patterns:
    - Reciprocal Rank Fusion (k=60) for hybrid result merging
    - ServerOption functional options for backward-compatible NewServer extension
    - mode= query param validated against allowlist (T-13-06 threat mitigation)
    - Graceful AI degradation: tabs hidden when Azure not configured
key_files:
  created:
    - internal/embedding/hybrid.go
    - internal/embedding/hybrid_test.go
  modified:
    - internal/web/handlers.go
    - internal/web/handlers_search.go
    - internal/web/server.go
    - internal/web/templates/search.templ
    - internal/web/templates/search_templ.go
    - internal/web/static/style.css
    - cmd/msgvault/cmd/web.go
    - cmd/msgvault/cmd/search.go
decisions:
  - "RRF k=60 constant per user decision from 13-CONTEXT.md"
  - "Fetch 2x limit from each source before RRF for better fusion coverage"
  - "Graceful degradation: semantic/hybrid mode falls back to keyword when Azure not configured"
  - "ServerOption/WithAI pattern keeps NewServer backward-compatible (variadic opts)"
  - "mode= param validated against allowlist; invalid values fall back to keyword (T-13-06)"
  - "Similarity badge: green >=80%, yellow >=60%, gray below (simple threshold per plan)"
metrics:
  duration: ~45 minutes
  tasks_completed: 2
  tasks_skipped: 1
  files_created: 2
  files_modified: 8
  tests_added: 9
  completed_date: "2026-04-11"
---

# Phase 13 Plan 02: Web UI Semantic Search and Hybrid RRF Re-ranking Summary

**One-liner:** Reciprocal Rank Fusion hybrid search (FTS5+vector, k=60) with HTMX Keyword/Semantic/Hybrid tab switcher and color-coded similarity badges on the web search page.

## What Was Built

### Task 1: Hybrid Search Engine with RRF Re-ranking (TDD)

Created `internal/embedding/hybrid.go`:

- `rankedItem` struct — internal RRF intermediary holding MessageID, SimilarityPct, RRFScore
- `reciprocalRankFusion(lists [][]rankedItem, k int, limit int) []rankedItem` — merges N ranked lists using score = sum(1/(k+rank)), deduplicates by MessageID, carries highest SimilarityPct from vector list, sorts descending
- `HybridSearch(ctx, client, store, engine, queryText, limit)` — fetches 2x limit from FTS5 (engine.Search) and vector (SemanticSearch), builds rankedItem slices, applies RRF with k=60, enriches results from cached summaries (avoiding extra DB queries), returns []SemanticResult

Created `internal/embedding/hybrid_test.go` with 9 table-driven tests:
- TestRRFMath — verifies 1/(60+1) + 1/(60+5) formula with 1e-9 precision
- TestRRFDeduplicate — message in both lists appears once
- TestRRFSortedDescending — results ordered by score descending
- TestRRFLimit — respects limit parameter
- TestRRFk60Constant — rank-1 score exactly 1/61
- TestRRFSimilarityCarried — vector SimilarityPct survives fusion
- TestRRFFallbackFTS5Only — empty vector list still ranks keyword results
- TestRRFFallbackVectorOnly — empty FTS5 list uses vector ranking
- TestRRFBothEmpty — two empty lists produce empty results

Modified `cmd/msgvault/cmd/search.go`:
- Added `--hybrid` bool flag
- `--semantic` and `--hybrid` are mutually exclusive — returns clear error
- `runHybridSearch()` mirrors `runSemanticSearch()` but calls `embedding.HybridSearch`
- Hybrid table output shows similarity % or "---" for keyword-only matches
- Hybrid JSON output includes `similarity_pct` field

### Task 2: Web UI Semantic Search Tab with Similarity Badges

Modified `internal/web/handlers.go`:
- Added `aiClient *ai.Client` and `store *store.Store` fields (both nil when Azure not configured)

Modified `internal/web/server.go`:
- Added `ServerOption` type and `WithAI(client, store)` option
- `NewServer` accepts variadic `...ServerOption` — fully backward-compatible

Modified `internal/web/handlers_search.go`:
- Replaced single handler with `searchPage` (router) + `keywordSearch` + `semanticOrHybridSearch`
- `mode=` param validated against allowlist: `"" | "keyword" | "semantic" | "hybrid"` — invalid values fall back to keyword (T-13-06 mitigation)
- `hasAI` flag derived from `h.aiClient != nil && h.store != nil` — controls tab visibility
- On semantic/hybrid error: falls back to keyword rather than showing error page

Modified `internal/web/templates/search.templ` (and regenerated `search_templ.go`):
- Added `similarityBadgeClass(pct)` and `formatSimilarity(pct)` helper functions
- Updated `SearchPage` signature: added `semanticResults []embedding.SemanticResult` and `hasAI bool`
- Tab bar rendered when `hasAI` is true: Keyword/Semantic/Hybrid buttons with HTMX partial swap
- Semantic/hybrid results branch renders 5-column table with Similarity column
- Similarity badge: `<span class="similarity-badge similarity-{high|mid|low}">87.3%</span>`
- Keyword-only hybrid matches show "---" badge

Modified `internal/web/static/style.css`:
- `.search-tabs` — flexbox tab container with bottom border
- `.search-tab` / `.search-tab.active` — tab styling, active tab highlighted in cyan with bottom border accent
- `.similarity-badge` / `.similarity-{high|mid|low}` — pill badge with color-coded backgrounds (solarized palette)
- `.col-similarity` — right-aligned 6rem column

Modified `cmd/msgvault/cmd/web.go`:
- Wires `web.WithAI(aiClient, store)` when `cfg.AzureOpenAI.Endpoint != ""`
- AI client failure is logged as warning, not fatal — server starts without semantic search

### Task 3: Human Verification Checkpoint

SKIPPED: autonomous mode. Checkpoint type `human-verify` noted for orchestrator.

## Deviations from Plan

None — plan executed exactly as written. The `templ generate` invocation was needed (installed `templ@v0.3.1001` via `go install`) but produced the correct output. The `search_templ.go` was regenerated correctly as confirmed by the new function signatures in the generated file.

## Known Stubs

None — all data flows are wired. Semantic/hybrid search falls back gracefully when Azure is not configured; no stub data is shown to the user.

## Threat Flags

None beyond the plan's threat model. T-13-06 (mode= parameter tampering) is mitigated: handler validates against allowlist and falls back to keyword for unknown values. T-13-07 (DoS via hybrid) is mitigated: limit capped at 2x requested limit per source.

## Self-Check: PASSED

Files exist:
- internal/embedding/hybrid.go: FOUND
- internal/embedding/hybrid_test.go: FOUND
- internal/web/handlers_search.go: FOUND (updated)
- internal/web/templates/search.templ: FOUND (updated)
- internal/web/templates/search_templ.go: FOUND (regenerated)

Commits exist:
- 831671ab (Task 1): FOUND
- c3ae9ff4 (Task 2): FOUND

Tests: 9/9 new RRF tests passing, all existing web/embedding tests passing
Build: clean
Vet: clean
