---
phase: 14-ai-enrichment-ui-integration
plan: 03
subsystem: query-engine, tui, web-ui
tags: [ai-enrichment, tui, web, categories, filter]
dependency_graph:
  requires: [14-01]
  provides: [ENRICH-03]
  affects: [internal/query, internal/tui, internal/web]
tech_stack:
  added: []
  patterns: [ViewType-enum-extension, templ-component-signature, DuckDB-SQLite-fallback]
key_files:
  created: []
  modified:
    - internal/query/models.go
    - internal/query/sqlite.go
    - internal/query/duckdb.go
    - internal/tui/keys.go
    - internal/tui/model.go
    - internal/tui/view.go
    - internal/web/handlers_messages.go
    - internal/web/templates/messages.templ
    - internal/web/templates/messages_templ.go
decisions:
  - DuckDB falls back to SQLite for ViewAICategories because Parquet labels table omits label_type; AI category counts are always small (max 8 rows) so SQLite performance is acceptable
  - Category dropdown uses existing label filter mechanism (?label= param) — no new backend query path needed
metrics:
  duration: ~25min
  completed: 2026-04-11
  tasks_completed: 2
  files_changed: 9
---

# Phase 14 Plan 03: TUI AI Categories View and Web Category Dropdown Summary

TUI ViewAICategories tab added to aggregate cycle (Labels → AI Categories → Time) with drill-down, and web /messages page gains a category dropdown filter backed by store.GetAutoLabels() using the existing label filter mechanism.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | ViewAICategories in query engine and TUI | ae071ecc | models.go, sqlite.go, duckdb.go, keys.go, model.go, view.go |
| 2 | Web messages page category dropdown filter | 2de09b8d | handlers_messages.go, messages.templ, messages_templ.go |

## What Was Built

### Task 1: ViewAICategories in Query Engine and TUI

- Added `ViewAICategories` to the `ViewType` iota in `internal/query/models.go`, immediately before `ViewTypeCount`
- `String()` returns `"AI Categories"`
- `aggDimensionForView` in `sqlite.go` handles `ViewAICategories` with the same JOIN pattern as `ViewLabels` but adds `WHERE l.label_type = 'auto'`
- `buildAggregateSearchParts` in `sqlite.go` treats `ViewAICategories` the same as `ViewLabels` for label search filtering
- DuckDB `Aggregate` and `SubAggregate` methods detect `ViewAICategories` and delegate to `sqliteEngine` — the Parquet `lbl` CTE only exports `id, name` (no `label_type`), so SQLite is required; max 8 rows makes this a non-issue for performance
- `setDrillFilterForView` in `keys.go` maps `ViewAICategories` to `filter.Label` (same field as `ViewLabels`)
- `nextSubGroupView` chains: `ViewLabels → ViewAICategories → ViewTime`
- `viewTypeAbbrev` returns `"AI Category"`, `viewTypePrefix` returns `"AI"`
- `buildMessageFilter` and `drillFilterKey` in `model.go` handle `ViewAICategories` via `filter.Label`

### Task 2: Web Messages Page Category Dropdown

- `messagesList` handler calls `h.store.GetAutoLabels()` non-fatally (empty slice on error or nil store)
- Reads `?label=` query param as `selectedCategory` for dropdown state persistence
- `MessagesPage` template signature extended with `autoLabels []string` and `selectedCategory string`
- Dropdown renders only when `len(autoLabels) > 0` — graceful empty state when enrichment hasn't run
- `<select name="label">` uses existing `?label=` filter mechanism — no new backend query path
- HTMX: `hx-get="/messages"`, `hx-target="#main-content"`, `hx-include="[name]"` preserves other filter params on change
- `messages_templ.go` regenerated via `templ generate -f messages.templ`

## Decisions Made

| Decision | Rationale |
|----------|-----------|
| DuckDB → SQLite fallback for ViewAICategories | Parquet labels table only exports id+name (not label_type). Adding label_type would require Parquet rebuild. AI category counts are tiny — SQLite is the right call. |
| Use existing label filter (?label= param) for category dropdown | Zero new backend code. The parseMessageFilter already handles label filtering via parameterized query. Category names from GetAutoLabels() are just label names with label_type='auto'. |
| Dropdown hides when no autoLabels | Avoids empty dropdown UI artifact before enrichment runs. Graceful degradation. |

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None. The dropdown shows real data from `store.GetAutoLabels()` when enrichment has run; it hides itself when no auto labels exist (graceful degradation, not a stub).

## Self-Check: PASSED

- All 8 modified files verified present on disk
- Commits ae071ecc and 2de09b8d verified in git log
- `go build ./cmd/msgvault/` passes
- `go vet ./...` passes
