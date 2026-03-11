---
phase: 06-foundation
plan: 01
subsystem: ui
tags: [templ, htmx, chi, go-embed, solarized, css, integration-tests]

requires: []

provides:
  - chi router with all route registrations, Server struct, Start method, and testable buildRouter()
  - go:embed static/* directive serving htmx.min.js, style.css, keys.js
  - Base HTML layout template (layout.templ) with navbar, account dropdown, deletion badge, HTMX attributes
  - Stub handlers for all 10 page/action routes returning valid HTML immediately
  - Parameter parsing helpers (params.go) extracted from old handlers
  - Integration test scaffold with mock query.Engine and 9 passing test functions
  - Solarized Dark CSS (~500 lines) with compact data density

affects: [06-02, 06-03, 06-04, 06-05]

tech-stack:
  added:
    - github.com/a-h/templ v0.3.1001
    - htmx.org 2.0.8 (vendored)
  patterns:
    - chi router with buildRouter() method for testability (same router in tests and production)
    - templ Layout with templ.WithChildren for composable page rendering
    - renderPage helper calls ListAccounts + pendingDeletionCount for every page automatically
    - hx-select="#main-content" pattern for HTMX partial updates without dedicated partial endpoints
    - Static assets embedded via go:embed static/* with staticSubFS() helper

key-files:
  created:
    - internal/web/params.go
    - internal/web/static/htmx.min.js
    - internal/web/static/style.css
    - internal/web/static/keys.js
    - internal/web/templates/helpers.go
    - internal/web/templates/layout.templ
    - internal/web/templates/layout_templ.go
    - internal/web/templates/stub.templ
    - internal/web/templates/stub_templ.go
    - internal/web/handlers_test.go
  modified:
    - internal/web/embed.go
    - internal/web/handlers.go
    - internal/web/middleware.go
    - internal/web/server.go
    - cmd/msgvault/cmd/web.go
    - go.mod
    - go.sum
  deleted:
    - internal/web/embed_dev.go
    - internal/web/dist/ (React SPA artifacts)

key-decisions:
  - "Removed dev bool from NewServer — no CORS needed for server-rendered approach"
  - "buildRouter() extracted from Start() so tests can call it without starting HTTP listener"
  - "Middleware signatures updated to chi pattern: func(logger) func(http.Handler) http.Handler"
  - "renderPage helper centralizes account list + deletion badge count on every request"
  - "Pre-existing test failures in cmd/msgvault/cmd and internal/mbox are out-of-scope (pre-date this plan)"

patterns-established:
  - "templ.WithChildren pattern: page = Layout(...); page.Render(templ.WithChildren(ctx, content), w)"
  - "All page routes go through renderPage() for consistent layout rendering"
  - "attachment handlers use http.Error (not renderPage) since they serve binary, not HTML"
  - "chi.URLParam(r, 'id') for path parameter extraction with chi router"

requirements-completed: [FOUND-01, FOUND-02, FOUND-04, FOUND-05]

duration: 6min
completed: 2026-03-11
---

# Phase 06 Plan 01: Foundation — Templ+HTMX+chi Infrastructure Summary

**chi router + templ layout + HTMX 2.0.8 + Solarized Dark CSS replacing React SPA scaffold, with 9 passing integration tests via mock query.Engine**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-11T02:09:35Z
- **Completed:** 2026-03-11T02:15:36Z
- **Tasks:** 2
- **Files modified:** 17 (10 created, 5 modified, 2 deleted)

## Accomplishments
- Replaced JSON API + React SPA scaffold with server-rendered Templ+HTMX+chi foundation
- Created Solarized Dark CSS with compact data density, all component styles for Phase 6 pages
- Established testable server architecture (buildRouter extracts router for httptest use)
- 9 integration tests all pass, covering every page route and all 3 static assets

## Task Commits

Each task was committed atomically:

1. **Task 1: Static assets, embed, params, templ, server, handlers, middleware, layout** - `5425a30` (feat)
2. **Task 2: Integration test scaffold with mock query.Engine** - `4830cd0` (test)

**Plan metadata:** (docs commit pending)

## Files Created/Modified
- `internal/web/embed.go` - go:embed static/*, staticSubFS() helper (removed build tags)
- `internal/web/params.go` - All parameter parsing helpers as package-level functions
- `internal/web/static/htmx.min.js` - HTMX 2.0.8 vendored (51KB)
- `internal/web/static/style.css` - Solarized Dark CSS (~500 lines)
- `internal/web/static/keys.js` - Keyboard shortcut stub
- `internal/web/templates/helpers.go` - FormatBytes, FormatNum, FormatTime, FormatDate, Pluralize
- `internal/web/templates/layout.templ` - Base HTML layout with navbar, account dropdown, deletion badge
- `internal/web/templates/layout_templ.go` - Generated from layout.templ
- `internal/web/templates/stub.templ` - StubPage and ErrorContent components
- `internal/web/templates/stub_templ.go` - Generated from stub.templ
- `internal/web/handlers.go` - Stub handlers + attachment handlers + renderPage/renderError helpers
- `internal/web/middleware.go` - Chi-compatible logging+recovery, CORS removed
- `internal/web/server.go` - Chi router, buildRouter() method, Start with context shutdown
- `internal/web/handlers_test.go` - Integration tests (mockEngine, 9 test functions)
- `cmd/msgvault/cmd/web.go` - Removed --dev flag, updated NewServer call

## Decisions Made
- Removed `dev bool` from `NewServer` — CORS not needed for server-rendered HTML (locked decision from STATE.md)
- Extracted `buildRouter()` from `Start()` so integration tests use the real router without a live HTTP listener
- Updated middleware signatures to chi pattern: `func(logger *slog.Logger) func(http.Handler) http.Handler`
- `renderPage` helper centralizes account listing and pending deletion count on every page request

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered
- Pre-existing build failures in `cmd/msgvault/cmd` (TestEmailValidation redeclared, buildCache signature mismatch) and `internal/mbox` — confirmed pre-date this plan via git stash verification. Logged to deferred-items, out-of-scope.

## Next Phase Readiness
- Foundation complete: binary compiles, server starts, all routes return styled HTML
- layout.templ provides account dropdown and deletion badge infrastructure for Plans 02-04
- params.go and template helpers ready for use by all subsequent plans
- Integration tests will grow as Plans 02-04 replace stubs with real implementations

---
*Phase: 06-foundation*
*Completed: 2026-03-11*
