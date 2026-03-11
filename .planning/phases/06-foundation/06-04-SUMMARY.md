---
phase: 06-foundation
plan: 04
subsystem: ui
tags: [templ, htmx, go, deletions, oob-swap, keyboard-shortcuts, javascript]

requires:
  - phase: 06-01
    provides: "chi router, renderPage helper, params.go, layout.templ, Solarized Dark CSS, stub handlers"
  - phase: 06-02
    provides: "Pagination and SortHeader shared components, handler patterns"
  - phase: 06-03
    provides: "aggregate drill-down page, AggregateDrilldownPage template, applyKeyToFilter helper"

provides:
  - Deletions page listing all manifests (pending/in-progress/completed/failed) sorted by CreatedAt desc
  - handlers_deletions.go with deletionsPage, stageDeletion (form POST + OOB badge), cancelDeletion handlers
  - DeletionsPage, DeletionBadgeOOB, StageResult templ components in deletions.templ
  - stagingForm templ component in aggregate.templ for one-click deletion staging from drill-down
  - Full keyboard shortcuts in keys.js: j/k nav, Enter open, Esc back, Tab cycle views, s/r sort, t time, a account, / search, ? help, q no-op
  - Help overlay created dynamically by ? key with full shortcut table
  - Account filter propagation via JS URL manipulation (preserves all query params)
  - layout.templ account-filter select uses pure JS (no HTMX attributes)

affects: [06-05]

tech-stack:
  added: []
  patterns:
    - "Two root-level response elements for HTMX OOB swap: primary swap target + DeletionBadgeOOB span"
    - "stagingForm templ component uses switch on groupBy to select correct hidden input field name"
    - "Account filter via setupAccountFilter(): reads URL, sets/deletes sourceId param, htmx.ajax to new URL"
    - "deletionStatusClass pure Go function in templates package returns CSS class string for status badge"

key-files:
  created:
    - internal/web/handlers_deletions.go
    - internal/web/templates/deletions.templ
    - internal/web/templates/deletions_templ.go
  modified:
    - internal/web/handlers.go
    - internal/web/static/keys.js
    - internal/web/static/style.css
    - internal/web/templates/aggregate.templ
    - internal/web/templates/aggregate_templ.go
    - internal/web/templates/layout.templ
    - internal/web/templates/layout_templ.go

key-decisions:
  - "Two root-level response elements for stageDeletion: templ renders StageResult then DeletionBadgeOOB sequentially to response writer — HTMX OOB requires root-level siblings"
  - "layout.templ always renders deletion-badge span (empty when count=0) so OOB swap can clear it"
  - "Account filter uses JS URL manipulation not HTMX hx-get — preserves all existing query params (groupBy, sortField, q, etc.) that hx-get cannot handle without duplicating all params"
  - "stagingForm as separate templ component using switch statement — avoids anonymous function in attribute value (templ parser limitation)"

patterns-established:
  - "OOB badge pattern: always render target element in layout (even empty), OOB component sends hx-swap-oob=true sibling in same response"
  - "Pure JS filter changes: setupAccountFilter() binds once (dataset.bound guard), re-binds after htmx:afterSettle for HTMX page swaps"

requirements-completed: [PARITY-06, PARITY-07, PARITY-08]

duration: 5min
completed: 2026-03-11
---

# Phase 06 Plan 04: Deletions + Keyboard Shortcuts + Account Filter Summary

**HTMX OOB badge-updating deletion staging, full Vim-style keyboard shortcuts with q no-op, and JS URL manipulation for account filter propagation across all views**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-11T02:35:05Z
- **Completed:** 2026-03-11T02:40:17Z
- **Tasks:** 2
- **Files modified:** 10 (3 created, 7 modified)

## Accomplishments
- Deletions page: lists all manifests by status (pending/in-progress/completed/failed), cancel button on pending rows with hx-delete + confirmation dialog
- Staging flow: "Stage for Deletion" form on aggregate drill-down sends form POST, resolves Gmail IDs via GetGmailIDsByFilter, creates manifest, responds with dual root-level elements (StageResult + DeletionBadgeOOB OOB swap)
- Full keyboard shortcuts: j/k navigate rows with visual focus highlight, Enter HTMX-navigates, Esc goes back (or closes help), Tab cycles view tabs, s/r sort, t time, a/? focus helpers, q is no-op (preventDefault), / goes to search
- Account filter: removed HTMX attributes from select, JS setupAccountFilter() reads URL and replaces/adds sourceId while preserving all other params

## Task Commits

Each task was committed atomically:

1. **Task 1: Deletions page, staging handler with OOB badge, cancel handler** - `3aa0181` (feat)
2. **Task 2: Full keyboard shortcuts with q no-op, and account filter via JavaScript URL manipulation** - `aa4bafc` (feat)

**Plan metadata:** (docs commit pending)

## Files Created/Modified
- `internal/web/handlers_deletions.go` - deletionsPage (all statuses sorted), stageDeletion (form POST → IDs → manifest → dual OOB response), cancelDeletion (remove pending + OOB badge)
- `internal/web/templates/deletions.templ` - DeletionsPage table, DeletionBadgeOOB (hx-swap-oob), StageResult banner
- `internal/web/templates/deletions_templ.go` - Generated from deletions.templ
- `internal/web/handlers.go` - Removed stub deletion handlers
- `internal/web/static/keys.js` - Full keyboard handler with all shortcuts, setupAccountFilter(), help overlay
- `internal/web/static/style.css` - Added staging-bar, status-badge, stage-success/error, kbd, help-dismiss styles
- `internal/web/templates/aggregate.templ` - Added stagingForm component + stagingFilterField helper; stagingForm called in drilldown messages view
- `internal/web/templates/aggregate_templ.go` - Generated
- `internal/web/templates/layout.templ` - Always render deletion-badge span; remove HTMX attrs from account-filter select; add else branch for empty badge
- `internal/web/templates/layout_templ.go` - Generated

## Decisions Made
- Two root-level response elements for `stageDeletion`: templ renders `StageResult` then `DeletionBadgeOOB` sequentially to the response writer. HTMX OOB swap requires root-level sibling elements — nesting OOB inside another element causes silent failure.
- `layout.templ` always renders the `deletion-badge` span (even when empty) so the OOB swap can clear it when count drops to 0. Previously only rendered when count > 0.
- Account filter uses JS URL manipulation (not HTMX `hx-get`): `setupAccountFilter()` reads `window.location.href`, modifies `sourceId` param, calls `htmx.ajax` to reload `#main-content`. This is the only correct approach — `hx-get` with `hx-include` cannot preserve all existing URL params across all pages without duplicating them on every page.
- `stagingForm` implemented as a separate `templ` component with a `switch` statement on `groupBy` to select the correct `<input type="hidden" name="...">` — templ parser cannot handle anonymous function expressions inside attribute values.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed templ parse error: `for` keyword in text content**
- **Found during:** Task 1 (deletions.templ generation)
- **Issue:** `Staged { count } { Pluralize(...) } for deletion:` — templ parser interprets standalone `for` as a loop statement, causing parse error at line 96 col 84
- **Fix:** Changed to `{ fmt.Sprintf("Staged %d %s for deletion: %s", ...) }` — single expression avoids the keyword parsing issue
- **Files modified:** internal/web/templates/deletions.templ
- **Verification:** `templ generate` succeeds, build passes
- **Committed in:** 3aa0181 (Task 1 commit)

**2. [Rule 1 - Bug] Fixed templ parse error: anonymous function in attribute value**
- **Found during:** Task 1 (aggregate.templ stagingForm)
- **Issue:** `name={ func() string { n, _ := stagingFilterField(...); return n }() }` — templ parser does not support anonymous function expressions in attribute values
- **Fix:** Extracted `stagingForm` as a separate `templ` component using a `switch` statement on `groupBy` to select the correct hidden input field name; extracted `stagingFilterField` as a pure Go helper
- **Files modified:** internal/web/templates/aggregate.templ
- **Verification:** `templ generate` succeeds, build passes
- **Committed in:** 3aa0181 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (2 templ parser limitations)
**Impact on plan:** Both fixes are required for build correctness. The `stagingForm` component approach is actually cleaner than inline anonymous functions. No scope creep.

## Issues Encountered
- Pre-existing modified files (`cmd/msgvault/cmd/stage_deletion.go`, `internal/gvoice/`, `internal/mbox/`) visible in git status — confirmed pre-existing (per previous plan SUMMARYs). Not staged, out-of-scope.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All PARITY requirements (PARITY-01 through PARITY-08) now complete
- All 9 integration tests pass
- Phase 6 foundation complete: chi+templ+HTMX server with full interactive web UI
- Phase 7 (HTML email rendering, iframe, bluemonday sanitization) can proceed immediately
- Deletion staging and management fully functional; execution (Phase 7/8) will pick up from here

## Self-Check: PASSED

- handlers_deletions.go: FOUND
- deletions.templ: FOUND
- deletions_templ.go: FOUND
- 06-04-SUMMARY.md: FOUND
- commit 3aa0181: FOUND
- commit aa4bafc: FOUND

---
*Phase: 06-foundation*
*Completed: 2026-03-11*
