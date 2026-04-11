---
phase: 13-embeddings-vector-search
plan: 01
subsystem: embedding
tags: [sqlite-vec, embeddings, vector-search, azure-openai, semantic-search]
dependency_graph:
  requires:
    - 12-pipeline-infrastructure  # BatchRunner, ai.Client
  provides:
    - vec_messages virtual table (sqlite-vec)
    - internal/embedding package
    - msgvault embed CLI command
    - msgvault search --semantic flag
  affects:
    - internal/store/store.go (InitSchema now calls InitVectorTable)
    - cmd/msgvault/cmd/search.go (--semantic flag added)
tech_stack:
  added:
    - github.com/asg017/sqlite-vec-go-bindings v0.1.6 (CGO sqlite-vec extension)
    - internal/sqlvec (local CGO wrapper for go-sqlcipher compatibility)
  patterns:
    - DELETE+INSERT idiom for idempotent vec0 updates (INSERT OR REPLACE not supported)
    - k= constraint syntax for KNN queries (required on SQLite < 3.38)
    - Local CGO package compiling sqlite-vec against go-sqlcipher's SQLite 3.33.0
key_files:
  created:
    - internal/sqlvec/vec.go
    - internal/sqlvec/sqlite-vec.c
    - internal/sqlvec/sqlite-vec.h
    - internal/sqlvec/sqlite3.h
    - internal/store/vectors.go
    - internal/store/vectors_test.go
    - internal/embedding/embed.go
    - internal/embedding/embed_test.go
    - internal/embedding/search.go
    - internal/embedding/search_test.go
    - cmd/msgvault/cmd/embed.go
  modified:
    - internal/store/schema.sql (vector search comment block)
    - internal/store/store.go (InitSchema calls InitVectorTable)
    - cmd/msgvault/cmd/search.go (--semantic flag, runSemanticSearch)
    - go.mod / go.sum (sqlite-vec-go-bindings added)
decisions:
  - "Local sqlvec CGO package to compile sqlite-vec against go-sqlcipher's SQLite 3.33.0 — avoids sqlite3_vtab_in symbol mismatch that occurs when using upstream cgo package against macOS system SQLite headers"
  - "k= constraint syntax for KNN queries instead of LIMIT — required on SQLite < 3.38 where LIMIT is not pushed into vec0 xBestIndex"
  - "DELETE+INSERT idiom for idempotent vec0 updates — INSERT OR REPLACE triggers UNIQUE constraint on vec0 virtual tables in sqlite-vec v0.1.6"
  - "textEmbeddingDeployment const maps to 'text-embedding' logical name — resolves to 'text-embedding-3-small' via config.toml deployments map"
metrics:
  duration: ~45 minutes
  tasks_completed: 2
  files_created: 12
  files_modified: 4
  tests_added: 18
  completed_date: "2026-04-11"
---

# Phase 13 Plan 01: sqlite-vec Integration, Embedding Pipeline, and CLI Semantic Search

**One-liner:** sqlite-vec KNN vector search integrated against go-sqlcipher's SQLite 3.33.0 with Azure OpenAI text-embedding pipeline and `msgvault embed`/`search --semantic` CLI.

## What Was Built

### Task 1: sqlite-vec schema and Store vector methods

Created `internal/sqlvec/` — a local CGO package that compiles `sqlite-vec.c` against `go-sqlcipher`'s bundled `sqlite3.h` (SQLite 3.33.0). This resolves a critical linker conflict: the upstream `sqlite-vec-go-bindings/cgo` package compiled against macOS system headers (SQLite 3.43+) which reference `sqlite3_vtab_in`, a symbol not exported by go-sqlcipher's older amalgamation.

Created `internal/store/vectors.go` with:
- `VectorEntry` and `SemanticResult` types
- `InitVectorTable()` — creates `vec_messages USING vec0(message_id INTEGER PRIMARY KEY, embedding FLOAT[1536])`
- `InsertEmbeddings([]VectorEntry)` — DELETE+INSERT idiom for idempotency
- `SearchSemantic(queryVec []float32, limit int)` — KNN via `WHERE embedding MATCH ? AND k = ?`; converts cosine distance [0,2] to similarity [0,1]
- `HasEmbedding(messageID int64)` — skip check for incremental embedding
- `EmbeddingCount() int64` — count stored vectors

`InitSchema()` now calls `InitVectorTable()` so the vec_messages table is created automatically on first use.

**9 tests passing** covering: create, idempotency, insert/retrieve, search ordering, limit, similarity range, HasEmbedding, EmbeddingCount, insert idempotency.

### Task 2: Embed pipeline, semantic search engine, and CLI commands

Created `internal/embedding/embed.go`:
- `BuildEmbedText(subject, snippet string)` — formats `"Subject: {subject}\n{snippet}"` with empty-part trimming
- `createQueryFunc(s *store.Store)` — queries messages WHERE NOT EXISTS in vec_messages, ordered by ID ASC
- `createProcessFunc(...)` — builds embed texts, calls `client.Embedding(ctx, "text-embedding", texts)`, maps responses back to message IDs, stores via `InsertEmbeddings`, returns `BatchResult` with token counts and cost ($0.02/1M tokens)
- `RunEmbedPipeline(ctx, client, store, logger)` — wires BatchRunner with `PipelineType="embedding"`, `BatchSize=100`, `CheckpointEvery=10`
- `CountUnembedded(s)` — for dry-run cost estimation

Created `internal/embedding/search.go`:
- `SemanticResult` struct embedding `query.MessageSummary` + `SimilarityPct float64` (0-100)
- `SemanticSearch(ctx, client, store, queryText, limit)` — embeds query, calls `SearchSemantic`, enriches results with full message metadata from SQLite JOIN
- `fetchMessageSummary()` — direct PK lookup on messages JOIN conversations LEFT JOIN participants

Created `cmd/msgvault/cmd/embed.go`:
- `msgvault embed [--batch-size N] [--dry-run]`
- `--dry-run` shows unembedded count, estimated tokens, estimated cost without API calls

Modified `cmd/msgvault/cmd/search.go`:
- `--semantic` flag routes to `runSemanticSearch()`
- Validates no structured operators (from:/to:/subject:/etc.) with clear error message
- Table output adds SIMILARITY column showing `"87.3%"`
- JSON output adds `"similarity_pct"` field
- Checks for empty vec_messages before calling Azure OpenAI (clear "run embed first" error)

**9 tests passing** covering: BuildEmbedText variants, ProcessFunc token counts, ProcessFunc skip-existing, QueryFunc ordering, SemanticSearch metadata enrichment, SimilarityPct range.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] sqlite-vec CGO linker conflict with go-sqlcipher**
- **Found during:** Task 1 GREEN phase
- **Issue:** `sqlite-vec-go-bindings/cgo` imports system SQLite headers (3.43+ on macOS) which include `sqlite3_vtab_in`. When linked, `go-sqlcipher`'s bundled SQLite 3.33.0 doesn't export `sqlite3_vtab_in`, causing undefined symbol linker error.
- **Fix:** Created `internal/sqlvec/` local CGO package that copies `sqlite-vec.c/h` and `sqlite3.h` from go-sqlcipher's module directory. sqlite-vec.c conditionally uses `sqlite3_vtab_in` only when `SQLITE_VERSION_NUMBER >= 3038000`; with 3.33.0, that code path is compiled out entirely.
- **Files modified:** `internal/sqlvec/` (new package), `internal/store/vectors.go` (import changed from upstream cgo to local sqlvec)
- **Commits:** 6046cb06

**2. [Rule 1 - Bug] KNN query required `k=` constraint, not just LIMIT**
- **Found during:** Task 1 test run (GREEN phase)
- **Issue:** Error: "A LIMIT or 'k = ?' constraint is required on vec0 knn queries." — on SQLite 3.33.0, LIMIT is not pushed into the vec0 virtual table's `xBestIndex`; the `k=` WHERE constraint is required.
- **Fix:** Changed `SearchSemantic` to use `WHERE embedding MATCH ? AND k = ?` instead of `LIMIT ?`.
- **Files modified:** `internal/store/vectors.go`
- **Commits:** 6046cb06

**3. [Rule 1 - Bug] INSERT OR REPLACE fails on vec0 virtual tables**
- **Found during:** Task 1 `TestVectorInsertIdempotent` (GREEN phase)
- **Issue:** `INSERT OR REPLACE INTO vec_messages` triggers UNIQUE constraint error on sqlite-vec v0.1.6 — the vec0 virtual table doesn't implement the conflict resolution path for OR REPLACE.
- **Fix:** Changed `InsertEmbeddings` to DELETE+INSERT within a transaction.
- **Files modified:** `internal/store/vectors.go`
- **Commits:** 6046cb06

**4. [Rule 1 - Bug] Test helper used RETURNING clause (SQLite 3.35+ only)**
- **Found during:** Task 2 search test run
- **Issue:** `insertTestMessage` in `search_test.go` used `INSERT ... RETURNING id` which is SQLite 3.35+, but go-sqlcipher has 3.33.0.
- **Fix:** Changed to `INSERT` + `SELECT last_insert_rowid()` pattern.
- **Files modified:** `internal/embedding/search_test.go`
- **Commits:** 41118633

## Known Stubs

None — all data flows are wired. The embed command requires real Azure OpenAI config but the binary compiles and the help text explains configuration requirements.

## Threat Flags

None beyond the plan's threat model. All new network calls (embed + search) go through the existing `ai.Client` which has rate limiting and key management from Phase 12.

## Self-Check: PASSED

Files exist:
- internal/sqlvec/vec.go: FOUND
- internal/store/vectors.go: FOUND
- internal/embedding/embed.go: FOUND
- internal/embedding/search.go: FOUND
- cmd/msgvault/cmd/embed.go: FOUND

Commits exist:
- 6046cb06 (Task 1): FOUND
- 41118633 (Task 2): FOUND

Tests: 18/18 passing
Build: clean
Vet: clean
