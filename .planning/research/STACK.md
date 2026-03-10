# Stack Research

**Domain:** Server-rendered Go web UI (Templ + HTMX) for email archive browser
**Researched:** 2026-03-10
**Confidence:** HIGH — all versions verified against GitHub releases and PR #176 source

## Context: What This Research Covers

This is a SUBSEQUENT MILESTONE on an existing Go codebase. The research scope is strictly
the NEW capabilities required for the Templ + HTMX web UI rebuild. Existing stack (Go,
chi/v5, SQLite, DuckDB, Bubble Tea, OAuth2, etc.) is validated and NOT re-researched.

**Verified status of existing dependencies:**
- `github.com/go-chi/chi/v5 v5.2.5` — already in go.mod, used by PR #176's web server
- All other existing dependencies remain unchanged

## Recommended Stack

### Core Technologies (New Additions Only)

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `github.com/a-h/templ` | `v0.3.1001` | Type-safe Go HTML templates that compile to Go code | The only Go library where templates are Go functions — no reflection, compile-time type safety, templates import as regular Go packages. PR #176 uses this version. It is the current stable release (published 2026-02-28). |
| HTMX (vendored JS) | `2.0.4` | Partial HTML updates via HTTP attributes | PR #176 vendors `htmx.min.js` directly into `internal/web/static/`. No npm, no CDN, embedded via `go:embed`. HTMX 2.0.4 is the version in PR #176; latest is 2.0.7 (2025-09-11) — either works, pin to what PR uses for clean merge. |
| `github.com/microcosm-cc/bluemonday` | `v1.0.26` | HTML sanitizer for email body rendering | PR #176 strips HTML to plain text. The design spec requires rendered HTML for thread view (inline images, link preservation). `bluemonday.UGCPolicy()` is the standard safe policy for user-generated content. Used with `@templ.Raw()` — Templ explicitly supports this pattern. No alternative exists in the Go ecosystem with comparable policy control. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/net/html` | already indirect via `golang.org/x/net v0.49.0` | HTML parsing (bluemonday dependency) | Pulled in automatically when bluemonday is added |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `templ` CLI | Generates `_templ.go` files from `.templ` sources | `go install github.com/a-h/templ/cmd/templ@v0.3.1001` — pin to same version as go.mod. Only needed when editing `.templ` files; pre-generated `_templ.go` files are committed so `go build` works without it. |

## Installation

```bash
# One new Go dependency
go get github.com/a-h/templ@v0.3.1001

# One new Go dependency for HTML sanitization (thread view + message detail)
go get github.com/microcosm-cc/bluemonday@v1.0.26

# Dev tool (not a go.mod dependency)
go install github.com/a-h/templ/cmd/templ@v0.3.1001

# Vendor HTMX — copy file directly, no package manager
# Download from: https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js
# Place at: internal/web/static/htmx.min.js
# PR #176 already has this file — it's picked up via cherry-pick/merge
```

## What NOT to Add

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `html/template` (stdlib) | No component composition, no type safety, verbose escaping | `github.com/a-h/templ` |
| `templ/runtime` CDN loading | CDN breaks single-binary philosophy | Vendor `htmx.min.js` via `go:embed` |
| Any JS framework (React, Vue, Alpine) | Defeats the purpose of this rebuild — upstream explicitly requires "no npm, no node, no JS build step" | HTMX for partial updates, vanilla JS for keyboard shortcuts (already in `keys.js`) |
| `npm` / `node_modules` | Build dependency that breaks `go build` producing a complete binary | Removed entirely in this milestone |
| `github.com/gorilla/mux` or `net/http.ServeMux` for routing | chi is already in go.mod and PR #176 uses it; switching routers creates merge conflicts | `github.com/go-chi/chi/v5 v5.2.5` (existing) |
| `templ generate` as a required build step | Forces all contributors to have `templ` CLI installed | Commit `_templ.go` files alongside `.templ` sources; `templ generate` only needed when modifying templates |
| A separate JSON API server | The design spec explicitly defers this: "No JSON API initially. Templ handlers call `query.Engine` directly." | Direct `query.Engine` calls from handlers |
| `html/template` for email body rendering | Escapes all HTML, making email bodies unreadable | `bluemonday.UGCPolicy().Sanitize()` + `@templ.Raw()` |
| A full-page-render-only approach | Email archives have large result sets requiring partial updates for drill-down, pagination, search debounce | HTMX partial endpoints at `/htmx/*` routes |

## Integration with Existing Codebase

### query.Engine interface (no changes needed)

`query.Engine` in `internal/query/` is the primary data source for all web handlers. PR #176
uses it directly — `engine.GetTotalStats`, `engine.Aggregate`, `engine.ListMessages`,
`engine.GetMessage`, `engine.GetAttachment`. The thread view uses `engine.ListMessages`
with a `ConversationID` filter (already supported by `query.MessageFilter`).

```go
// Thread handler — uses existing engine, no new methods needed
filter := query.MessageFilter{
    ConversationID: &convID,
    Sorting: query.MessageSorting{Field: query.MessageSortByDate, Direction: query.SortAsc},
    Pagination: query.Pagination{Limit: 100},
}
msgs, err := h.engine.ListMessages(ctx, filter)
```

### store.Store (not needed in web handler)

The design spec mentions `store.Store.GetMessagesByConversationID` but the existing
`query.Engine` already supports conversation filtering. The web handler struct only needs
`query.Engine`, `deletion.Manager`, and `attachmentsDir` — matching PR #176's `NewHandler`
signature exactly.

### bluemonday integration point

The only place bluemonday is called is in `helpers.go` for email body rendering:

```go
// In internal/web/templates/helpers.go
import "github.com/microcosm-cc/bluemonday"

var ugcPolicy = bluemonday.UGCPolicy()

func sanitizeHTML(s string) string {
    return ugcPolicy.Sanitize(s)
}
```

Then in `.templ` files:

```go
// In message_detail.templ and thread.templ
if msg.BodyHTML != "" {
    @templ.Raw(sanitizeHTML(msg.BodyHTML))
} else {
    <pre class="msg-body">{ msg.BodyText }</pre>
}
```

PR #176's `htmlToPlainText` helper (regex-based strip) is kept for contexts where
plain text is preferred. `sanitizeHTML` is additive.

### chi router mounting (no changes to existing internal/api/server.go needed)

PR #176 mounts the Templ web UI handler at root (`r.Mount("/", webHandler.Routes())`)
inside the chi router that already exists in `internal/api/server.go`. The fork's
version of `internal/web/` currently serves the React SPA — that package is replaced
wholesale by PR #176's Templ-based version.

### go:embed pattern (no changes needed)

PR #176 uses the same `go:embed` pattern already established in the codebase:

```go
//go:embed static
var staticFS embed.FS
```

Static assets (CSS, JS) are embedded. Generated `_templ.go` files are Go source,
compiled directly — no embed needed for templates.

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `github.com/a-h/templ` | `html/template` (stdlib) | Never for this project — component composition and type safety are required for a maintainable multi-page UI |
| bluemonday `UGCPolicy` | `bluemonday.StrictPolicy()` | When zero HTML is acceptable (renders plain text only) — not appropriate here since HTML email rendering requires inline images and formatting |
| Vendor htmx.min.js | Load from CDN | Never for a privacy-first offline tool — CDN load leaks access patterns and requires network |
| Commit `_templ.go` files | `.gitignore` them, require `go generate` | Only if team has `templ` CLI standardized in all dev environments; for an open source tool, committing generated files is the friendlier approach |

## Version Compatibility

| Package | Compatible With | Notes |
|---------|----------------|-------|
| `github.com/a-h/templ v0.3.1001` | Go 1.23+ (project uses 1.25.8) | Full compatibility. Templ generates standard Go code — no runtime surprises. |
| `github.com/a-h/templ v0.3.1001` | `templ` CLI `v0.3.1001` | CLI and runtime library versions must match exactly. Pin both. |
| `github.com/microcosm-cc/bluemonday v1.0.26` | `golang.org/x/net` (already in go.mod as indirect) | bluemonday uses x/net/html; already an indirect dependency, no version conflict. |
| HTMX 2.0.4 (vendored) | Any modern browser | HTMX 2.x dropped IE11 support. Acceptable for a personal desktop tool. |

## Build Toolchain Change Summary

| Before (React SPA) | After (Templ + HTMX) |
|--------------------|----------------------|
| Node.js 22 required | None |
| `npm install` → `~201MB node_modules` | None |
| `make web-build` → Vite bundler | `make generate` → `templ generate` (optional if `_templ.go` committed) |
| `go build` alone produces incomplete binary | `go build` alone produces complete binary |
| React 19, TanStack, Recharts, Tailwind | Zero JS frameworks |
| `internal/web/dist/` embedded | `internal/web/static/` embedded (CSS + vendored htmx.min.js only) |

## Sources

- PR #176 go.mod diff — `github.com/a-h/templ v0.3.1001` confirmed as the only Go dependency addition (HIGH confidence, direct source inspection)
- `github.com/a-h/templ` releases — v0.3.1001 is current stable, published 2026-02-28 (HIGH confidence, GitHub API)
- HTMX releases — `bigskysoftware/htmx` latest is v2.0.7; PR #176 vendors 2.0.4 (HIGH confidence, GitHub API + source inspection)
- `github.com/go-chi/chi` releases — v5.2.5 is current stable (HIGH confidence, GitHub API)
- `github.com/microcosm-cc/bluemonday` releases — v1.0.26 is current stable, last updated 2023-10-12 (HIGH confidence, GitHub API — project is stable/mature)
- Templ raw HTML docs — `@templ.Raw()` with explicit sanitization is the documented pattern for rendering trusted HTML (HIGH confidence, official templ repository)
- PR #176 message_detail.templ — `htmlToPlainText` regex approach confirmed; bluemonday is NOT in PR #176 (HIGH confidence, direct source inspection)
- PR #176 Makefile diff — `generate:` target uses `templ generate`; pre-generated files committed (HIGH confidence, direct source inspection)

---
*Stack research for: Templ + HTMX web UI rebuild in msgvault*
*Researched: 2026-03-10*
