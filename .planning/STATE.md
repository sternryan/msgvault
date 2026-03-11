---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: Web UI Rebuild (Templ + HTMX)
status: planning
stopped_at: Completed 06-02-PLAN.md — Dashboard, Messages list, Message detail pages
last_updated: "2026-03-11T02:23:55.254Z"
last_activity: 2026-03-10 — Roadmap created for v1.1 milestone (phases 6-9)
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 5
  completed_plans: 2
  percent: 20
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-10)

**Core value:** Users can safely archive their entire Gmail history offline and search it instantly, with confidence that nothing is lost before deletion.
**Current focus:** Phase 6 — Foundation (PR #176 adoption, Templ + HTMX, single-binary)

## Current Position

Phase: 6 of 9 — v1.1 (Foundation)
Plan: 0 of TBD in current phase
Status: Ready to plan
Last activity: 2026-03-10 — Roadmap created for v1.1 milestone (phases 6-9)

Progress: [██░░░░░░░░] ~20% (v1.0 complete, v1.1 not started)

## Performance Metrics

**Velocity:**
- Total plans completed: 0 (v1.1)
- Average duration: —
- Total execution time: —

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1-5 (v1.0) | — | shipped | — |

*Updated after each plan completion*
| Phase 06-foundation P01 | 6min | 2 tasks | 17 files |
| Phase 06-foundation P02 | 4 | 2 tasks | 11 files |

## Accumulated Context

### Decisions

- [Pre-v1.1]: Directory-copy strategy for PR #176 adoption, not cherry-pick (avoids unresolvable conflicts between fork's store.Store handlers and PR #176's query.Engine-only handlers)
- [Pre-v1.1]: templ CLI must be pinned to v0.3.1001 in Makefile to match go.mod exactly
- [Pre-v1.1]: bluemonday sanitizeHTML helper established in helpers.go in Phase 6 before any template renders email bodies
- [Pre-v1.1]: All HTML email bodies render in sandboxed iframe — never allow-scripts + allow-same-origin together
- [Pre-v1.1]: HTMX hx-select pattern for partials — full pages always, HTMX extracts fragment client-side
- [Phase 06-01]: buildRouter() extracted from Start() for testability; chi middleware signatures updated to func(logger) func(http.Handler) http.Handler; renderPage centralizes account listing and deletion count on every page
- [Phase 06-02]: GetTotalStats used for messages pagination count — avoids adding SearchFastCount dependency for unfiltered list
- [Phase 06-02]: hx-get on tr rows instead of templ.SafeScript onclick — templ.SafeScript returns string not ComponentScript, HTMX native row navigation is superior
- [Phase 06-02]: Messages page limit locked to 50 in handler (not URL param); Pagination/SortHeader components extracted to components.templ for Plan 03-04 reuse

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 9]: Dashboard chart — CSS bar chart is the plan (POLISH-02), but validate approach is sufficient before Phase 9; SVG fallback if CSS insufficient
- [Phase 7]: Email iframe resizing edge cases — fixed min-height vs. ResizeObserver; decide during Phase 7 execution, document choice
- [Phase 7]: bluemonday policy specifics — validate exact allowlist against real email corpus; use sanitise_html_email reference, not UGCPolicy() directly
- [Phase 7]: CSRF on deletion POSTs — nosurf or gorilla/csrf with chi; neither in go.mod; decision needed during Phase 7 planning

## Session Continuity

Last session: 2026-03-11T02:23:55.252Z
Stopped at: Completed 06-02-PLAN.md — Dashboard, Messages list, Message detail pages
Resume file: None
