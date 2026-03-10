# Web UI Rebuild: Templ + HTMX

**Date:** 2026-03-10
**Status:** Approved
**Approach:** Fork PR #176, collaborate with sarcasticbird, contribute thread view + inline attachments back upstream

## Context

The fork's current Web UI is a React 19 + TypeScript SPA (~1,900 LOC frontend, ~900 LOC backend) built with Vite, TanStack, Recharts, and Tailwind. It works but introduces a Node.js build dependency that conflicts with msgvault's single-binary Go philosophy.

Upstream PR #176 by sarcasticbird implements a Web UI using Templ (type-safe Go HTML templates) + HTMX (14KB vendored JS for partial page updates). Zero JS framework, zero npm, templates compile to Go, `go build` produces the complete binary. The PR achieves feature parity with the TUI: dashboard, browse, search, message detail, deletions — plus Vim-style keyboard nav and Solarized theming.

The fork needs thread view and inline attachment rendering, which #176 doesn't have.

## Decision

Replace the React SPA with Templ + HTMX by forking PR #176 and adding thread view + inline attachments as contributions back to the upstream PR.

### Why not keep React?

- Upstream will not accept a React SPA (PR #176's design doc: "No npm. No node. No JS build step.")
- React + Vite + Node.js is overkill for a read-heavy data browser with request-response interaction patterns
- Breaks single-binary purity — `go build` alone doesn't produce a complete artifact
- Server-rendered Templ handlers call `query.Engine` directly (no JSON serialization overhead)

## Architecture

```
Browser                         Go HTTP Server (chi)
┌─────────────┐  HTML pages     ┌──────────────────────────┐
│  htmx.js    │ ◄────────────► │  Templ Handlers           │
│  keys.js    │  HTMX partials  │    ↓                      │
│  style.css  │                 │  query.Engine (DuckDB)    │
└─────────────┘                 │  store.Store (SQLite)     │
                                │                           │
                                │  Static: go:embed         │
                                │  (htmx, css, js vendored) │
                                └───────────────────────────┘
```

### Package Layout

```
internal/web/
├── server.go                 # chi router, middleware, mount
├── handlers.go               # Page handlers (dashboard, browse, search)
├── handlers_thread.go        # NEW: Thread view handler
├── handlers_deletions.go     # Deletion staging handlers
├── handlers_attachments.go   # Download + inline rendering
├── params.go                 # URL query param parsing
├── static/
│   ├── style.css             # Solarized theme (~5KB)
│   ├── htmx.min.js           # Vendored HTMX (~14KB)
│   └── keys.js               # Keyboard shortcuts + delete mode
└── templates/
    ├── layout.templ           # Base layout, nav, help overlay
    ├── dashboard.templ        # Stats overview
    ├── aggregates.templ       # Browse with drill-down
    ├── messages.templ         # Message list + sort
    ├── message_detail.templ   # Single message + inline attachments
    ├── thread.templ           # NEW: Conversation thread view
    ├── search.templ           # Full-text search
    ├── deletions.templ        # Deletion management
    └── helpers.go             # Template helper functions
```

## Pages & Data Flow

| Page | Route | Data Source | HTMX |
|------|-------|-------------|------|
| Dashboard | `/` | query.Engine — stats, time series, top senders/domains | No |
| Browse | `/browse` | query.Engine — aggregates by dimension with drill-down | Yes |
| Messages | `/messages` | query.Engine — filtered/sorted list with pagination | Yes |
| Message Detail | `/messages/{id}` | store.Store — full message, headers, body, attachments | No |
| Thread | `/messages/{id}/thread` | store.Store — all messages in conversation | No |
| Search | `/search` | query.Engine (fast) → store.Store FTS5 (deep) | Yes |
| Deletions | `/deletions` | deletion.Manager — stage, list, cancel | Yes |

### Thread View

- All messages in a conversation, chronological order
- Each message: subject, from, date, body (HTML sanitized or plain text)
- Inline attachments (images) rendered via `<img src="/attachments/{id}/inline">`
- Link from message detail: "View thread (N messages)"
- Prev/next thread navigation

### Inline Attachments

- Content-type routing: images → `<img>`, PDFs → download link, other → download
- Served from existing content-addressed attachment storage
- SHA-256 hash validation on download
- CSP headers to sandbox inline content

### Key Design Decisions

- **No JSON API initially.** Templ handlers call `query.Engine` directly. JSON API added back when needed for MCP/mobile/programmatic access.
- **URL-driven state.** All view state in query params — bookmarkable, back-button works, no client-side state.
- **HTMX for partial updates.** Debounced search, drill-down, pagination — no full reloads within a view.
- **go:embed for everything.** CSS, JS, generated Templ code all in the binary.

## What Gets Removed

- `web/` — React SPA (19 files, package.json, node_modules)
- `internal/web/` — current Go handlers serving React
- `internal/api/` — separate API server with auth/rate limiting (~2,300 LOC)
- `cmd/msgvault/cmd/web.go` — React web command
- `cmd/msgvault/cmd/serve.go` — React serve command
- Makefile web-build targets, npm/Vite config

## What Gets Added

- `internal/web/` — Templ handlers + templates (from PR #176)
- `internal/web/templates/thread.templ` — new
- `handlers_thread.go` — new
- Inline attachment rendering in message_detail.templ and thread.templ
- `templ` CLI as dev dependency (`go install github.com/a-h/templ/cmd/templ@latest`)
- Committed `_templ.go` files — `go generate` only needed when editing templates

## Migration Strategy

### Phase 1: Adopt PR #176

1. Add sarcasticbird's repo as remote
2. Cherry-pick/merge `feature-templ-ui` branch into fork
3. Delete React `web/`, `internal/web/`, `internal/api/`
4. Resolve conflicts with fork extras (store changes, encryption)
5. Verify `go build` produces working binary with Templ UI
6. Add `templ` to dev toolchain

### Phase 2: Add Thread + Inline Attachments

1. `thread.templ` + `handlers_thread.go`
2. Inline attachment rendering in message_detail and thread templates
3. Attachment handler with content-type routing + CSP
4. Keyboard shortcut `t` for thread nav from message detail

### Phase 3: Contribute Back

1. Open PR against sarcasticbird's branch adding thread + inline
2. Comment on upstream #176 offering additions
3. If collaboration works → features land in #176 → #176 lands upstream
4. If unresponsive → maintain independently in fork

## Testing

- Handler tests with `httptest.NewRecorder` — verify status codes and expected HTML fragments
- Thread handler: real SQLite test DB, verify message ordering and conversation grouping
- Inline attachments: content-type routing, CSP headers, hash validation
- No frontend test framework (no frontend code)
- No E2E/browser tests (personal tool, pragmatic scope)

## Error Handling

- Template rendering errors → 500 page with Templ error layout
- Missing message/thread → 404 page
- DuckDB unavailable → fallback to SQLite query engine
- Invalid query params → redirect with defaults

## Planning Notes

- **Thread data source:** `store.Store` has no `GetMessagesByConversationID` — use `query.Engine.GetThreadMessages` which already exists (TUI uses it).
- **HTML sanitization:** Use `bluemonday` or similar Go-side sanitizer for message body HTML. Do not pass raw HTML to templates.
- **Templ version:** Pin to specific version in Makefile for reproducible builds.
- **Phase 1 conflict fallback:** If full branch merge is too messy, cherry-pick only `internal/web/` directory from PR #176 instead.

## Risk

If Wes rejects PR #176 or demands a fundamentally different approach, the fork has built on a rejected foundation. Mitigation: the Templ + HTMX stack is sound regardless — worst case it's maintained in the fork, or adapted to whatever upstream adopts.

## Build Toolchain Change

| Before | After |
|--------|-------|
| Node.js 22, npm, Vite 6 | None |
| React 19, Tailwind 4, TanStack, Recharts | None |
| `make web-build` → `npm run build` → embed | `go generate` → `go build` |
| ~201MB node_modules | 0 |
| `templ` CLI | `go install github.com/a-h/templ/cmd/templ@latest` |
