# Vector Subsystem Decision

_Written 2026-05-11. See MERGE_REPORT.md §8 for the merge context that created this situation._

---

## TL;DR

**Keep Subsystem B (`internal/vector/`). Delete Subsystem A (`internal/embedding/` + `internal/ai/`).**

Subsystem A is hardcoded to Azure OpenAI — a paid cloud API that requires internet access — making it architecturally incompatible with msgvault's core thesis of an offline, local-first archive. Subsystem B uses any OpenAI-compatible endpoint (including local servers like Ollama or llama.cpp), has roughly 12× more test coverage, a substantially more robust architecture, and was already partially adopted by the merge (the `--hybrid` flag was rerouted to B before this document was written).

---

## Both Subsystems As Found In Code

### Subsystem A — `internal/embedding/` ("AI Archive Intelligence")

**Files:** `internal/embedding/{embed,search,hybrid}.go` plus test files (1,094 lines total), backed by `internal/ai/{client,pipeline,progress,ratelimit}.go` (1,063 lines), `internal/store/{vectors,pipeline}.go`, and DDL in `internal/store/schema.sql`.

**How it works:**

The embedding client (`internal/ai/client.go`) is hardcoded to the Azure OpenAI REST API at `https://<instance>.openai.azure.com/openai/deployments/<name>/embeddings?api-version=2024-10-21`. There is no fallback path and no way to point it at a local server; the URL format is Azure-specific and incompatible with the standard `/v1/embeddings` spec.

`RunEmbedPipeline()` in `embed.go` drives a checkpoint-based batch loop: it reads messages from `messages` ordered by ID, calls `ai.BatchRunner`, and stores 1536-dim float32 vectors into the `vec_messages` virtual table via `store.InsertEmbeddings()`. Checkpoint state lives in `pipeline_runs` / `pipeline_checkpoints` (managed by `internal/store/pipeline.go`). The embed text is `"Subject: {subject}\n{snippet}"` — subject and snippet only, not full body.

`SemanticSearch()` in `search.go` embeds the query string (one API call per search), then runs a KNN query against `vec_messages` via `store.SearchSemantic()`, enriches results with a JOIN on participants, and returns `[]SemanticResult` with `SimilarityPct` in [0, 100].

`HybridSearch()` in `hybrid.go` fetches 2× limit from both FTS5 and the semantic path, then applies RRF with k=60. Degrades gracefully if either source fails.

**CLI surface (post-merge):**
- `--semantic` flag in `search.go` → `runSemanticSearch()` → this subsystem.
- `--hybrid` flag was _deleted_ from subsystem A during the merge and rerouted to subsystem B's `runHybridSearch()` in `search_vector.go`.
- `msgvault build-embeddings` was replaced wholesale by upstream's version; it now targets subsystem B's pipeline, not subsystem A's `pipeline_runs` table.

**Schema tables in `msgvault.db`:**
```
vec_messages     — vec0 virtual table (message_id PK, embedding FLOAT[1536])
pipeline_runs    — batch job lifecycle (id, type, status, started_at, ...)
pipeline_checkpoints — per-batch progress (pipeline_run_id, last_message_id, ...)
```

`InitSchema()` unconditionally calls `InitVectorTable()`, which means `vec_messages` is attempted on every database open. If sqlite-vec is not loaded, this call fails silently (or errors out depending on driver behavior).

**Test coverage:** 519 lines across `embed_test.go`, `search_test.go`, `hybrid_test.go`. Tests cover RRF math, text formatting, similarity percentage conversion, graceful degradation. No integration tests against a real database or real embedding API.

---

### Subsystem B — `internal/vector/` (Upstream PR #277)

**Files:** `internal/vector/` root (backend.go, config.go, errors.go, generations.go, stats.go, env.go), `internal/vector/sqlitevec/` (backend.go 1,079 lines, fused.go 539 lines, migrate.go, ext.go, ext_stub.go, schema.sql), `internal/vector/hybrid/` (engine.go 195 lines, filter.go 218 lines, rrf.go 86 lines), `internal/vector/embed/` (client.go 241 lines, worker.go 427 lines, queue.go 171 lines, preprocess.go 80 lines, enqueue.go). CLI in `cmd/msgvault/cmd/search_vector.go` (322 lines, build-tagged `sqlite_vec`).

**How it works:**

The embedding client (`internal/vector/embed/client.go`) calls a generic `/v1/embeddings` endpoint. The `Endpoint` config field is any base URL — it can be `http://localhost:11434/v1` (Ollama), a llama.cpp server, or a hosted provider. The API key field is optional. This is the critical architectural difference from subsystem A.

Embeddings are stored in a _separate_ `vectors.db` file (not `msgvault.db`), managed by the sqlitevec backend. The schema uses a generation lifecycle: each embed run creates a `building` generation, seeds `pending_embeddings`, processes them into the `embeddings` table and the `vectors_vec_dN` vec0 virtual table, then atomically promotes to `active` (retiring the previous generation). Crash recovery: pending rows use 16-char random claim tokens with a stale-reclaim threshold (default 10 min).

Hybrid search runs as a single CTE in SQLite: `vectors.db` is ATTACHed to the main DB connection, then a single query executes BM25 (FTS5) and ANN (vec0) in two CTEs, full-outer-joins them, and applies RRF. Subject boost is applied post-query via a batch lookup. Over-fetching (2× KPerSignal) absorbs clustered deletions.

The `Backend` interface allows future non-sqlite-vec implementations. `FusingBackend` is an optional capability interface for single-query hybrid search; the pure-Go `rrf.go` is a fallback if the backend can't fuse in SQL.

Structured filters (`internal/vector/hybrid/filter.go`) parse `internal/search.Query` operators (`from:`, `to:`, `subject:`, `label:`, date bounds, `has:attachment`) into AND-of-OR groups resolved against participant and label IDs — mirroring the FTS5 path exactly.

**Current build status:** the `sqlite_vec` build tag is **disabled by default** in the Makefile (`BUILD_TAGS := fts5`). The reason is documented in the Makefile: go-sqlcipher v4.4.2 ships SQLite 3.37.x; sqlite-vec requires `sqlite3_vtab_in*` APIs added in SQLite 3.38 (Jan 2022). The sqlitevec backend is therefore dormant in all default builds. `ext_stub.go` returns `ErrNotBuilt` when the tag is absent.

**CLI surface (post-merge):**
- `--mode hybrid` and `--explain` in `search.go` → `runHybridSearch()` in `search_vector.go` → this subsystem (build-tagged, so inactive by default).
- `--hybrid` flag was rerouted here from subsystem A during the merge.
- `msgvault build-embeddings` targets this subsystem's pipeline (taken wholesale from upstream during merge).

**Schema tables in `vectors.db` (separate file):**
```
index_generations   — generation lifecycle (id, model, dimension, fingerprint, state, ...)
embeddings          — per-message metadata (gen_id, message_id, embedded_at, truncated, ...)
pending_embeddings  — work queue with claim tokens for crash safety
embed_runs          — per-run stats
vectors_vec_dN      — vec0 virtual table per dimension N (e.g. vectors_vec_d1536)
```

**Test coverage:** ~6,300 lines across 15 test files, including integration-style tests against in-memory SQLite, fused-search CTE tests, worker crash-recovery tests, filter resolution tests, RRF math tests, config validation tests. Substantially more exercised surface area than subsystem A.

---

## Comparison Matrix

| Criterion | Subsystem A (`internal/embedding/`) | Subsystem B (`internal/vector/`) |
|---|---|---|
| **Offline/local-first alignment** | Poor. Azure OpenAI is hardcoded — every search query requires an internet-accessible paid API. Cannot be pointed at a local model without rewriting the client. | Good. Endpoint is configurable to any OpenAI-compatible server, including local (Ollama, llama.cpp). Search itself is fully local once the index is built. |
| **Maintainability** | Moderate. Clean code, but simpler architecture (no generation lifecycle, no crash-safe queue, no deletion handling). 519 test lines. The embed CLI was replaced by upstream's during the merge — the CLI surface is already partially broken. | Strong. Backend interface abstracts storage. Generation lifecycle, crash-safe pending queue, structured filters, deletion handling, separate DB. ~6,300 test lines, ~12× more than A. |
| **Real-world data tested** | Unknown for integration tests. Unit tests use mock vectors and fixed floats. No evidence of tests against a real DB or real API. The `--hybrid` flag (the primary entry point) was deleted from A's path during the merge. | Subsystem B has backend integration tests using real in-memory SQLite + synthetic messages. No evidence of tests against production Gmail data in either subsystem. |
| **Performance characteristics** | KNN via vec0 in the main `msgvault.db`. Embed text is subject + snippet only (not full body), keeping vectors small. No over-fetch for deletions — stale deleted rows can pollute results. RRF is pure Go across two separate queries. | KNN via vec0 in a separate `vectors.db`. Over-fetch (2×) absorbs deletions. Single-CTE hybrid path ATTACHes vectors.db and executes BM25 + ANN in one round trip. Subject boost applied post-query. Deletion is cleanly scoped per generation. Comparable latency for the embed step; single-query hybrid should be faster than A's two-query approach. |
| **Dependency footprint** | Requires `azure_openai` config section with valid endpoint and API key. Hard dependency on Azure; no local fallback. | Requires `vector.embeddings` config section. Endpoint can be local. API key optional. Additional Go module: `github.com/asg017/sqlite-vec-go-bindings v0.1.6`. |
| **Current functional state** | Partially functional: `--semantic` path works if sqlite-vec is loaded and Azure credentials are configured. `--hybrid` was rerouted to B. `build-embeddings` was replaced by B's CLI. | Dormant: `sqlite_vec` build tag disabled due to sqlcipher compatibility. No users can currently exercise this path without a custom build. |

---

## Recommendation

**Keep Subsystem B. Delete Subsystem A.**

The decisive factor is alignment with the project's stated purpose. msgvault exists to be an _offline_ archive of Gmail data. A vector search feature that requires a live internet connection to a paid Azure API for every query — including at index time and at search time — is fundamentally misaligned with that goal. This isn't a minor inconvenience; it means semantic search is unavailable on a plane, on a home NAS with no egress configured, or after the Azure subscription expires. Every search query leaks email text to an external service, which conflicts with the privacy rationale for self-hosting an email archive in the first place.

Subsystem B resolves this. Its embedding client is a generic HTTP client that accepts any base URL. Pointed at a local Ollama instance or llama.cpp server, the entire pipeline — index build and query — runs offline. The endpoint can also be a hosted provider for users who want that, but it is not mandatory.

Beyond the philosophical alignment, Subsystem B is the technically stronger implementation by a large margin: ~12× more test coverage, generation lifecycle management for safe re-indexing, a crash-safe pending queue with claim tokens, structured query filters that mirror FTS5 semantics, deletion-aware over-fetching, and a single-CTE hybrid query that avoids the round-trip overhead of A's two-query approach.

The merge itself already made a partial bet on B: `--hybrid` was rerouted to B's `runHybridSearch()`, and `build-embeddings` was replaced by B's CLI. Completing the migration finishes what the merge started.

**The blocker is solvable.** The `sqlite_vec` build tag is disabled because go-sqlcipher v4.4.2 ships SQLite 3.37.x, which is missing `sqlite3_vtab_in*` APIs that sqlite-vec requires. The fix is to upgrade go-sqlcipher (or use a binding that ships a newer SQLite amalgamation). This is a dependency version issue, not an architectural flaw in B.

**The "already shipped" concern for A is manageable.** Users who ran subsystem A's pipeline have data in `vec_messages`. Those vectors do not need to be migrated — they cannot be; the two subsystems use different tables, different schemas, and potentially different embedding models. The migration is: drop the old tables, re-run `build-embeddings` with the new pipeline. Vectors are recomputable; the emails are the durable asset.

---

## Migration Plan (Keeping Subsystem B)

### Phase 1: Unblock the build tag

1. Upgrade `github.com/mutecomm/go-sqlcipher/v4` to a release that ships SQLite ≥ 3.38, or switch to a sqlcipher binding that vendors a current SQLite amalgamation.
2. Re-enable `sqlite_vec` in `Makefile`: change `BUILD_TAGS := fts5` to `BUILD_TAGS := fts5,sqlite_vec`.
3. Verify `make build` links cleanly with no duplicate symbol errors.
4. Verify `go test -tags sqlite_vec ./internal/vector/...` passes.

### Phase 2: Wire subsystem B fully into CLI

5. In `cmd/msgvault/cmd/search.go`: remove `--semantic` flag and `runSemanticSearch()` call. The `--mode vector` and `--mode hybrid` flags (routed to `search_vector.go`) replace it.
6. Confirm `msgvault build-embeddings` documentation matches the new pipeline's config keys (`[vector.embeddings]` endpoint, model, dimension).
7. Update `README.md` "Vector Search" section to describe `[vector]` config, local endpoint options, and the generation lifecycle.

### Phase 3: Schema cleanup (user-facing migration)

8. Write a `msgvault migrate` sub-step (or add to `init-db`) that drops `vec_messages` and `pipeline_runs` / `pipeline_checkpoints` from `msgvault.db` for existing users, with a warning message: "Existing AI Archive Intelligence vectors will be removed; re-run `build-embeddings` to rebuild the index."
9. Remove the `InitVectorTable()` call from `store.go::InitSchema()` — subsystem B manages its own `vectors.db` outside the main DB.
10. Remove the `vec_messages` creation comment and `pipeline_runs` DDL from `internal/store/schema.sql`.

---

## Deletion Plan for Subsystem A

This is a concrete checklist. Do not execute until the Phase 1 unblock is complete and the new build tag is tested.

### Files to delete

```
internal/embedding/embed.go
internal/embedding/embed_test.go
internal/embedding/search.go
internal/embedding/search_test.go
internal/embedding/hybrid.go
internal/embedding/hybrid_test.go
internal/ai/client.go
internal/ai/client_test.go
internal/ai/pipeline.go
internal/ai/pipeline_test.go
internal/ai/progress.go
internal/ai/ratelimit.go
internal/ai/ratelimit_test.go
internal/store/vectors.go
internal/store/vectors_test.go
internal/store/pipeline.go
internal/store/pipeline_test.go
```

### Code paths to remove

- `store.go::InitSchema()`: remove the `s.InitVectorTable()` call (line ~570–573).
- `store.go`: remove any methods or imports that reference `vectors.go` / `pipeline.go` symbols.
- `cmd/msgvault/cmd/search.go`: remove `--semantic` flag declaration, `searchSemantic` variable, `runSemanticSearch()` function, and the `if searchSemantic` dispatch branch. Remove import of `internal/embedding`.
- `cmd/msgvault/cmd/search.go`: remove import of `internal/ai` (used only by `runSemanticSearch`).
- Remove `[azure_openai]` config section from `internal/config/config.go` _only if_ no other subsystem references it (the triage pipeline and other AI features may still use it — verify before deleting).

### Schema changes

- Drop `vec_messages`, `pipeline_runs`, `pipeline_checkpoints` tables from `internal/store/schema.sql`.
- Write a schema migration (version bump in the migrations list in `store.go`) that executes:
  ```sql
  DROP TABLE IF EXISTS pipeline_checkpoints;
  DROP TABLE IF EXISTS pipeline_runs;
  DROP TABLE IF EXISTS vec_messages;
  ```
  Note: `vec_messages` is a virtual table; `DROP TABLE` on a vec0 virtual table should work but test this against the go-sqlcipher version in use.

### Documentation / config updates

- `CLAUDE.md` "Implementation Status" section: remove `pipeline_runs`, `vec_messages` from schema table list.
- `README.md`: replace any references to `--semantic`, `build-embeddings` (old semantics), `[azure_openai]` for vector search.
- `internal/config/config.go`: remove `AzureOpenAIConfig` only if no remaining callers exist. The triage pipeline (`internal/triage/`) may share this config — audit before deleting.

### go.mod cleanup (deferred)

- After deleting all callers, check if `internal/ai` was the only user of any dependency unique to it. Run `go mod tidy` and inspect the diff before committing.

---

## Open Questions / Things That Could Not Be Determined Without Running Real Data

1. **What SQLite version ships with the target go-sqlcipher upgrade?** The path to re-enabling `sqlite_vec` depends on finding a go-sqlcipher release (or fork) that ships SQLite ≥ 3.38. This needs a concrete dependency version, not just "upgrade it."

2. **Does any real user currently have data in `vec_messages`?** The presence of `pipeline_runs` rows is the signal. If no one has run the old embed pipeline against production data, the schema migration is trivially safe.

3. **Does the triage pipeline (`internal/triage/`) use `internal/ai/client.go`?** CLAUDE.md states the triage hot path must not call LLM/encoder endpoints (D-04), but it's possible the `internal/ai/` package is used for something other than embedding (e.g., chat completions for digest generation). Before deleting `internal/ai/`, audit all callers: `grep -r 'internal/ai' --include='*.go'`.

4. **Subsystem B's embedding quality vs. A's for Gmail data.** Both subsystems are untested against production Gmail archives in the test suite. A uses `text-embedding-3-small` (Azure) at 1536 dims; B is model-agnostic. Whether a locally-served model gives comparable recall on email subject+body text is an empirical question. Recommend running both against a 1K-message sample before committing to the migration.

5. **Is the `--semantic` flag in active use anywhere downstream?** If forge or another consumer calls `msgvault search --semantic` as part of a pipeline, removing the flag is a breaking change for that consumer. Audit downstream callers before removing.

6. **Preprocess quality for emails.** Subsystem B's `preprocess.go` strips reply quotes, signature blocks, and prepends subject — designed for threaded email content. Subsystem A embeds `subject + snippet` only. The full-body approach of B is likely better for recall but needs empirical validation on real archives.
