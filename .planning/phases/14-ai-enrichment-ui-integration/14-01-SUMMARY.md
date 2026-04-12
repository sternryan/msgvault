---
phase: 14-ai-enrichment-ui-integration
plan: "01"
subsystem: enrichment
tags: [ai, enrichment, pipeline, sqlite, cli, gpt4o-mini]
dependency_graph:
  requires:
    - internal/ai (BatchRunner, Client.ChatCompletion — Phase 12)
    - internal/store (Store, labels/message_labels — existing)
  provides:
    - internal/enrichment (RunEnrichPipeline, CountUnenriched)
    - internal/store/enrichment.go (GetOrCreateAutoLabel, InsertLifeEvent, InsertEntity, GetLifeEvents, GetEntities, GetAutoLabels, GetLifeEventsForExport, GetEntityMessageIDs)
    - cmd/msgvault/cmd/enrich.go (msgvault enrich CLI)
  affects:
    - internal/store/schema.sql (new life_events, entities tables; partial unique index)
    - Plans 14-02, 14-03 (UI integration, export-timeline — consume enrichment data)
tech_stack:
  added: []
  patterns:
    - BatchRunner ProcessFunc (Phase 12 pattern)
    - NOT EXISTS semi-join for uncategorized query (CLAUDE.md SQL guidelines)
    - Parameterized queries for all store writes (threat model T-14-02)
    - Category allowlist validation post-LLM-response (threat model T-14-01)
    - Markdown fence stripping + regex fallback for LLM JSON parsing
key_files:
  created:
    - internal/store/enrichment.go
    - internal/store/enrichment_test.go
    - internal/enrichment/prompt.go
    - internal/enrichment/pipeline.go
    - internal/enrichment/pipeline_test.go
    - cmd/msgvault/cmd/enrich.go
  modified:
    - internal/store/schema.sql (appended life_events, entities, partial unique index)
decisions:
  - "normalizeEntityValue lowercases all types and strips company suffixes (Inc/LLC/Corp) — makes entity deduplication tractable in v1.3"
  - "ProcessFunc is 1:1 message-per-ChatCompletion call (not batched) per research pitfall 3 — GPT-4o-mini structured output works better 1:1"
  - "Default batch size 20 (not 100) — each batch makes 20 sequential API calls; 100 would be 10s/batch at 10 QPS"
  - "runEnrichDryRun takes *store.Store directly — avoids double-open pattern from initial draft"
  - "buildEnrichProcessFuncTestable injection pattern (chatFn, isCategorized, writeResults) mirrors embedding package's testable helpers"
metrics:
  duration_minutes: 45
  completed_date: "2026-04-12"
  tasks_completed: 2
  files_created: 6
  files_modified: 1
  lines_added: 1297
  commits: 2
---

# Phase 14 Plan 01: Schema, Store Methods, Enrichment Pipeline, and CLI Summary

**One-liner:** GPT-4o-mini enrichment pipeline via BatchRunner that categorizes messages into 8 auto labels, extracts life events and entities, with full store layer and `msgvault enrich` CLI.

## What Was Built

### Task 1: Schema + Store (commit `58d12f8d`)

Added two new tables to `internal/store/schema.sql`:
- `life_events` (message_id FK, event_date TEXT, event_type, description) with 3 indexes
- `entities` (message_id FK, entity_type, value, normalized_value, context) with 4 indexes
- Partial unique index `idx_labels_auto_name ON labels(name) WHERE source_id IS NULL AND label_type = 'auto'` — prevents duplicate auto labels since SQLite NULL != NULL in UNIQUE constraints

New `internal/store/enrichment.go` with 8 Store methods:
- `GetOrCreateAutoLabel(name) (int64, error)` — idempotent, race-safe
- `InsertLifeEvent(messageID, date, type, description) error`
- `InsertEntity(messageID, type, value, normalizedValue, context) error` — empty strings stored as NULL
- `GetAutoLabels() ([]string, error)` — sorted distinct category names for UI dropdowns
- `GetLifeEvents(eventType, limit, offset) ([]LifeEventRow, int64, error)` — optional type filter
- `GetLifeEventsForExport(eventType) ([]LifeEventExportRow, error)` — JOINs messages for source_message_id
- `GetEntities(entityType, searchQuery, limit, offset) ([]EntityRow, int64, error)` — type + search filters
- `GetEntityMessageIDs(entityValue) ([]int64, error)` — drill-down from entity to source messages

10 tests in `enrichment_test.go`, all passing.

### Task 2: Pipeline + CLI (commit `030fb727`)

`internal/enrichment/prompt.go`:
- `EnrichResult`, `LifeEvent`, `Entity` structs with JSON tags
- `buildEnrichRequest(subject, snippet)` — system+user role separation (T-14-04 mitigated)
- `parseEnrichResponse(content)` — direct unmarshal → markdown fence strip → regex fallback
- `validateCategory(cat)` — 8-value allowlist, defaults to "personal" (T-14-01 mitigated)
- `normalizeEntityValue(type, value)` — company suffix stripping, lowercase

`internal/enrichment/pipeline.go`:
- `createEnrichQueryFunc(s)` — NOT EXISTS semi-join (CLAUDE.md SQL guideline)
- `buildEnrichProcessFunc(client, s, deployment, logger)` — 1:1 message/ChatCompletion, idempotency check, graceful per-message failure (increment Failed, continue)
- `buildEnrichProcessFuncTestable(chatFn, isCategorized, writeResults, ...)` — injectable for testing
- `writeEnrichResults(s, messageID, result)` — fan-out to labels, life_events, entities
- `RunEnrichPipeline(ctx, client, s, logger, batchSize)` — BatchRunner with PipelineType="categorize"
- `CountUnenriched(s)` — count for dry-run

`cmd/msgvault/cmd/enrich.go`:
- `msgvault enrich` Cobra command
- `--batch-size int` (default 20), `--deployment string` (default "chat"), `--dry-run bool`
- Follows `embed.go` pattern exactly: config check → openStore → InitSchema → [dry-run | pipeline]

8 enrichment tests pass.

## Verification Results

```
go test ./internal/store/... ./internal/enrichment/... -count=1
ok  github.com/wesm/msgvault/internal/store      9.792s
ok  github.com/wesm/msgvault/internal/enrichment 3.862s

go build ./cmd/msgvault/   → SUCCESS
go vet ./...               → CLEAN

./msgvault enrich --help   → shows --batch-size, --deployment, --dry-run flags
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Double-open pattern in runEnrichDryRun**
- **Found during:** Task 2 implementation review
- **Issue:** Initial draft of `runEnrichDryRun` opened a second store connection despite the store already being open and schema-initialized in `runEnrich`. This caused unnecessary connection overhead and an awkward `interface{}` parameter type.
- **Fix:** Changed `runEnrichDryRun` to accept `*store.Store` directly, reusing the already-opened connection.
- **Files modified:** `cmd/msgvault/cmd/enrich.go`
- **Commit:** `030fb727`

## Known Stubs

None — all store methods are fully implemented and wired. No placeholder data or hardcoded empty values flow to any UI consumer.

## Threat Flags

No new threat surface beyond what was in the plan's threat model. All T-14-01 through T-14-04 mitigations are implemented:
- T-14-01: `validateCategory` allowlist enforced post-response
- T-14-02: All store methods use parameterized queries
- T-14-03: Only subject+snippet sent (accepted)
- T-14-04: System/user role separation in ChatRequest

## Self-Check: PASSED

All 6 created files exist. Both task commits (58d12f8d, 030fb727) confirmed in git log.
