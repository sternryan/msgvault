---
phase: 10-integration-cleanup
plan: 01
subsystem: testing, ui
tags: [htmx, templ, go-test, dom-ids]

# Dependency graph
requires:
  - phase: 09-polish
    provides: unified email-toolbar replacing email-images-banner, hx-target using closest .email-render-wrapper
  - phase: 08-thread-view
    provides: per-message wrapper IDs (email-body-wrapper-{msgID}) in eager ThreadMessageCard

provides:
  - Passing TestMessageBodyWrapperEndpoint with Phase 9 toolbar assertions
  - wrapperID query param in messageBodyWrapper handler (parameterized wrapper ID)
  - Thread lazy-load hx-get URLs pass ?wrapperID=email-body-wrapper-{msgID}
  - TestBodyWrapperWithWrapperIDParam test validating scoped wrapper IDs

affects: [thread-view, message-detail, web-handlers]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "wrapperID query param: handler reads optional wrapperID, defaults to email-body-wrapper, used in all 6 output paths"
    - "Thread lazy-load scoped IDs: hx-get URL includes ?wrapperID=email-body-wrapper-{msgID} to match eager-rendered card pattern"

key-files:
  created: []
  modified:
    - internal/web/handlers_test.go
    - internal/web/handlers_messages.go
    - internal/web/templates/thread.templ
    - internal/web/templates/thread_templ.go

key-decisions:
  - "wrapperID defaults to email-body-wrapper when absent — preserves backward compatibility for message detail page"
  - "All 6 fmt.Fprintf handler output paths use the parameterized wrapperID — no code paths left with hardcoded id"

patterns-established:
  - "Handler query param defaults: read optional param, set default if empty, use throughout handler"

requirements-completed: [RENDER-04, POLISH-01]

# Metrics
duration: 5min
completed: 2026-03-11
---

# Phase 10 Plan 01: Integration Cleanup Summary

**wrapperID query param added to messageBodyWrapper handler and thread lazy-load URLs scoped to per-message DOM IDs, fixing stale test assertions and duplicate ID bug**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-11T19:46:37Z
- **Completed:** 2026-03-11T19:51:49Z
- **Tasks:** 2
- **Files modified:** 15 (4 core + 11 formatting from go fmt ./...)

## Accomplishments

- Fixed two stale test assertions in TestMessageBodyWrapperEndpoint that referenced Phase 7 banner markup instead of Phase 9 unified toolbar
- Added TestBodyWrapperWithWrapperIDParam that validates the wrapperID scoping feature end-to-end
- Parameterized all 6 hardcoded `id="email-body-wrapper"` occurrences in messageBodyWrapper handler using wrapperID query param
- Updated thread.templ lazy-load hx-get to pass `?wrapperID=email-body-wrapper-{msgID}`, eliminating duplicate DOM IDs in thread view

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix stale test assertions and add wrapperID param test** - `eaeeab04` (test)
2. **Task 2: Add wrapperID param to handler + thread template lazy-load** - `383cd59f` (feat)

## Files Created/Modified

- `internal/web/handlers_test.go` - Updated 2 stale assertions in TestMessageBodyWrapperEndpoint; added TestBodyWrapperWithWrapperIDParam
- `internal/web/handlers_messages.go` - Added wrapperID param reading with default fallback; replaced all 6 hardcoded id occurrences
- `internal/web/templates/thread.templ` - Updated lazy-load hx-get URL to include ?wrapperID=email-body-wrapper-{msgID}
- `internal/web/templates/thread_templ.go` - Regenerated from thread.templ
- (11 other *_templ.go files) - Formatting-only changes from go fmt ./...

## Decisions Made

- wrapperID defaults to `email-body-wrapper` when the param is absent — preserves backward compatibility for the message detail page which does not pass the param
- Thread lazy-load passes wrapperID matching the eager-rendered card pattern (`email-body-wrapper-{msgID}`) for DOM symmetry

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `templ generate` reported `updates=0` after updating thread.templ — the generated file already contained the correct output (the tool was consistent). Verified by grepping thread_templ.go directly.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- INT-01 and INT-02 from v1.1 milestone audit are closed
- `go test ./internal/web/...` passes with zero failures
- Phase 10 plan 01 complete; ready for remaining integration cleanup plans if any

---
*Phase: 10-integration-cleanup*
*Completed: 2026-03-11*
