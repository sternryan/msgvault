# Phase 6: Foundation - Research

**Researched:** 2026-03-10
**Domain:** Go server-rendered web UI with Templ + HTMX, single-binary embedding
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Visual Design**: Solarized Dark palette exclusively — no light mode, no toggle. CSS custom properties: `--base03: #002b36`, `--base02: #073642`, `--base01: #586e75`, `--base0: #839496`, `--base1: #93a1a1`, `--blue: #268bd2`, `--cyan: #2aa198`, `--green: #859900`, `--red: #dc322f`, `--yellow: #b58900`
- **CSS**: Single hand-written CSS file (~300-500 lines), embedded via `go:embed static/style.css`. No CSS framework (no Tailwind, no Bootstrap).
- **Data density**: Compact rows, tight padding, 15+ rows visible on 1080p
- **Navigation**: Top navbar with links — Dashboard | Messages | Aggregate | Search | Deletions. Account filter dropdown always visible. Breadcrumbs for aggregate drill-down.
- **Keyboard shortcuts**: Mirror TUI keybindings in `keys.js` (embedded via go:embed): j/k navigate, Enter drill-down, Esc back, Tab cycle view types, s cycle sort, r reverse sort, t Time view, a account filter, / focus search, ? help overlay, q no-op
- **PR #176 base**: Directory-copy strategy, templ CLI pinned to v0.3.1001, `_templ.go` files committed so `go build` works without CLI
- **HTMX hx-select pattern**: Full pages always served; HTMX extracts fragment client-side
- **Artifact cleanup**: Delete `internal/api/` and `web/` entirely (normal delete commit, preserve git history)
- **Search**: Debounced live search `hx-trigger="input changed delay:500ms, keyup[key=='Enter']"`. Two-tier: DuckDB fast path first, FTS5 fallback. Results replace below input.
- **Aggregate**: Filter bar (debounced), clickable column headers for sort, 7 view types preserved
- **Deletion staging**: Inline via `hx-post`, badge count via `hx-swap-oob`, staged rows get strikethrough. Cancel via `hx-delete` with row fade-out.
- **Pagination**: Offset, 50 rows/page, URL params `?page=1&limit=50`, `hx-replace-url` keeps URL bookmarkable, "Showing 1-50 of N messages"
- **Dashboard**: Stat cards (total messages, accounts, size, date range), top 5 senders, top 5 domains. Time-series chart deferred to Phase 9.
- **chi router**: Replaces `http.ServeMux` (already in go.mod v5.2.5)

### Claude's Discretion
- Exact CSS specifics (spacing, font sizes, border-radius) within Solarized Dark constraints
- Error page design (404, 500)
- Loading indicator style (spinner vs skeleton vs text)
- Help overlay design for `?` shortcut
- How to handle empty states (no messages, no search results)
- Whether to show message snippet in message list rows

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| FOUND-01 | User can access the web UI from a single `go build` binary with no npm/Node.js dependency | `go:embed` for all static assets; `_templ.go` committed; Makefile updated to drop `web-build` from default `build` target |
| FOUND-02 | Web UI serves all pages via server-rendered Templ + HTMX | Templ v0.3.1001 component pattern; chi router; handler struct with `query.Engine` + `deletion.Manager` |
| FOUND-03 | React SPA (`web/`), JSON API server (`internal/api/`), and all npm/Vite tooling are removed | Normal `git rm -r` delete commit; no squash per CONTEXT.md |
| FOUND-04 | All static assets (HTMX, CSS, JS) are embedded via `go:embed` in the binary | `//go:embed static/*` on `embed.FS`; `fs.Sub` to serve at `/static/` |
| FOUND-05 | Generated `_templ.go` files are committed so `go build` works without the templ CLI | Commit all `*_templ.go` files; Makefile `templ-generate` target is dev-only, not part of `build` |
| PARITY-01 | User can view dashboard with archive stats overview | `query.Engine.GetTotalStats()` + `ListAccounts()` + top senders/domains via `Aggregate()`; chart deferred to Phase 9 |
| PARITY-02 | User can browse aggregates with drill-down across all 7 view types | URL-driven state: `?groupBy=senders&filterKey=...&filterView=...`; `Aggregate()` + `SubAggregate()` |
| PARITY-03 | User can view paginated message list with sort and filter | `ListMessages()` with `MessageFilter` pagination; offset params; `hx-replace-url` |
| PARITY-04 | User can search messages with full-text search (debounced input) | `SearchFast()` → `Search()` fallback; `hx-trigger="input changed delay:500ms"` |
| PARITY-05 | User can view message detail with headers, body, and attachments | `GetMessage()` + attachment routes; body rendered as plain text in Phase 6 (HTML body deferred to Phase 7) |
| PARITY-06 | User can stage messages for deletion and manage staged deletions | `hx-post` to stage; `hx-swap-oob` for navbar badge; `deletion.Manager.CreateManifest()` + `SaveManifest()` |
| PARITY-07 | User can navigate the web UI with Vim-style keyboard shortcuts | `keys.js` embedded; document-level `keydown` listener; `data-href` pattern for HTMX row navigation |
| PARITY-08 | User can filter all views by account (multi-account support) | Account filter dropdown in navbar; `?sourceId=N` propagated across all handlers |
</phase_requirements>

---

## Summary

Phase 6 replaces the React SPA with server-rendered HTML using Templ (Go template compiler) and HTMX (hypermedia-driven partial page updates). The output is a single Go binary — no Node.js, no npm, no CDN — achieved by embedding HTMX, CSS, and the keyboard JS file via `go:embed`, and by committing all generated `_templ.go` files so `go build` works without the templ CLI installed.

The existing codebase already has chi v5.2.5 in go.mod and all the domain logic in `query.Engine` and `deletion.Manager`. The JSON API server (`internal/api/`) and React SPA (`web/`) are deleted; `internal/web/` becomes the new Templ+HTMX server. The `handlers` struct pattern, middleware, and parameter-parsing helpers from the existing `internal/web/` package are reused or lightly adapted.

HTMX's `hx-select` pattern (serve full pages, extract fragment client-side) means handlers always return full pages; no separate fragment endpoints required. Out-of-band swaps (`hx-swap-oob`) handle the deletion badge count in the navbar without reloading. Keyboard shortcuts are implemented in a small vanilla JS file (`keys.js`) embedded in the binary and referenced from the base layout.

**Primary recommendation:** Build `internal/web/` as the Templ+HTMX package, add templ as a go.mod dependency (runtime only), keep HTMX as a vendored static file under `internal/web/static/`, commit all `_templ.go` files, update the Makefile so `build` no longer depends on npm.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/a-h/templ | v0.3.1001 | Go HTML template compiler | Already pinned in STATE.md; compiles `.templ` to Go code, type-safe, zero runtime overhead |
| HTMX | 2.0.8 (vendored file) | Hypermedia-driven partial updates | Self-hostable single JS file; no build step; pairs naturally with server-rendered Go |
| github.com/go-chi/chi/v5 | v5.2.5 (already in go.mod) | HTTP router | Already present; more ergonomic than `http.ServeMux` for path params; clean middleware API |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| embed (stdlib) | Go 1.16+ | Embed static files into binary | All static assets: htmx.min.js, style.css, keys.js |
| net/http (stdlib) | — | HTTP server foundation | chi wraps it; use http.FileServer + fs.Sub for static serving |

### What Is NOT Needed
| Not Needed | Why |
|------------|-----|
| `go-htmx` / `htmx-go` helper libs | Thin wrappers around header strings; add dep cost without meaningful value; just write `r.Header.Get("HX-Request") == "true"` inline |
| Tailwind, Bootstrap | Locked decision: custom CSS only |
| Any JS bundler | HTMX is a single vendored file; keys.js is hand-written |
| `nosurf` / `gorilla/csrf` | CSRF deferred to Phase 7 per STATE.md |

**Installation (new dependency):**
```bash
go get github.com/a-h/templ@v0.3.1001
```

HTMX is downloaded once and vendored as a static file (do not use CDN):
```bash
curl -Lo internal/web/static/htmx.min.js \
  https://unpkg.com/htmx.org@2.0.8/dist/htmx.min.js
```

---

## Architecture Patterns

### Recommended Package Structure
```
internal/web/
├── static/
│   ├── htmx.min.js     # vendored HTMX 2.0.8 (no CDN)
│   ├── style.css       # single hand-written Solarized Dark CSS
│   └── keys.js         # keyboard shortcut handler
├── templates/
│   ├── layout.templ    # base HTML shell with navbar + account filter
│   ├── dashboard.templ # dashboard page component
│   ├── aggregate.templ # aggregate table + drill-down breadcrumbs
│   ├── messages.templ  # message list with pagination
│   ├── message.templ   # message detail (headers + plain text body)
│   ├── search.templ    # search input + results
│   └── deletions.templ # deletion batch list
├── embed.go            # //go:embed static/* templates (generated _templ.go)
├── server.go           # Server struct, chi router setup, Start()
├── handlers.go         # handlers struct with all page handlers
├── params.go           # parseViewType, parseSortField, parseMessageFilter (extracted from old handlers.go)
└── middleware.go       # loggingMiddleware, recoveryMiddleware (keep as-is, drop corsMiddleware)
```

Generated files committed alongside source:
```
internal/web/templates/
├── layout_templ.go
├── dashboard_templ.go
├── aggregate_templ.go
├── messages_templ.go
├── message_templ.go
├── search_templ.go
└── deletions_templ.go
```

### Pattern 1: Base Layout with Children (Templ Composition)

Every page handler renders the full layout, passing page content as a `templ.Component` child.

```templ
// internal/web/templates/layout.templ
package templates

templ Layout(title string, accounts []query.AccountInfo, activeAccount *int64, pendingDeletions int) {
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8"/>
        <title>{ title } — msgvault</title>
        <link rel="stylesheet" href="/static/style.css"/>
    </head>
    <body>
        <nav class="navbar">
            <a href="/" class="nav-link">Dashboard</a>
            <a href="/messages" class="nav-link">Messages</a>
            <a href="/aggregate" class="nav-link">Aggregate</a>
            <a href="/search" class="nav-link">Search</a>
            <a href="/deletions" class="nav-link">
                Deletions
                if pendingDeletions > 0 {
                    <span id="deletion-badge" class="badge">{ strconv.Itoa(pendingDeletions) }</span>
                }
            </a>
            <select
                id="account-filter"
                hx-get="/filter/account"
                hx-trigger="change"
                hx-target="body"
                hx-select="main"
                hx-swap="outerHTML"
                hx-replace-url="true"
                name="sourceId">
                <option value="">All accounts</option>
                for _, acc := range accounts {
                    <option value={ strconv.FormatInt(acc.ID, 10) }>{ acc.Identifier }</option>
                }
            </select>
        </nav>
        <main id="main-content">
            { children... }
        </main>
        <script src="/static/htmx.min.js"></script>
        <script src="/static/keys.js"></script>
    </body>
    </html>
}
```

```go
// Handler renders full page — HTMX extracts fragment via hx-select
func (h *handlers) dashboard(w http.ResponseWriter, r *http.Request) {
    stats, _ := h.engine.GetTotalStats(r.Context(), query.StatsOptions{})
    accounts, _ := h.engine.ListAccounts(r.Context())
    // ...build page data...

    page := templates.Layout("Dashboard", accounts, nil, pendingCount)
    content := templates.Dashboard(stats, topSenders, topDomains)

    // Always render full page; HTMX uses hx-select="#main-content" to extract partial
    component := templ.WithChildren(r.Context(), content)
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    page.Render(r.Context(), w)
}
```

### Pattern 2: hx-select for Partials (No Fragment Endpoints Needed)

Locked decision: handlers always return full HTML. HTMX requests use `hx-select="#main-content"` to extract only the content region. This means zero duplicate handler logic.

```html
<!-- In navbar: clicking a nav link does a full render, HTMX extracts #main-content -->
<a href="/aggregate"
   hx-get="/aggregate"
   hx-select="#main-content"
   hx-target="#main-content"
   hx-swap="outerHTML"
   hx-push-url="true"
   class="nav-link">Aggregate</a>
```

### Pattern 3: hx-swap-oob for Deletion Badge

When a message is staged for deletion, the response includes the updated badge count as an out-of-band element at the top level of the response.

```templ
// Included in stageDeletion handler response alongside the row update
templ DeletionBadgeOOB(count int) {
    <span id="deletion-badge" hx-swap-oob="true">
        if count > 0 {
            <span class="badge">{ strconv.Itoa(count) }</span>
        }
    </span>
}
```

**Critical constraint**: OOB elements must be root-level siblings in the response. The staging handler returns both the row update AND the badge update in one response body.

### Pattern 4: Static File Serving with go:embed

```go
// internal/web/embed.go
package web

import (
    "embed"
    "io/fs"
)

//go:embed static/*
var staticFS embed.FS

func staticSubFS() fs.FS {
    sub, _ := fs.Sub(staticFS, "static")
    return sub
}
```

```go
// In server.go chi router registration
r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS()))))
```

**Note**: `all:static` variant only needed if static files start with `.` or `_` — unlikely here. Plain `static/*` is correct.

### Pattern 5: Account Filter Propagation

Account filter is a URL query param (`?sourceId=N`). All handlers read `r.URL.Query().Get("sourceId")`. The account dropdown in the navbar triggers an HTMX GET to the current page URL with the new `sourceId` param. The handler re-renders the page with that filter applied, and `hx-replace-url="true"` keeps the URL bookmarkable.

This is pure server-side state — no client-side state management needed.

### Pattern 6: Pagination with hx-replace-url

```templ
templ Pagination(baseURL string, offset, limit int, total int64) {
    <div class="pagination">
        <span>Showing { fmt.Sprintf("%d-%d of %s", offset+1, min(offset+limit, int(total)), formatNum(total)) }</span>
        if offset > 0 {
            <a href={ templ.SafeURL(fmt.Sprintf("%s&offset=%d", baseURL, offset-limit)) }
               hx-get={ fmt.Sprintf("%s&offset=%d", baseURL, offset-limit) }
               hx-select="#main-content"
               hx-target="#main-content"
               hx-replace-url="true">Prev</a>
        }
        if int64(offset+limit) < total {
            <a href={ templ.SafeURL(fmt.Sprintf("%s&offset=%d", baseURL, offset+limit)) }
               hx-get={ fmt.Sprintf("%s&offset=%d", baseURL, offset+limit) }
               hx-select="#main-content"
               hx-target="#main-content"
               hx-replace-url="true">Next</a>
        }
    </div>
}
```

### Pattern 7: Debounced Search

```templ
templ SearchInput(q string) {
    <input
        type="text"
        name="q"
        value={ q }
        placeholder="Search emails (e.g., from:alice subject:meeting)"
        hx-get="/search"
        hx-trigger="input changed delay:500ms, keyup[key=='Enter']"
        hx-select="#search-results"
        hx-target="#search-results"
        hx-replace-url="true"
        hx-indicator="#search-indicator"
        class="search-input"
    />
    <span id="search-indicator" class="htmx-indicator">Searching...</span>
    <div id="search-results">
        <!-- Results rendered here -->
    </div>
}
```

### Pattern 8: Keyboard Shortcuts (Pure Vanilla JS)

Keyboard shortcuts are implemented in `static/keys.js`, a small plain JS file embedded in the binary. It uses `document.addEventListener('keydown', ...)` and manipulates `data-focused-row` on the table. HTMX row activation happens by calling `htmx.trigger()` or navigating `window.location`.

```javascript
// static/keys.js — conceptual structure
(function() {
    let currentRow = -1;

    document.addEventListener('keydown', function(e) {
        // Ignore if focused in input/textarea
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
            if (e.key === 'Escape') { e.target.blur(); }
            return;
        }

        switch(e.key) {
            case 'j': case 'ArrowDown': moveRow(1); break;
            case 'k': case 'ArrowUp':  moveRow(-1); break;
            case 'Enter': activateRow(); break;
            case 'Escape': window.history.back(); break;
            case '/': focusSearch(); e.preventDefault(); break;
            case '?': toggleHelp(); break;
            case 's': cycleSortField(); break;
            case 'r': reverseSortDir(); break;
            case 't': navigateToTimeView(); break;
        }
    });

    function moveRow(delta) {
        const rows = document.querySelectorAll('[data-row]');
        if (!rows.length) return;
        rows[currentRow]?.classList.remove('row-focused');
        currentRow = Math.max(0, Math.min(rows.length - 1, currentRow + delta));
        rows[currentRow].classList.add('row-focused');
        rows[currentRow].scrollIntoView({ block: 'nearest' });
    }

    function activateRow() {
        const row = document.querySelector('.row-focused');
        if (!row) return;
        const href = row.dataset.href;
        if (href) htmx.ajax('GET', href, { target: '#main-content', select: '#main-content', swap: 'outerHTML' });
    }
})();
```

### Anti-Patterns to Avoid

- **Separate fragment endpoints**: The locked decision is `hx-select` pattern — always render full pages. Don't add `/htmx/messages` routes alongside `/messages`.
- **Inline `text/template` in handlers**: All HTML must go through Templ components. No `fmt.Fprintf(w, "<div>...")` anywhere.
- **Checking `HX-Request` to conditionally render layout**: With `hx-select`, the handler never needs to know if it's an HTMX request — always render the full layout.
- **Committing `node_modules` or `web/` directory**: The entire `web/` tree goes, per FOUND-03.
- **Using `all:static` glob when not needed**: Plain `static/*` suffices; `all:` prefix adds dotfiles.
- **Forgetting to commit `_templ.go` files**: If `_templ.go` files are gitignored or forgotten, `go build` breaks without the CLI. These MUST be committed.
- **Deletion OOB element not at response root**: `hx-swap-oob` elements must be at the top level of the HTTP response body. Nesting inside the main fragment causes HTMX to silently ignore them.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTML template compilation | Custom text/template wrappers, string concatenation | Templ (`github.com/a-h/templ`) | Type-safe, XSS-safe by default, compiles to Go code |
| Partial page updates | Manual JSON APIs + fetch() calls | HTMX `hx-get` / `hx-select` | Zero JS required; back button works; bookmarkable |
| Out-of-band DOM updates | Custom JS event bus, WebSocket | HTMX `hx-swap-oob` | Declarative, stable, no custom protocol |
| Binary static file embedding | Custom file server, reading from disk at runtime | `go:embed` | Compile-time, zero runtime cost, single binary |
| Debounced search | Custom JS debounce function | `hx-trigger="input changed delay:500ms"` | Built into HTMX; no custom JS |

**Key insight:** HTMX replaces the entire React+JSON API layer. The JSON API (`internal/api/`) is not deprecated for lack of use — it's architecturally superseded. Every React component that made a `fetch()` call to `/api/v1/...` is replaced by a Templ component that renders the data server-side.

---

## Common Pitfalls

### Pitfall 1: _templ.go Files Not Committed
**What goes wrong:** `go build` succeeds locally (templ CLI installed) but fails in CI or for contributors without templ CLI.
**Why it happens:** Developers run `templ generate` locally, gitignore the output, or forget to `git add` the `_templ.go` files.
**How to avoid:** After any change to `.templ` files, run `templ generate` and explicitly `git add internal/web/templates/*_templ.go`. Consider a CI check (`templ generate && git diff --exit-code`) if CI is ever set up.
**Warning signs:** CI build failure mentioning undefined symbols like `dashboard_templ`.

### Pitfall 2: hx-swap-oob Element Not at Response Root
**What goes wrong:** Deletion badge doesn't update when a message is staged. No error shown.
**Why it happens:** The OOB element was nested inside the target swap region, not a root sibling.
**How to avoid:** The staging handler response must look like:
```html
<tr id="row-42" class="staged">...</tr>
<span id="deletion-badge" hx-swap-oob="true"><span class="badge">3</span></span>
```
Both the row update and the OOB badge must be root-level siblings.
**Warning signs:** Badge count never changes after staging; browser dev tools show `hx-swap-oob` attribute present but swap not occurring.

### Pitfall 3: Account Dropdown Doesn't Preserve Filter Across Page Transitions
**What goes wrong:** User selects account, navigates to a page, account resets to "All accounts".
**Why it happens:** Account filter is URL state (`?sourceId=N`) that must be round-tripped through every link and form action.
**How to avoid:** Every page handler reads `sourceId` from the request; every internal link includes the current `sourceId` as a query param. The account dropdown in the layout re-selects the correct option by comparing `acc.ID` with the parsed `sourceId` from the request.
**Warning signs:** Account dropdown shows "All accounts" on page load even though `?sourceId=2` is in the URL.

### Pitfall 4: Makefile `build` Still Calls `web-build`
**What goes wrong:** `go build` works but `make build` fails without Node.js.
**Why it happens:** Old Makefile has `build: web-build` as a prerequisite.
**How to avoid:** Remove `web-build` from `build` target prerequisites. Add a `templ-generate` target that is a dev-only prerequisite, not part of the standard build.
**Warning signs:** `make build` output includes "npm run build"; CI failures on machines without Node.

### Pitfall 5: Using `corsMiddleware` With Server-Rendered UI
**What goes wrong:** CORS headers are set unnecessarily; `corsMiddleware` code kept from old API-serving days.
**Why it happens:** `middleware.go` is copied from the old package without removing `corsMiddleware`.
**How to avoid:** Do not register `corsMiddleware` in `applyMiddleware()`. Same-origin requests from the browser to localhost don't need CORS. Drop the `--dev` flag and `corsMiddleware` function entirely.

### Pitfall 6: `go:embed` Path Must Be Relative to Source File
**What goes wrong:** Build error: "pattern static/\*: no matching files found".
**Why it happens:** The embed directive path is relative to the Go source file containing it, not the module root.
**How to avoid:** `embed.go` must be in the same directory as the `static/` folder it embeds. If `embed.go` is at `internal/web/embed.go`, then `static/` must be at `internal/web/static/`.
**Warning signs:** `go build` compilation error mentioning `no matching files found` in the embed pattern.

### Pitfall 7: templ URL Safety — `templ.SafeURL` Required
**What goes wrong:** XSS via crafted URL parameters in anchor hrefs. Or: templ compilation error "cannot use string as templ.SafeURL".
**Why it happens:** Templ distinguishes `string` from `templ.SafeURL` for href attributes. Dynamic URLs must be explicitly wrapped.
**How to avoid:** Any `href` attribute built from server-side data must use `templ.SafeURL(url)`. Pagination links, drill-down links, and breadcrumb links all need this.
**Warning signs:** Templ generate error "cannot use expression of type string as value of type templ.SafeURL".

---

## Code Examples

Verified patterns from official sources and codebase analysis:

### Rendering a Templ Component in a Go Handler
```go
// Source: pkg.go.dev/github.com/a-h/templ
func (h *handlers) dashboard(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    stats, err := h.engine.GetTotalStats(ctx, query.StatsOptions{})
    if err != nil {
        h.renderError(w, r, http.StatusInternalServerError, "stats failed")
        return
    }

    w.Header().Set("Content-Type", "text/html; charset=utf-8")

    // Build full page — HTMX extracts #main-content via hx-select
    component := templates.DashboardPage(stats, ...)
    if err := templates.Layout("Dashboard", ..., component).Render(ctx, w); err != nil {
        h.logger.Error("render failed", "err", err)
    }
}
```

### Static File Serving with chi + go:embed
```go
// Source: go.dev/pkg/embed, chi fileserver example
r.Handle("/static/*",
    http.StripPrefix("/static/",
        http.FileServer(http.FS(staticSubFS()))))
```

### Debounced Search Trigger (HTMX)
```html
<!-- Source: htmx.org/examples/active-search/ -->
<input type="text" name="q"
    hx-get="/search"
    hx-trigger="input changed delay:500ms, keyup[key=='Enter']"
    hx-target="#search-results"
    hx-select="#search-results"
    hx-replace-url="true"
    hx-indicator="#search-loading" />
```

### Out-of-Band Badge Update
```go
// Source: htmx.org/examples/update-other-content/
// Handler for POST /deletions/stage:
func (h *handlers) stageDeletion(w http.ResponseWriter, r *http.Request) {
    // ... create manifest ...

    pending, _ := h.deletions.ListPending()

    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    // Render the row update (primary target) + badge OOB (secondary update)
    templates.StagedRowUpdate(msgID).Render(r.Context(), w)
    templates.DeletionBadgeOOB(len(pending)).Render(r.Context(), w)
}
```

### Templ Layout with Children
```templ
// Source: templ.guide/syntax-and-usage/template-composition/
templ Layout(title string) {
    <!DOCTYPE html>
    <html>
    <head><title>{ title }</title></head>
    <body>
        <nav>...</nav>
        <main id="main-content">
            { children... }
        </main>
    </body>
    </html>
}

// Usage in Go:
// @Layout("Dashboard") {
//     @DashboardContent(data)
// }
```

### chi Router with Middleware
```go
// Source: go-chi.io, existing internal/web/server.go pattern
r := chi.NewRouter()
r.Use(loggingMiddleware(logger))
r.Use(recoveryMiddleware(logger))

r.Get("/", h.dashboard)
r.Get("/messages", h.messages)
r.Get("/messages/{id}", h.messageDetail)
r.Get("/aggregate", h.aggregate)
r.Get("/search", h.search)
r.Get("/deletions", h.deletions)
r.Post("/deletions/stage", h.stageDeletion)
r.Delete("/deletions/{id}", h.cancelDeletion)
r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS()))))
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| React SPA + JSON REST API | Templ server-rendered + HTMX partials | This phase | Eliminates Node.js build dependency, reduces JS bundle to ~50KB (HTMX only) |
| http.ServeMux with pattern matching | chi v5 router | Already in go.mod | More ergonomic path params, clean middleware stack |
| `//go:embed all:dist` (Vite output) | `//go:embed static/*` (hand-crafted assets) | This phase | Zero build step for frontend assets |
| CDN-served HTMX | Vendored `static/htmx.min.js` | This phase | No CDN dependency; binary is fully self-contained |
| CORS middleware for SPA dev | No CORS middleware (same-origin) | This phase | Simplified middleware stack |

**Deprecated/outdated:**
- `embed_dev.go`: The dev build tag pattern (reads from disk at runtime) is no longer needed — all assets are vendored and committed. If a dev mode is desired in future, it can be re-added, but it's not needed for Phase 6.
- `corsMiddleware`: No longer needed; server-rendered apps make same-origin requests.
- `writeJSON` / `apiResponse` / `readJSON`: API response helpers go away with `internal/api/`. The new handlers write HTML, not JSON.

---

## Open Questions

1. **Message detail body rendering in Phase 6**
   - What we know: `MessageDetail.BodyHTML` and `BodyText` are available. Email HTML sanitization (bluemonday) and sandboxed iframe rendering are Phase 7 (RENDER-01, RENDER-02).
   - What's unclear: Should Phase 6 show raw plain text only, or attempt a stripped/safe HTML render?
   - Recommendation: Render `BodyText` only in Phase 6. Show a "HTML rendering available in v1.1 update" note if `BodyHTML` is non-empty and `BodyText` is empty. This avoids XSS while meeting PARITY-05 (headers, body, attachments present).

2. **Deletion staging UI: per-row vs. per-aggregate**
   - What we know: The React SPA has complex selection UX (checkbox + D key to stage all matching filter). The locked decision says "inline stage via hx-post."
   - What's unclear: How does the user select which rows to stage? The TUI uses Space to toggle individual rows and D/A for bulk ops.
   - Recommendation: For Phase 6, implement per-row "Stage" button (hx-post) in message list and a "Stage all matching filter" button in aggregate drill-down view. Row-level checkbox selection is a Phase 9 polish item.

3. **templ v0.3.1001 vs. latest (post-v0.3.1001)**
   - What we know: v0.3.1001 was published Feb 28, 2026. The project pins this version per STATE.md.
   - What's unclear: There may be newer versions. The pin is locked per project decision.
   - Recommendation: Use v0.3.1001 exactly as pinned. Do not upgrade unless a critical bug affects Phase 6 implementation.

---

## Validation Architecture

> `workflow.nyquist_validation` is absent from `.planning/config.json` — treating as enabled.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib (`testing` package) + `go test` |
| Config file | None — existing `go test -tags fts5 ./...` runs all tests |
| Quick run command | `go test -tags fts5 ./internal/web/... -run TestHandlers` |
| Full suite command | `go test -tags fts5 ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FOUND-01 | `go build` succeeds with no npm installed | smoke | `go build -tags fts5 ./cmd/msgvault` | ✅ (Makefile `build-go`) |
| FOUND-02 | HTTP server serves HTML pages (not JSON) | integration | `go test -tags fts5 ./internal/web/... -run TestHandlersReturnHTML` | ❌ Wave 0 |
| FOUND-03 | `web/` and `internal/api/` directories absent | smoke | `test ! -d web && test ! -d internal/api` (shell) | ❌ Wave 0 (manual verify) |
| FOUND-04 | Static assets served at /static/ | integration | `go test -tags fts5 ./internal/web/... -run TestStaticFiles` | ❌ Wave 0 |
| FOUND-05 | `_templ.go` files present and buildable without CLI | smoke | `go build -tags fts5 ./... && git status --short '*_templ.go'` | ❌ Wave 0 |
| PARITY-01 | Dashboard returns stats HTML | integration | `go test -tags fts5 ./internal/web/... -run TestDashboard` | ❌ Wave 0 |
| PARITY-02 | Aggregate endpoint returns 7 view types | integration | `go test -tags fts5 ./internal/web/... -run TestAggregate` | ❌ Wave 0 |
| PARITY-03 | Messages list returns paginated rows | integration | `go test -tags fts5 ./internal/web/... -run TestMessages` | ❌ Wave 0 |
| PARITY-04 | Search returns results, empty triggers FTS5 fallback | integration | `go test -tags fts5 ./internal/web/... -run TestSearch` | ❌ Wave 0 |
| PARITY-05 | Message detail renders headers and body | integration | `go test -tags fts5 ./internal/web/... -run TestMessageDetail` | ❌ Wave 0 |
| PARITY-06 | Staging deletion returns OOB badge update | integration | `go test -tags fts5 ./internal/web/... -run TestStageDeletion` | ❌ Wave 0 |
| PARITY-07 | keys.js served, keyboard shortcuts fire | manual-only | N/A — browser keyboard interaction not automatable without Playwright | — |
| PARITY-08 | sourceId param filters all views | integration | `go test -tags fts5 ./internal/web/... -run TestAccountFilter` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go build -tags fts5 ./cmd/msgvault` (ensures binary compiles)
- **Per wave merge:** `go test -tags fts5 ./...` (full suite)
- **Phase gate:** Full suite green + manual browser smoke test before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/handlers_test.go` — covers FOUND-02, FOUND-04, PARITY-01 through PARITY-06, PARITY-08
- [ ] Test helper: `httptest.NewServer` with mock `query.Engine` (interface already defined)
- [ ] No new framework install needed — Go stdlib `testing` + `net/http/httptest` is sufficient

---

## Sources

### Primary (HIGH confidence)
- `pkg.go.dev/github.com/a-h/templ` — Component interface, Handler types, Render API, version confirmation (v0.3.1001)
- `templ.guide/syntax-and-usage/basic-syntax/` — Templ file syntax, package declaration, component definition
- `templ.guide/syntax-and-usage/template-composition/` — Children pattern, layout composition
- `templ.guide/core-concepts/template-generation/` — `templ generate` workflow, committing `_templ.go` files
- `pkg.go.dev/embed` — `go:embed` directives, `all:` prefix, `fs.Sub`, `http.FileServer` integration
- `htmx.org/attributes/hx-swap/` — hx-swap options (innerHTML, outerHTML)
- `htmx.org/examples/update-other-content/` — hx-swap-oob pattern for badge updates
- `htmx.org/examples/keyboard-shortcuts/` — `hx-trigger` with `from:body` for global keyboard shortcuts
- Existing `internal/web/server.go`, `handlers.go`, `middleware.go` — Reusable handler struct, param parsing, middleware patterns (codebase analysis)
- Existing `internal/query/engine.go`, `models.go` — All Engine interface methods and data models available for handlers

### Secondary (MEDIUM confidence)
- `templ.guide/developer-tools/cicd/` — Verified: committing `_templ.go` is the official pattern for CLI-free builds
- `htmx.org/docs/` — Verified: `hx-select`, `hx-replace-url`, `hx-trigger` delay syntax all confirmed in HTMX 2.0 docs
- WebSearch: HTMX 2.0.8 is current stable release as of March 2026

### Tertiary (LOW confidence)
- WebSearch: chi `http.StripPrefix` + `http.FileServer` static serving pattern — verified against chi GitHub examples but not fetched directly

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — templ v0.3.1001 pinned by project decision; chi already in go.mod; HTMX version confirmed by official release post
- Architecture: HIGH — patterns derived from official templ docs + codebase analysis of existing `internal/web/` package
- Pitfalls: HIGH — OOB constraint verified in HTMX docs; go:embed path pitfall is documented stdlib behavior; other pitfalls derived from codebase analysis

**Research date:** 2026-03-10
**Valid until:** 2026-04-10 (templ is active development; HTMX 2.x is stable)
