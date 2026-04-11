# Phase 13: Embeddings & Vector Search - Context

**Gathered:** 2026-04-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can find semantically related emails using natural language queries, beyond exact keyword matching. Embed all messages via Azure OpenAI text-embedding-3-small, store in sqlite-vec, and expose semantic search via CLI and web UI with hybrid FTS5+vector re-ranking.

Requirements: EMBED-01, EMBED-02, EMBED-03, EMBED-04, EMBED-05

</domain>

<decisions>
## Implementation Decisions

### Embedding Strategy
- Embed subject + snippet (~200 chars) per message — keeps token cost low, subject carries most search intent
- Use text-embedding-3-small with default 1536 dimensions — no truncation
- Store vectors in sqlite-vec virtual table in main msgvault.db — single file, same connection
- Batch size 100 messages per API call — balances latency and checkpoint granularity

### Search UX
- Explicit mode switch: `--semantic` flag on CLI, toggle button on web UI
- Top 50 results by default, `--limit N` flag — matches existing search patterns
- Show similarity score as percentage (0-100%) next to each result
- Hybrid re-ranking via Reciprocal Rank Fusion (RRF) — simple, proven, no tuning

### Web UI Integration
- Add "Semantic" tab to existing search page — extends current HTMX search pattern
- Same message list layout as keyword search with similarity % badge — consistent UX
- HTMX indicator with "Searching semantically..." text

### Claude's Discretion
- sqlite-vec Go binding choice (asg017/sqlite-vec or cgo wrapper)
- Embedding text preprocessing (trimming, encoding normalization)
- RRF constant k value (standard is 60)
- Incremental embedding (only embed new messages on subsequent runs)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/ai/client.go` — Client.Embedding() method (from Phase 12)
- `internal/ai/pipeline.go` — BatchRunner with checkpoints (from Phase 12)
- `internal/ai/progress.go` — ProgressReporter (from Phase 12)
- `internal/search/` — existing FTS5 search engine
- `internal/web/` — Templ + HTMX templates, search page exists
- `internal/query/engine.go` — DuckDB query engine

### Established Patterns
- HTMX search with partial swaps in web UI
- Cobra CLI commands with flags in cmd/msgvault/cmd/
- Store struct for all DB operations
- sqlite-vec requires CGO (already required for go-sqlite3)

### Integration Points
- New `msgvault embed` CLI command (uses BatchRunner from Phase 12)
- New `msgvault search --semantic` flag on existing search command
- New sqlite-vec virtual table (vec_messages) in schema.sql
- Web UI search template needs semantic tab/toggle
- Hybrid search engine wrapping both FTS5 and vector engines

</code_context>

<specifics>
## Specific Ideas

- Use the Phase 12 BatchRunner directly — ProcessFunc calls Client.Embedding()
- sqlite-vec vec0 virtual table with message_id INTEGER PRIMARY KEY, embedding FLOAT[1536]
- RRF formula: score = sum(1/(k+rank)) across result lists, k=60
- Embed command should be idempotent — skip messages already in vec_messages

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
