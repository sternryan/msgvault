---
phase: 06-foundation
plan: 03
subsystem: ui
tags: [templ, htmx, go, aggregate, search, duckdb, fts5, drill-down, breadcrumbs]

requires:
  - phase: 06-01
    provides: "chi router, renderPage helper, params.go, layout.templ, Solarized Dark CSS, stub handlers"
  - phase: 06-02
    provides: "Pagination and SortHeader shared components, handlers_dashboard.go, handlers_messages.go patterns"

provides:
  - Aggregate page with 7 view type tabs (Senders, Sender Names, Recipients, Recipient Names, Domains, Labels, Time)
  - Filter bar with 500ms debounced input on aggregate page
  - Sortable aggregate columns (Name, Count, Size, Att. Size) using SortHeader component
  - Drill-down from aggregate row to messages list with breadcrumb navigation
  - Sub-view tabs in drill-down pass ?filterView={viewType} to trigger SubAggregate()
  - Search page with debounced live search input (500ms), autofocus, htmx-indicator
  - DuckDB fast-path (SearchFast) with FTS5 Search() fallback when fast returns empty
  - Search results as paginated messages table with mode badge (metadata vs full-text)
  - BreadcrumbItem type in templates package for template access

affects: [06-04, 06-05]

tech-stack:
  added: []
  patterns:
    - "applyKeyToFilter: dispatches ViewType enum to MessageFilter field in a single switch — reuse this for any future drilldown logic"
    - "Two-tier search: SearchFast first, fall back to Search only when fast returns zero and TextTerms exist"
    - "Offset-aware pagination for drilldown: handler passes offset explicitly to template, template passes to Pagination component"

key-files:
  created:
    - internal/web/handlers_aggregate.go
    - internal/web/handlers_search.go
    - internal/web/templates/aggregate.templ
    - internal/web/templates/aggregate_templ.go
    - internal/web/templates/search.templ
    - internal/web/templates/search_templ.go
  modified:
    - internal/web/handlers.go
    - internal/web/templates/helpers.go

key-decisions:
  - "BreadcrumbItem defined in templates/helpers.go (templates package) so templ functions can use it as a typed parameter"
  - "Sub-view tabs in drill-down always include ?filterView={viewType} in URL — aggregateDrilldown handler branches on this param to call SubAggregate vs ListMessages"
  - "SearchFast count (SearchFastCount) used for metadata path; deep FTS5 path has no count endpoint so estimate from len(results)"
  - "Drilldown message pagination estimate: if len(results) == limit, set total = offset+limit+1 to show Next button"

patterns-established:
  - "All new page handlers go in dedicated handler_* files (handlers_aggregate.go, handlers_search.go) — not monolithic handlers.go"
  - "URL state for bookmarkability: groupBy, filterKey, filterView, sortField, sortDir, sourceId all preserved in URLs"
  - "viewTabs slice in aggregate.templ drives both top-level tabs and sub-view tabs to keep them in sync"

requirements-completed: [PARITY-02, PARITY-04]

duration: 6min
completed: 2026-03-11
---

# Phase 06 Plan 03: Aggregate + Search Pages Summary

**7-dimension aggregate browser with drill-down/sub-view tabs and two-tier DuckDB+FTS5 debounced search — both HTMX-powered with URL-bookmarkable state**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-11T02:25:36Z
- **Completed:** 2026-03-11T02:31:36Z
- **Tasks:** 2
- **Files modified:** 8 (6 created, 2 modified)

## Accomplishments
- Aggregate page: 7 view type tabs, 500ms debounced filter bar, sortable columns (Name/Count/Size/Att.Size), clickable rows drill into messages
- Drill-down page: breadcrumbs (Aggregate > ViewType > Key), sub-view tabs for all other dimensions with ?filterView param triggering SubAggregate()
- Search page: autofocused debounced input (500ms), htmx-indicator, DuckDB fast path first with FTS5 fallback, mode badge showing which path was used, paginated results

## Task Commits

Each task was committed atomically:

1. **Task 1: Aggregate page with 7 view types, drill-down, breadcrumbs, sub-view tabs** - `8b28937` (feat)
2. **Task 2: Search page with debounced live search and two-tier fallback** - `243ff47` (feat)

**Plan metadata:** (docs commit pending)

## Files Created/Modified
- `internal/web/handlers_aggregate.go` - aggregate and aggregateDrilldown handlers; applyKeyToFilter, buildAggregateBaseURL, drilldownURL helpers
- `internal/web/handlers_search.go` - searchPage handler: SearchFast + Search() fallback; SearchFastCount for pagination
- `internal/web/handlers.go` - Removed aggregate/aggregateDrilldown/searchPage stubs
- `internal/web/templates/helpers.go` - Added BreadcrumbItem type (moved to templates package for templ access)
- `internal/web/templates/aggregate.templ` - AggregatePage (tabs, filter, table) + AggregateDrilldownPage (breadcrumbs, sub-view tabs, messages/sub-agg content)
- `internal/web/templates/aggregate_templ.go` - Generated from aggregate.templ
- `internal/web/templates/search.templ` - SearchPage (input, indicator, help text, results table, Pagination)
- `internal/web/templates/search_templ.go` - Generated from search.templ

## Decisions Made
- `BreadcrumbItem` defined in `templates/helpers.go` rather than `handlers_aggregate.go`. Templ components reference types by package, so the type must live in the same package as the templ file that uses it.
- Sub-view tabs always carry `?filterView={param}` in their URLs. The `aggregateDrilldown` handler branches on this param: if set, calls `SubAggregate()` and renders a sub-aggregate table; if unset, calls `ListMessages()` and renders a messages table.
- `SearchFastCount` used for metadata search pagination count. For deep FTS5 fallback, there is no separate count method — total is estimated from `len(results)` with a sentinel `+1` if the page is full.
- Drilldown message total is estimated conservatively: `offset + len(results)` when last page, `offset + limit + 1` when page is full (shows Next button).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] BreadcrumbItem moved from web package to templates package**
- **Found during:** Task 1 (aggregate.templ generation)
- **Issue:** `aggregate_templ.go` references `BreadcrumbItem` as `templates.BreadcrumbItem` since it lives in the template — but the type was defined in `handlers_aggregate.go` (web package), causing `undefined: BreadcrumbItem` build error
- **Fix:** Moved `BreadcrumbItem` struct definition to `internal/web/templates/helpers.go`; updated handler to use `templates.BreadcrumbItem`
- **Files modified:** internal/web/templates/helpers.go, internal/web/handlers_aggregate.go
- **Verification:** `go build -tags fts5 ./internal/web/...` passes
- **Committed in:** 8b28937 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking — type locality issue with templ generated code)
**Impact on plan:** Required fix for build correctness. The templates package is the natural home for types used as template parameters anyway.

## Issues Encountered
- Pre-existing modified files (`cmd/msgvault/cmd/stage_deletion.go`, `internal/gvoice/`, `internal/mbox/`) visible in git status — confirmed pre-existing, not staged or modified by this plan.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Aggregate and Search pages fully functional with real data
- All 9 integration tests still pass (TestAggregate, TestSearch both pass)
- Plan 04 (Deletions page) can proceed immediately
- BreadcrumbItem type available in templates package for any future plan that needs breadcrumbs

---
*Phase: 06-foundation*
*Completed: 2026-03-11*
