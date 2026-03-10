# Architecture Research

**Domain:** Server-rendered Go web UI — Templ + HTMX integration with existing query/store packages
**Researched:** 2026-03-10
**Confidence:** HIGH

## System Overview

```
Browser                          Go HTTP Server (chi v5)
┌──────────────────────┐         ┌──────────────────────────────────────────┐
│  htmx.min.js (14KB)  │ HTML    │  internal/web/                           │
│  keys.js             │◄───────►│  ┌─────────────────────────────────────┐ │
│  style.css           │ partials│  │  server.go  (chi router, middleware) │ │
└──────────────────────┘         │  │  handlers.go         (page handlers) │ │
                                 │  │  handlers_thread.go  (thread view)   │ │
                                 │  │  handlers_deletions.go               │ │
                                 │  │  handlers_attachments.go             │ │
                                 │  │  params.go           (URL parsing)   │ │
                                 │  └──────────────┬──────────────────────┘ │
                                 │                 │                         │
                                 │  templates/     │  (compiled to _templ.go)│
                                 │  ┌──────────────┴──────────────────────┐ │
                                 │  │  layout.templ   dashboard.templ      │ │
                                 │  │  aggregates.templ messages.templ     │ │
                                 │  │  message_detail.templ thread.templ   │ │
                                 │  │  search.templ deletions.templ        │ │
                                 │  │  helpers.go  (non-template helpers)  │ │
                                 │  └──────────────┬──────────────────────┘ │
                                 │                 │                         │
                                 │  static/        │  (go:embed)             │
                                 │  ┌──────────────┴──────────────────────┐ │
                                 │  │  htmx.min.js   style.css  keys.js   │ │
                                 │  └─────────────────────────────────────┘ │
                                 │                 │                         │
                                 └─────────────────┼─────────────────────────┘
                                                   │
                              ┌────────────────────┼────────────────────┐
                              │                    │                    │
                    ┌─────────▼──────────┐  ┌──────▼───────────┐  ┌────▼──────────────────┐
                    │  query.Engine      │  │  store.Store     │  │  deletion.Manager     │
                    │  (DuckDB/SQLite)   │  │  (SQLite direct) │  │  (manifest files)     │
                    │  Aggregates, list, │  │  Message detail, │  │  Stage, list, cancel  │
                    │  search, stats     │  │  raw body, attach│  │                       │
                    └────────────────────┘  └──────────────────┘  └───────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | Package |
|-----------|----------------|---------|
| `server.go` | chi router setup, middleware stack, signal handling | `internal/web` |
| `handlers.go` | Dashboard, browse/aggregate, messages list, search page handlers | `internal/web` |
| `handlers_thread.go` | Thread view — fetch conversation, render chronological messages | `internal/web` (NEW) |
| `handlers_deletions.go` | Stage, list, cancel deletions via HTMX POST/DELETE | `internal/web` |
| `handlers_attachments.go` | Serve attachments inline or as download, CSP headers | `internal/web` |
| `params.go` | URL query param parsing, shared between all handler files | `internal/web` |
| `templates/*.templ` | Type-safe HTML components compiled to `_templ.go` by `templ generate` | `internal/web/templates` |
| `templates/helpers.go` | Pure Go helper functions used from templates (format bytes, dates) | `internal/web/templates` |
| `static/` | Vendored htmx.min.js, style.css, keys.js — embedded via `go:embed` | `internal/web/static` |
| `embed.go` | `//go:embed static/*` directive, exports `staticFS embed.FS` | `internal/web` |
| `query.Engine` | DuckDB over Parquet (fast) or SQLite fallback — aggregates, lists, search | `internal/query` |
| `store.Store` | SQLite direct — message detail, raw MIME body, attachments metadata | `internal/store` |
| `deletion.Manager` | File-based manifest system for staged Gmail deletions | `internal/deletion` |

## Recommended Project Structure

```
internal/web/
├── server.go                    # chi.NewRouter(), middleware, Start()
├── handlers.go                  # dashboard, browse, messages, search handlers
├── handlers_thread.go           # NEW: thread view handler
├── handlers_deletions.go        # HTMX deletion POST/DELETE handlers
├── handlers_attachments.go      # attachment inline/download handler
├── params.go                    # shared URL param parsing (reuse existing logic)
├── embed.go                     # //go:embed static/* static assets
├── static/
│   ├── htmx.min.js              # Vendored HTMX ~14KB (no CDN, single-binary)
│   ├── style.css                # Solarized theme (~5KB)
│   └── keys.js                  # Keyboard shortcuts + delete mode
└── templates/
    ├── layout.templ              # Base HTML shell, nav, help overlay
    ├── layout_templ.go           # Generated — committed to repo
    ├── dashboard.templ           # Stats overview, top senders/domains
    ├── dashboard_templ.go
    ├── aggregates.templ          # Browse with drill-down, HTMX partials
    ├── aggregates_templ.go
    ├── messages.templ            # Message list, sort controls, pagination
    ├── messages_templ.go
    ├── message_detail.templ      # Single message: headers, body, attachments
    ├── message_detail_templ.go
    ├── thread.templ              # NEW: Conversation thread view
    ├── thread_templ.go           # NEW: Generated
    ├── search.templ              # Search input + results (HTMX debounce)
    ├── search_templ.go
    ├── deletions.templ           # Deletion management UI
    ├── deletions_templ.go
    └── helpers.go                # formatBytes(), formatDate() — plain Go
```

### Structure Rationale

- **`templates/` subdirectory:** Keeps `.templ` source files and generated `_templ.go` files co-located but separated from handler logic. The `templates` package is imported by handler files.
- **`embed.go` at package root:** `go:embed` directives must be in the same package that declares the `embed.FS` variable — putting it at `internal/web/embed.go` means `staticFS` is accessible to `server.go` for route registration.
- **Handler files split by concern:** `handlers.go` (read-only page views), `handlers_thread.go` (thread-specific), `handlers_deletions.go` (mutation endpoints), `handlers_attachments.go` (file serving). Each file stays under ~300 LOC.
- **`helpers.go` is plain Go, not a `.templ` file:** Template helper functions (byte formatting, date display, HTML sanitization) live in a regular `.go` file inside the `templates` package, importable without running `templ generate`.

## Architectural Patterns

### Pattern 1: Handler Struct with Injected Dependencies

**What:** A single `handlers` struct holds `query.Engine`, `store.Store` (for message detail), `deletion.Manager`, `attachmentsDir`, and `logger`. All HTTP handlers are methods on this struct.

**When to use:** Always — this is the established pattern in the existing codebase (`internal/web/handlers.go` already does this).

**Trade-offs:** Slightly more setup than closures, but enables clean testing with mock implementations of `query.Engine`.

**Example:**
```go
// internal/web/handlers.go
type handlers struct {
    engine         query.Engine
    store          *store.Store     // ADDED: needed for message detail body
    attachmentsDir string
    deletions      *deletion.Manager
    logger         *slog.Logger
}

func (h *handlers) dashboard(w http.ResponseWriter, r *http.Request) {
    stats, err := h.engine.GetTotalStats(r.Context(), query.StatsOptions{})
    if err != nil {
        h.renderError(w, r, http.StatusInternalServerError, err)
        return
    }
    component := templates.Dashboard(stats)
    if err := component.Render(r.Context(), w); err != nil {
        h.logger.Error("render dashboard", "err", err)
    }
}
```

**Integration point:** `server.go` constructs `handlers` with the same `query.Engine`, `*store.Store`, and `*deletion.Manager` that `cmd/msgvault/cmd/web.go` already wires up via `initQueryEngine()`. The `web.NewServer()` signature gains a `*store.Store` parameter alongside the existing `query.Engine`.

### Pattern 2: HTMX Partial Detection with `htmx-go`

**What:** Handlers check `htmx.IsHTMX(r)` (from `github.com/angelofallars/htmx-go`) to decide whether to render a full page (layout + content) or just the content fragment. For simple cases, always render the full page and let HTMX use `hx-select` to extract the fragment — this avoids server-side conditional logic.

**When to use:** For drill-down (aggregates), search-as-you-type (debounced), and pagination. For initial page loads and message detail, always render the full page.

**Two approaches:**

Option A — `hx-select` (simpler, recommended for this project):
```go
// Handler always returns full page; HTMX client extracts target with hx-select
func (h *handlers) browse(w http.ResponseWriter, r *http.Request) {
    rows, _ := h.engine.Aggregate(r.Context(), viewType, opts)
    templates.Aggregates(rows).Render(r.Context(), w)
}
```
```html
<!-- Template — HTMX extracts #results on subsequent requests -->
<div hx-get="/browse" hx-select="#results" hx-target="#results">
  <div id="results">...</div>
</div>
```

Option B — Fragment detection (more explicit, from templ fragments feature):
```go
func (h *handlers) browse(w http.ResponseWriter, r *http.Request) {
    rows, _ := h.engine.Aggregate(r.Context(), viewType, opts)
    if htmx.IsHTMX(r) {
        // Render only the results fragment
        templ.Handler(templates.Aggregates(rows),
            templ.WithFragments("results")).ServeHTTP(w, r)
        return
    }
    templates.BrowsePage(rows).Render(r.Context(), w)
}
```

**Recommendation:** Start with Option A (hx-select) to match PR #176's established approach. Option B adds complexity only when needed for independent fragment updates.

### Pattern 3: `go:embed` for Static Assets (No CDN)

**What:** Vendor HTMX, CSS, and JS into `internal/web/static/` and embed them at build time with `//go:embed`. No CDN dependencies, single-binary output guaranteed.

**When to use:** Always — this is the explicit requirement ("no CDN, single-binary").

**How `go:embed` works with Templ:** Templ generates pure Go code (`.go` files). There is no separate build step that produces files needing embedding. The `_templ.go` files are just Go source — `go build` compiles them directly. Only the vendored static files (CSS, JS) require `go:embed`.

**Example:**
```go
// internal/web/embed.go
package web

import "embed"

//go:embed static/*
var staticFS embed.FS

// In server.go route registration:
staticSub, _ := fs.Sub(staticFS, "static")
r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
```

**Key distinction:** The current `embed.go` embeds `dist/` (React build output). The new `embed.go` embeds `static/` (vendored JS/CSS). The `_templ.go` files are compiled Go source, not embedded assets — they don't need `go:embed` at all.

### Pattern 4: Templ Component Rendering

**What:** Each Templ component implements `templ.Component` (the `Render(ctx context.Context, w io.Writer) error` interface). Handlers call `.Render(r.Context(), w)` directly or wrap with `templ.Handler()`.

**When to use:** `.Render()` directly for handlers that need pre/post logic around rendering. `templ.Handler()` for simple read-only routes.

**Example — direct render (recommended for handlers with error handling):**
```go
func (h *handlers) messageDetail(w http.ResponseWriter, r *http.Request) {
    id, err := pathInt64(r, "id")
    if err != nil {
        h.renderError(w, r, http.StatusBadRequest, err)
        return
    }
    msg, err := h.engine.GetMessage(r.Context(), id)
    if err != nil {
        h.renderError(w, r, http.StatusNotFound, err)
        return
    }
    // store.Store needed here for raw body access
    body, _ := h.store.GetMessageBody(r.Context(), id)
    sanitized := sanitizeHTML(body.HTMLBody) // bluemonday
    if err := templates.MessageDetail(msg, sanitized).Render(r.Context(), w); err != nil {
        h.logger.Error("render message detail", "err", err)
    }
}
```

### Pattern 5: Thread View Data Flow

**What:** Thread view (`/messages/{id}/thread`) uses the existing `ListMessages` with a `ConversationID` filter, matching the approach already in `internal/web/handlers.go`'s `getThread`. No new store methods needed.

**When to use:** `/messages/{id}/thread` route only.

**Data flow:**
```
GET /messages/42/thread
    ↓
handlers_thread.go: engine.GetMessage(ctx, 42)     → get conversationID
    ↓
engine.ListMessages(ctx, filter{ConversationID: &convID, SortAsc: date})
    ↓
store.GetMessageBody(ctx, msgID) × N                → raw bodies per message
    ↓
templates.Thread(messages, bodies).Render(ctx, w)
```

**Note:** The design spec references `query.Engine.GetThreadMessages` but the actual Engine interface uses `ListMessages` with a `ConversationID` filter (as seen in the existing `getThread` handler). This is the correct approach — no new Engine methods needed.

## Data Flow

### Full Page Request Flow

```
Browser GET /browse?groupBy=senders
    ↓
chi router → handlers.browse(w, r)
    ↓
params.go: parseViewType(r), parseAggregateOptions(r)
    ↓
query.Engine.Aggregate(ctx, ViewSenders, opts)   ← DuckDB over Parquet (fast)
    ↓
templates.BrowsePage(rows).Render(ctx, w)
    ↓ (templ renders to io.Writer = http.ResponseWriter)
Complete HTML page with layout, nav, data
```

### HTMX Partial Update Flow

```
Browser hx-get="/browse?groupBy=domains" (drill-down click)
    ↓ (HX-Request: true header present)
chi router → handlers.browse(w, r)
    ↓
[same handler, same Engine call]
    ↓
templates.BrowsePage(rows).Render(ctx, w)   ← full page rendered
    ↓
HTMX client applies hx-select="#results" → extracts and swaps target element
```

### Message Detail Flow (store.Store, not query.Engine)

```
Browser GET /messages/42
    ↓
handlers.messageDetail(w, r)
    ↓
query.Engine.GetMessage(ctx, 42)          ← metadata (fast, Parquet/SQLite)
    ↓
store.Store.GetMessageBody(ctx, 42)       ← raw body (SQLite direct, PK lookup only)
    ↓
bluemonday.UGCPolicy().Sanitize(htmlBody) ← XSS sanitization server-side
    ↓
templates.MessageDetail(msg, safeBody).Render(ctx, w)
```

### Static Asset Flow

```
Browser GET /static/htmx.min.js
    ↓
chi router: r.Handle("/static/*", ...)
    ↓
http.FileServer(http.FS(staticSub))
    ↓
embed.FS lookup: internal/web/static/htmx.min.js  ← in-binary, no disk I/O
```

## Integration Points

### New vs Modified Components

| Component | Status | Change |
|-----------|--------|--------|
| `internal/web/server.go` | MODIFIED | Replace `http.ServeMux` with chi, swap route registrations from JSON API + SPA to Templ page routes + static file serving. Remove `spaHandler()`. |
| `internal/web/handlers.go` | REPLACED | Delete JSON response handlers. New file: Templ-rendering page handlers using same `handlers` struct but with `*store.Store` added as field. |
| `internal/web/embed.go` | MODIFIED | Change `//go:embed all:dist` → `//go:embed static/*`. Rename `distFS` → `staticFS`. |
| `internal/web/embed_dev.go` | DELETED | Dev mode SPA serving no longer needed. |
| `internal/web/middleware.go` | KEPT | `loggingMiddleware`, `recoveryMiddleware` — reusable, no changes. Remove `corsMiddleware` (no longer needed). |
| `internal/web/templates/` | NEW | All `.templ` files + generated `_templ.go` files + `helpers.go`. |
| `internal/web/static/` | NEW | Vendored `htmx.min.js`, `style.css`, `keys.js`. |
| `internal/web/handlers_thread.go` | NEW | Thread view handler. |
| `internal/web/handlers_deletions.go` | NEW | Deletion HTMX handlers (moved from handlers.go). |
| `internal/web/handlers_attachments.go` | NEW | Attachment serving (logic reused from current handlers.go). |
| `internal/web/params.go` | NEW | URL param parsing extracted to shared file (currently inline in handlers.go). |
| `internal/api/` | DELETED | Entire package removed (separate API server with auth, rate limiting, scheduler). |
| `web/` | DELETED | React SPA source (19 files, Vite config, node_modules). |
| `cmd/msgvault/cmd/web.go` | MODIFIED | Remove `--dev` flag (CORS gone). Add `*store.Store` to `web.NewServer()` call (already returned by `initQueryEngine`). |
| `cmd/msgvault/cmd/serve.go` | DELETED | React serve command. |

### `web.NewServer()` Signature Change

Current:
```go
func NewServer(engine query.Engine, attachmentsDir string, deletions *deletion.Manager, logger *slog.Logger, dev bool) *Server
```

After rebuild:
```go
func NewServer(engine query.Engine, store *store.Store, attachmentsDir string, deletions *deletion.Manager, logger *slog.Logger) *Server
```

`initQueryEngine()` already returns `*store.Store` — the calling code in `cmd/web.go` just needs to pass it through.

### query.Engine Integration (Unchanged)

All existing `query.Engine` methods are used as-is. No new methods needed.

| Handler | Engine Method | Purpose |
|---------|---------------|---------|
| dashboard | `GetTotalStats()`, `ListAccounts()`, `Aggregate(ViewTime)` | Stats + time series |
| browse | `Aggregate()`, `SubAggregate()` | Drill-down aggregates |
| messages | `ListMessages()` | Filtered/sorted message list |
| message_detail | `GetMessage()`, `GetAttachment()` | Metadata + attachment info |
| thread | `GetMessage()`, `ListMessages(ConversationID filter)` | Thread messages |
| search | `SearchFastWithStats()`, fallback `Search()` | Full-text search |
| deletions | `GetGmailIDsByFilter()` | Resolve filter to Gmail IDs |

### store.Store Integration (New Usage in Web Layer)

`store.Store` is currently NOT used by `internal/web/` — it's only used for sync, TUI, and cache building. The new web layer needs it for one purpose: fetching raw message bodies for detail/thread views.

Required new method (or verify existing): `store.Store.GetMessageBody(ctx, messageID int64) (*MessageBody, error)` — returning the decompressed HTML and plain text body. This accesses `message_bodies` via PK lookup only (per the CLAUDE.md SQL guideline: never JOIN message_bodies in list queries).

### deletion.Manager Integration (Unchanged)

The `deletion.Manager` API is used identically to the current implementation:
- `CreateManifest()` → `SaveManifest()` for staging
- `ListPending()` / `ListInProgress()` / `ListCompleted()` / `ListFailed()` for status
- `CancelManifest()` for cancellation

The Templ handlers for deletions POST/DELETE instead of JSON API calls, but the underlying Manager calls are identical.

### go:generate + Committed _templ.go Files

**Decision: Commit `_templ.go` files** (templ maintainer's recommendation).

Rationale for this project:
- Single-binary philosophy requires `go build` alone to work
- Contributors shouldn't need `templ` CLI installed to build or run tests
- Public repo — external contributors can build without extra tooling

Build workflow:
```
Edit .templ files
    ↓
templ generate   (developer only, when editing templates)
    ↓
git add templates/*_templ.go   (commit generated files)
    ↓
go build         (everyone — no templ CLI needed)
```

`go:generate` directive in `server.go` or a `generate.go` file:
```go
//go:generate go run github.com/a-h/templ/cmd/templ@latest generate ./templates
```

Pin the version in `Makefile` for reproducible template regeneration:
```makefile
TEMPL_VERSION := v0.3.1001
templ-generate:
    go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate ./internal/web/templates
```

## Anti-Patterns

### Anti-Pattern 1: Accessing message_bodies in List Queries

**What people do:** JOIN `message_bodies` in `ListMessages` or aggregate queries to include body snippets.

**Why it's wrong:** `message_bodies` is a separate table specifically to keep the `messages` B-tree small for fast scans. Joining it in list/aggregate queries eliminates this optimization and causes severe performance degradation on large archives (20+ years of email).

**Do this instead:** Only access `message_bodies` via direct PK lookup (`WHERE message_id = ?`) in the message detail and thread handlers. Use the `snippet` column from `messages` for list views.

### Anti-Pattern 2: Implementing a JSON API Layer Between Templ Handlers and query.Engine

**What people do:** Add a JSON API server for the web UI to call, mirroring the current React SPA architecture.

**Why it's wrong:** It re-introduces the complexity being removed. Templ handlers are Go code — they call `query.Engine` directly with no serialization overhead. A JSON intermediary is only needed for external clients (MCP, mobile), not for the web UI itself.

**Do this instead:** Handlers call `query.Engine` and `store.Store` directly. JSON API can be re-added as a separate concern later if MCP access is needed.

### Anti-Pattern 3: Fetching CDN Resources in Templates

**What people do:** Use `<script src="https://unpkg.com/htmx.org@2.0.0/dist/htmx.min.js">` in layout template.

**Why it's wrong:** Breaks offline use (the core value proposition), introduces a hard CDN dependency, prevents single-binary purity. PR #176 explicitly chose "no CDN."

**Do this instead:** Vendor HTMX into `internal/web/static/htmx.min.js` and serve via `go:embed`.

### Anti-Pattern 4: Generating _templ.go in CI Only (Without Committing)

**What people do:** Add `_templ.go` to `.gitignore` and run `templ generate` in CI/CD.

**Why it's wrong:** For a public single-binary tool, `go build` must work for contributors without extra tools. Uncommitted generated files break `go build` for anyone without `templ` CLI installed.

**Do this instead:** Commit `_templ.go` files. Run CI check that verifies committed files match freshly generated output (catch stale generated files).

### Anti-Pattern 5: Using store.Store for Aggregate/List Queries

**What people do:** Use `store.Store` (SQLite) for all queries because it's simpler.

**Why it's wrong:** SQLite joins for aggregates on 20+ year archives are ~3000x slower than DuckDB over Parquet. The entire analytics layer exists to avoid this.

**Do this instead:** Use `query.Engine` for all aggregates, lists, and search. Reserve `store.Store` for message detail body access (PK lookup only) and admin operations (account management, sync state).

## Scaling Considerations

| Concern | Local Single-User Tool | Notes |
|---------|----------------------|-------|
| Query performance | DuckDB Parquet handles 1M+ messages | Already solved; web layer inherits it |
| Concurrent requests | chi with standard Go http — fine for single user | No multi-user concerns |
| Template rendering | In-memory, sub-millisecond per page | No caching needed |
| Static assets | In-binary via go:embed — zero disk I/O | No CDN, no external requests |
| HTML body rendering | bluemonday sanitization ~1ms per message | Acceptable; only on detail views |

## Build Order for Implementation

**Dependency order for phase execution:**

1. **`internal/web/static/`** — Add vendored files. No Go dependencies. Unblock embed.go.
2. **`internal/web/embed.go`** — Update `go:embed` directive. Depends on static/ existing.
3. **`internal/web/templates/helpers.go`** — Pure Go helpers (formatBytes, formatDate, sanitizeHTML with bluemonday). No templ dependency.
4. **`internal/web/templates/*.templ`** — Template components. Depend on helpers.go for helper functions.
5. **`templ generate`** — Produce `_templ.go` files. Depends on .templ files existing.
6. **`internal/web/params.go`** — URL param parsing (extracted from existing handlers.go). Depends on `query` package types only.
7. **`internal/web/handlers*.go`** — All handler files. Depend on templates package, params.go, query.Engine, store.Store, deletion.Manager.
8. **`internal/web/server.go`** — chi router, route registration. Depends on all handlers, embed.go.
9. **`cmd/msgvault/cmd/web.go`** — Wire updated `web.NewServer()` signature. Depends on server.go.

**Verify `go build ./...` passes after step 8 before touching cmd.**

## Sources

- [templ documentation — Web Frameworks (chi integration)](https://templ.guide/integrations/web-frameworks/)
- [templ documentation — HTMX integration](https://templ.guide/server-side-rendering/htmx/)
- [templ documentation — Fragments](https://templ.guide/syntax-and-usage/fragments/)
- [templ documentation — Template generation](https://templ.guide/core-concepts/template-generation/)
- [templ pkg.go.dev — Component interface, Handler, WithFragments](https://pkg.go.dev/github.com/a-h/templ) — version v0.3.1001 (Feb 2026)
- [htmx-go pkg.go.dev — IsHTMX, MustRenderTempl, header constants](https://pkg.go.dev/github.com/angelofallars/htmx-go)
- [templ Discussion #419 — Should generated files be committed?](https://github.com/a-h/templ/discussions/419)
- [bluemonday — Go HTML sanitizer for email display](https://github.com/microcosm-cc/bluemonday)
- Existing codebase: `internal/web/handlers.go`, `internal/web/server.go`, `internal/query/engine.go`, `cmd/msgvault/cmd/engine.go` — HIGH confidence (direct code read)

---
*Architecture research for: msgvault Templ + HTMX Web UI rebuild*
*Researched: 2026-03-10*
