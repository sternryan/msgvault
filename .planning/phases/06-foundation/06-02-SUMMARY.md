---
phase: 06-foundation
plan: 02
subsystem: ui
tags: [templ, htmx, chi, go, dashboard, pagination, messages]

requires:
  - phase: 06-01
    provides: "chi router, renderPage helper, params.go, layout.templ, Solarized Dark CSS, stub handlers"

provides:
  - Dashboard page with GetTotalStats stat cards (messages, accounts, size, attachments)
  - Top 5 senders and top 5 domains tables with hx-get navigation on rows
  - Messages list page with paginated 50-row view, sortable columns (date/size/subject)
  - Shared Pagination component (Showing X-Y of N, Prev/Next with HTMX partial updates)
  - Shared SortHeader component (clickable th with sort arrow, URL state)
  - Message detail page with headers (From/To/Cc/Bcc/Date/Size/Labels), plain text body, attachment list
  - handlers_dashboard.go and handlers_messages.go implementing real query.Engine calls

affects: [06-03, 06-04, 06-05]

tech-stack:
  added: []
  patterns:
    - "hx-get on <tr> rows for HTMX navigation — full page with hx-select='#main-content' extract"
    - "buildMessagesBaseURL helper strips offset/limit before passing baseURL to Pagination"
    - "GetTotalStats used for pagination total count (fast, avoids extra COUNT query)"
    - "paginationURL and sortURL helpers are pure Go functions in components.templ (not templ funcs)"

key-files:
  created:
    - internal/web/handlers_dashboard.go
    - internal/web/handlers_messages.go
    - internal/web/templates/dashboard.templ
    - internal/web/templates/dashboard_templ.go
    - internal/web/templates/messages.templ
    - internal/web/templates/messages_templ.go
    - internal/web/templates/message.templ
    - internal/web/templates/message_templ.go
    - internal/web/templates/components.templ
    - internal/web/templates/components_templ.go
  modified:
    - internal/web/handlers.go

key-decisions:
  - "GetTotalStats used for messages pagination count — avoids adding SearchFastCount dependency for unfiltered list"
  - "hx-get on <tr> rows instead of onclick/SafeScript — templ SafeScript is for ComponentScript not string, rows use HTMX natively"
  - "limit locked to 50 in messagesList handler, not exposed to URL param"
  - "paginationURL/sortURL implemented as pure Go functions (not templ components) so they can be called from template expressions"

patterns-established:
  - "All new page handlers split into dedicated files (handlers_dashboard.go, handlers_messages.go) — not monolithic handlers.go"
  - "Pagination component takes baseURL (path+params without offset/limit) and handles URL construction internally"
  - "MessageDetail shows plain text; HTML body deferred to Phase 7 bluemonday+iframe approach"
  - "formatAddresses, truncateStr, displayFrom helpers live in templates package as plain Go functions"

requirements-completed: [PARITY-01, PARITY-03, PARITY-05]

duration: 4min
completed: 2026-03-11
---

# Phase 06 Plan 02: Dashboard + Messages + Detail Pages Summary

**Dashboard with real stats/top-lists, paginated sortable messages list, and message detail view — all using HTMX partial updates and Solarized Dark theme**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-11T02:18:05Z
- **Completed:** 2026-03-11T02:22:22Z
- **Tasks:** 2
- **Files modified:** 11 (10 created, 1 modified)

## Accomplishments
- Dashboard page: 4 stat cards (messages/accounts/size/attachments) plus top-5 senders and top-5 domains tables, empty state for fresh archives
- Messages list: 50-row paginated view with HTMX sort controls (date/size/subject), "Showing X-Y of N messages" pagination bar with Prev/Next
- Message detail: full headers (From/To/Cc/Bcc/Date/Size/Labels), plain text body with pre-wrap, attachment list with download links
- Shared Pagination and SortHeader templ components usable by Plans 03-04 as well

## Task Commits

Each task was committed atomically:

1. **Task 1: Dashboard page with stats and top lists** - `0a4985b` (feat)
2. **Task 2: Messages list and Message detail pages with pagination** - `d823bf6` (feat)

**Plan metadata:** (docs commit pending)

## Files Created/Modified
- `internal/web/handlers_dashboard.go` - Dashboard handler: GetTotalStats + Aggregate (senders/domains) with sourceId propagation
- `internal/web/handlers_messages.go` - messagesList (50/page, GetTotalStats for count) + messageDetail (chi URLParam, GetMessage, 404 on nil)
- `internal/web/handlers.go` - Removed dashboard/messagesList/messageDetail stubs
- `internal/web/templates/dashboard.templ` - Stat cards grid + top-lists two-column layout
- `internal/web/templates/dashboard_templ.go` - Generated
- `internal/web/templates/messages.templ` - Sortable table + Pagination component usage
- `internal/web/templates/messages_templ.go` - Generated
- `internal/web/templates/message.templ` - Detail headers dl/dt/dd, pre-wrap body, attachment list
- `internal/web/templates/message_templ.go` - Generated
- `internal/web/templates/components.templ` - Pagination + SortHeader reusable components
- `internal/web/templates/components_templ.go` - Generated

## Decisions Made
- Used `hx-get` on `<tr>` rows for click-to-navigate instead of `onclick`. `templ.SafeScript` requires `templ.ComponentScript` type but returns a string — using HTMX attributes directly on rows is the correct templ-native approach.
- Used `GetTotalStats` for pagination total count in messages list. It's already called for the dashboard and is faster than adding a separate COUNT query for the unfiltered case.
- Locked messages page limit to 50 in the handler (not a URL param), matching the plan's "50 rows/page" locked decision.
- Pure Go helper functions (`paginationURL`, `sortURL`, `minInt64`) in components.templ package — these can be called from template expressions without being templ components.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Replaced templ.SafeScript onclick with hx-get on rows**
- **Found during:** Task 1 (Dashboard template compilation)
- **Issue:** Plan suggested `onclick={ templ.SafeScript(...) }` pattern, but `templ.SafeScript` returns a `string`, not a `templ.ComponentScript` — build failed with type mismatch errors
- **Fix:** Used `hx-get`, `hx-select="#main-content"`, `hx-target`, `hx-swap`, `hx-replace-url` on `<tr>` elements directly — HTMX handles click-to-navigate natively without JavaScript
- **Files modified:** internal/web/templates/dashboard.templ
- **Verification:** `go build -tags fts5 ./cmd/msgvault` passes
- **Committed in:** 0a4985b (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug/type error)
**Impact on plan:** The HTMX approach is actually superior — no JavaScript onclick needed, consistent with the project's HTMX-first pattern from layout.templ.

## Issues Encountered
- Pre-existing vet failures in `internal/export`, `internal/mbox`, `cmd/msgvault/cmd` — confirmed pre-date this plan (per 06-01 SUMMARY). Out-of-scope, not fixed.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Dashboard, Messages list, and Message detail pages fully implemented with real data
- Pagination and SortHeader components ready for reuse in Plans 03-04 (Aggregate, Search)
- All 9 integration tests still pass
- Plan 03 (Aggregate view) and Plan 04 (Search + Deletions) can proceed immediately

---
*Phase: 06-foundation*
*Completed: 2026-03-11*
