---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: Web UI Rebuild (Templ + HTMX)
status: planning
stopped_at: Completed 08-01-PLAN.md
last_updated: "2026-03-11T07:18:53.693Z"
last_activity: 2026-03-10 — Roadmap created for v1.1 milestone (phases 6-9)
progress:
  total_phases: 4
  completed_phases: 2
  total_plans: 9
  completed_plans: 8
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
| Phase 06-foundation P03 | 6min | 2 tasks | 8 files |
| Phase 06-foundation P04 | 5 | 2 tasks | 10 files |
| Phase 06-foundation P05 | 3min | 2 tasks | 36 files |
| Phase 07-email-rendering P01 | 5min | 2 tasks | 12 files |
| Phase 07-email-rendering P02 | 8min | 3 tasks | 9 files |
| Phase 08-thread-view P01 | 3min | 2 tasks | 6 files |

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
- [Phase 06-03]: BreadcrumbItem defined in templates/helpers.go (templates package) so templ generated code can reference it directly
- [Phase 06-03]: Sub-view tabs in drill-down include ?filterView={viewType} in URL; aggregateDrilldown branches on this param to call SubAggregate vs ListMessages
- [Phase 06-03]: Two-tier search: SearchFast first (DuckDB/Parquet), fall back to Search (FTS5) only when fast returns zero and TextTerms exist
- [Phase 06-04]: Two root-level OOB response: StageResult then DeletionBadgeOOB rendered sequentially to writer — HTMX silently ignores nested OOB swaps
- [Phase 06-04]: Account filter uses JS URL manipulation (not HTMX hx-get): setupAccountFilter() preserves all existing URL params when changing sourceId
- [Phase 06-04]: layout.templ always renders deletion-badge span (empty when count=0) so OOB swap can clear it
- [Phase 06-foundation]: serve.go rewritten as scheduler-only daemon — JSON API removed since React SPA is gone; templ-generate is dev-only target since _templ.go files are committed to repo
- [Phase 07-01]: bluemonday AllowUnsafe(true) required for style blocks; security depends on sandboxed iframe, NOT sanitizer alone
- [Phase 07-01]: CID substitution must run before bluemonday sanitization — bluemonday strips cid: URL scheme
- [Phase 07-01]: content_id migration uses PRAGMA table_info check before ALTER TABLE — safe for existing databases
- [Phase 07-02]: HTMX hx-get outerHTML swap on Load images (not JS src mutation): hx-target=#email-body-wrapper, hx-swap=outerHTML, server returns new wrapper with showImages iframe
- [Phase 07-02]: messageBody endpoint serves standalone HTML with CSP headers (img-src self+data default, * with showImages=true); X-Frame-Options omitted to allow parent framing
- [Phase 07-02]: iframe sandbox: allow-scripts allow-popups allow-popups-to-escape-sandbox (no allow-same-origin) — iframe JS cannot access parent DOM for XSS defense
- [Phase 08-01]: threadView handler pre-computes participants (deduplicated, insertion order) before passing to template
- [Phase 08-01]: hx-target=closest .email-render-wrapper on Load images in thread context to avoid multi-ID collision
- [Phase 08-01]: Per-message wrapper IDs (id=email-body-wrapper-{msgID}) in ThreadMessageCard for DOM uniqueness

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 9]: Dashboard chart — CSS bar chart is the plan (POLISH-02), but validate approach is sufficient before Phase 9; SVG fallback if CSS insufficient
- [Phase 7]: Email iframe resizing edge cases — fixed min-height vs. ResizeObserver; decide during Phase 7 execution, document choice
- [Phase 7]: bluemonday policy specifics — validate exact allowlist against real email corpus; use sanitise_html_email reference, not UGCPolicy() directly
- [Phase 7]: CSRF on deletion POSTs — nosurf or gorilla/csrf with chi; neither in go.mod; decision needed during Phase 7 planning

## Session Continuity

Last session: 2026-03-11T07:18:53.691Z
Stopped at: Completed 08-01-PLAN.md
Resume file: None
