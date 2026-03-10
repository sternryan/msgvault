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

## Accumulated Context

### Decisions

- [Pre-v1.1]: Directory-copy strategy for PR #176 adoption, not cherry-pick (avoids unresolvable conflicts between fork's store.Store handlers and PR #176's query.Engine-only handlers)
- [Pre-v1.1]: templ CLI must be pinned to v0.3.1001 in Makefile to match go.mod exactly
- [Pre-v1.1]: bluemonday sanitizeHTML helper established in helpers.go in Phase 6 before any template renders email bodies
- [Pre-v1.1]: All HTML email bodies render in sandboxed iframe — never allow-scripts + allow-same-origin together
- [Pre-v1.1]: HTMX hx-select pattern for partials — full pages always, HTMX extracts fragment client-side

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 9]: Dashboard chart — CSS bar chart is the plan (POLISH-02), but validate approach is sufficient before Phase 9; SVG fallback if CSS insufficient
- [Phase 7]: Email iframe resizing edge cases — fixed min-height vs. ResizeObserver; decide during Phase 7 execution, document choice
- [Phase 7]: bluemonday policy specifics — validate exact allowlist against real email corpus; use sanitise_html_email reference, not UGCPolicy() directly
- [Phase 7]: CSRF on deletion POSTs — nosurf or gorilla/csrf with chi; neither in go.mod; decision needed during Phase 7 planning

## Session Continuity

Last session: 2026-03-10
Stopped at: Roadmap written, STATE.md initialized. Ready for /gsd:plan-phase 6.
Resume file: None
