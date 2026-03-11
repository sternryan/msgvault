---
phase: 06-foundation
plan: 05
subsystem: ui
tags: [cleanup, react-removal, api-removal, makefile, pure-go]

requires:
  - phase: 06-04
    provides: "Complete Templ+HTMX web UI with deletions, keyboard shortcuts, account filter"

provides:
  - Repository with no web/ directory (React SPA fully removed)
  - Repository with no internal/api/ directory (JSON REST API fully removed)
  - Makefile with pure Go build (no npm dependency)
  - templ-generate Makefile target for dev workflow
  - serve command rewritten to scheduler-only daemon (no JSON API server)
  - go mod tidy with unused dependencies removed

affects: []

tech-stack:
  added: []
  patterns:
    - "build target: pure go build, no npm/web-build dependency"
    - "templ-generate: dev-only target using pinned templ@v0.3.1001"

key-files:
  created: []
  modified:
    - Makefile
    - cmd/msgvault/cmd/serve.go
    - go.mod
    - go.sum
  deleted:
    - web/ (entire React SPA directory — index.html, package.json, src/, tsconfig.json, vite.config.ts)
    - internal/api/ (JSON REST API — server.go, handlers.go, middleware.go, *_test.go)

key-decisions:
  - "serve.go rewritten to scheduler-only daemon — removed api.NewServer usage since JSON API is gone; scheduler loop + graceful shutdown retained"
  - "build target is now pure go build (no npm) — templ-generate is dev-only, _templ.go files are committed"
  - "config.toml in root left untouched — it is user-local config, not tracked by git"

requirements-completed: [FOUND-03, FOUND-05, FOUND-01]

duration: 3min
completed: 2026-03-11
---

# Phase 06 Plan 05: React SPA and JSON API Removal Summary

**web/ and internal/api/ deleted; Makefile updated to pure Go build; serve.go rewritten as scheduler-only daemon; go build works without npm or templ CLI**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-11T02:44:26Z
- **Completed:** 2026-03-11T02:47:00Z
- **Tasks:** 2 (1 auto + 1 auto-approved checkpoint)
- **Files modified:** 4 (36 total changes counting deletions)

## Accomplishments
- Deleted entire `web/` directory (React SPA: index.html, package.json, TypeScript source, Vite config, node_modules)
- Deleted entire `internal/api/` directory (JSON REST API: server.go, handlers.go, middleware.go, tests)
- Rewrote `cmd/msgvault/cmd/serve.go`: removed `api.NewServer` usage, kept scheduler daemon loop with graceful shutdown
- Updated `Makefile`: `build` target no longer calls `web-build`, added `templ-generate` dev target, removed web-* targets, updated `clean` and `help`
- Ran `go mod tidy` to remove unused dependencies
- All `_templ.go` files confirmed present and up to date (templ generate reported 0 updates)
- Binary builds cleanly: `go build -tags fts5 ./cmd/msgvault` succeeds
- `internal/web` tests pass: `ok internal/web 0.395s`

## Task Commits

Each task was committed atomically:

1. **Task 1: Delete React SPA, JSON API, update Makefile, rewrite serve.go** - `3f0659a` (chore)
2. **Task 2: checkpoint:human-verify** - Auto-approved (per execution directive)

## Files Created/Modified
- `Makefile` - Pure Go build: removed web-build dependency from build, removed web-install/web-dev/web-build targets, added templ-generate target, updated clean and help
- `cmd/msgvault/cmd/serve.go` - Scheduler-only daemon: removed api.NewServer and storeAPIAdapter/schedulerAdapter; kept scheduler setup, graceful shutdown, runScheduledSync
- `go.mod` - go mod tidy applied (removed api-package dependencies)
- `go.sum` - Updated by go mod tidy

## Decisions Made
- `serve.go` rewritten as scheduler-only daemon — the JSON API (`internal/api`) served the old React SPA; since the SPA is gone, the API server has no clients. The scheduler (cron sync) remains useful for automated background syncing. The `web` command (`msgvault web --port 8484`) provides the Templ+HTMX web UI.
- `build` target is now pure `go build` — all `_templ.go` files are committed to the repository, so `go build` does not require the templ CLI. `templ-generate` is available for developers regenerating templates after `.templ` changes.
- `config.toml` in project root left untouched — it is untracked (user-local config), not a stale artifact.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing functionality] Rewrote serve.go instead of removing it**
- **Found during:** Task 1 (checking serve.go before deletion)
- **Issue:** The plan said to remove serve.go if it imported `internal/api`, but deleting it entirely would remove the scheduler daemon command (`msgvault serve`). The plan's intent was to clean up the JSON API, not the scheduler.
- **Fix:** Rewrote `serve.go` to remove all `internal/api` references while keeping the scheduler daemon functionality. The result is ~140 lines vs 295 original — cleaner and focused on scheduling only.
- **Files modified:** cmd/msgvault/cmd/serve.go
- **Verification:** `go build -tags fts5 ./cmd/msgvault` succeeds; no api import present

## Auto-Approved Checkpoint

**Task 2 (checkpoint:human-verify):** Auto-approved per execution directive.

What was verified automatically:
- `web/` directory does not exist
- `internal/api/` directory does not exist
- `go build -tags fts5 ./cmd/msgvault` succeeds (no npm, no templ CLI required)
- `go test -tags fts5 ./internal/web/...` passes
- All `_templ.go` files present in `internal/web/templates/`
- `make build` target has no web-build dependency
- `templ generate` reports 0 updates (all files current)

## Self-Check: PASSED

- web/ directory: GONE (confirmed `[ ! -d web ]`)
- internal/api/ directory: GONE (confirmed `[ ! -d internal/api ]`)
- Makefile build target: no web-build dependency (confirmed)
- templ-generate target: present in Makefile (confirmed)
- All _templ.go files: 9 files present in internal/web/templates/ (confirmed)
- Binary build: PASSED (`go build -tags fts5 -o /dev/null ./cmd/msgvault`)
- internal/web tests: PASSED (`ok internal/web 0.395s`)
- commit 3f0659a: FOUND

---
*Phase: 06-foundation*
*Completed: 2026-03-11*
