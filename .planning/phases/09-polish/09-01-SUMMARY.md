---
phase: 09-polish
plan: 01
subsystem: ui
tags: [htmx, templ, solarized, css-chart, email-toggle]

# Dependency graph
requires:
  - phase: 08-thread-view
    provides: thread view with per-message HTMX body-wrapper pattern
  - phase: 07-email-rendering
    provides: messageBodyWrapper endpoint, sandboxed iframe rendering
provides:
  - Text/HTML format toggle on message detail and thread views with URL persistence
  - CSS-only horizontal bar chart on dashboard showing archive volume by month
  - MaxAggregateCount and BarPercent helpers in templates package
affects:
  - future polish phases that touch message body rendering or dashboard layout

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "hx-replace-url with literal canonical URL (not 'true') to control address bar on body-wrapper swaps"
    - "chartMaxCount pre-computed in handler (not O(n^2) in template loop) — passed as int64 param"
    - "CSS bar chart via flex layout + percentage width on .chart-bar-fill divs"

key-files:
  created: []
  modified:
    - internal/web/handlers_messages.go
    - internal/web/handlers_dashboard.go
    - internal/web/templates/message.templ
    - internal/web/templates/message_templ.go
    - internal/web/templates/thread.templ
    - internal/web/templates/thread_templ.go
    - internal/web/templates/dashboard.templ
    - internal/web/templates/dashboard_templ.go
    - internal/web/templates/helpers.go
    - internal/web/static/style.css

key-decisions:
  - "hx-replace-url uses literal canonical message URL (/messages/{id}?format={f}), not hx-replace-url=true, to prevent body-wrapper URL appearing in address bar"
  - "chartMaxCount computed once in handler (templates.MaxAggregateCount) and passed to template — avoids O(n^2) MaxAggregateCount calls in template loop"
  - "Thread expanded card shows email toolbar unconditionally (no BodyText check since MessageSummary lacks body fields) — handler returns graceful no-text message if needed"
  - "hasBothFormats guard in messageBodyWrapper ensures toolbar only renders when both text and HTML exist"
  - "chart Limit=10000 (not 0) because Limit=0 triggers internal default=100"

patterns-established:
  - "Email toolbar pattern: .email-toolbar div with .email-toolbar-btn/.active spans and HTMX hx-get on toggle buttons"
  - "Format toggle uses outerHTML swap on .email-render-wrapper via hx-target=closest .email-render-wrapper"

requirements-completed: [POLISH-01, POLISH-02]

# Metrics
duration: 3min
completed: 2026-03-11
---

# Phase 09 Plan 01: Text/HTML Toggle and Dashboard Bar Chart Summary

**HTMX-driven text/HTML email body toggle with URL persistence and CSS-only horizontal bar chart on dashboard showing archive volume by month**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-11T16:47:29Z
- **Completed:** 2026-03-11T16:51:00Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments

- Text/HTML toggle in message detail: toolbar with Text/HTML buttons, hx-replace-url sets canonical URL (?format=text / ?format=html), toolbar only shown when both formats exist
- Text/HTML toggle in thread view: expanded cards show email toolbar above iframe; collapsed cards get toolbar from messageBodyWrapper endpoint (no template change needed)
- Dashboard bar chart: CSS-only horizontal bars, chronological sort, all months (Limit=10000), clickable rows navigate to aggregate/drilldown?groupBy=time

## Task Commits

Each task was committed atomically:

1. **Task 1: Text/HTML body toggle — handler, message.templ, CSS** - `5ae353ea` (feat)
2. **Task 2: Thread toggle, dashboard bar chart, chart helpers** - `814a9830` (feat)

## Files Created/Modified

- `internal/web/handlers_messages.go` - messageBodyWrapper extended with format param; text view renders html.EscapeString pre block; HTML view includes toolbar when both formats exist; messageDetail passes format to template
- `internal/web/handlers_dashboard.go` - fetches ViewTime+TimeMonth chart data (Limit=10000), pre-computes chartMaxCount, passes both to template
- `internal/web/templates/message.templ` - MessageDetailPage(msg, format string): renders text pre or HTML iframe based on format; toolbar conditionally shown
- `internal/web/templates/thread.templ` - expanded ThreadMessageCard: email-toolbar with Text/HTML buttons above iframe
- `internal/web/templates/dashboard.templ` - DashboardPage signature updated; CSS bar chart inserted between stat-grid and top-lists
- `internal/web/templates/helpers.go` - MaxAggregateCount and BarPercent added (imports query package)
- `internal/web/static/style.css` - .email-toolbar, .email-toolbar-btn, .email-toolbar-sep, .body-text-pre, .archive-chart, .chart-row, .chart-bar-track, .chart-bar-fill, .chart-count

## Decisions Made

- `hx-replace-url` uses literal canonical URL string (`/messages/{id}?format=text`) not `hx-replace-url="true"` — ensures address bar shows the message URL, not the body-wrapper fragment URL
- `chartMaxCount` pre-computed in handler (not in template loop) to avoid O(n^2) calls
- Thread expanded card toolbar shows unconditionally — MessageSummary lacks body format fields, but messageBodyWrapper handles empty BodyText gracefully

## Deviations from Plan

None - plan executed exactly as written.

Pre-existing `go vet` failures in unrelated test files logged to `deferred-items.md` (not caused by these changes, out of scope).

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Text/HTML toggle and dashboard bar chart are complete — POLISH-01 and POLISH-02 requirements fulfilled
- Phase 09 plan 02 (if any) can proceed

---
*Phase: 09-polish*
*Completed: 2026-03-11*
