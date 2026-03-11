# Phase 6: Foundation - Context

**Gathered:** 2026-03-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Replace the React SPA with server-rendered Templ + HTMX, achieving full feature parity across all pages (Dashboard, Messages, Aggregate, Search, Message Detail, Deletions). Produce a single `go build` binary with no npm, no Node.js, and no CDN dependencies. Delete React SPA (`web/`), JSON API server (`internal/api/`), and all npm/Vite artifacts.

Thread view and email rendering are Phase 7-8 — this phase focuses on parity with what the React SPA already provides.

</domain>

<decisions>
## Implementation Decisions

### Visual Design & Theme
- Solarized Dark palette exclusively — no light mode, no toggle
- CSS custom properties for Solarized colors: `--base03: #002b36`, `--base02: #073642`, `--base01: #586e75`, `--base0: #839496`, `--base1: #93a1a1`, `--blue: #268bd2`, `--cyan: #2aa198`, `--green: #859900`, `--red: #dc322f`, `--yellow: #b58900`
- Single hand-written CSS file (~300-500 lines), embedded via `go:embed static/style.css`
- Compact data density — tight rows, small padding, maximize rows on screen (15+ on 1080p)
- No CSS framework (no Tailwind, no Bootstrap) — custom CSS only

### Navigation & Page Flow
- Top navbar with horizontal page links: Dashboard | Messages | Aggregate | Search | Deletions
- Account filter dropdown in navbar (always visible), shows "All accounts" by default, updates current page via HTMX on change
- Breadcrumbs for aggregate drill-down context: `Aggregate > Senders > alice@example.com > Messages`
- Click any breadcrumb level to navigate back; Esc/Backspace goes up one level
- Each drill-down level is a separate URL — back button works naturally

### Keyboard Shortcuts
- Mirror TUI keybindings in `keys.js` (embedded via go:embed):
  - `j/k` or `↑/↓` — navigate rows
  - `Enter` — drill down / open
  - `Esc` — go back one level
  - `Tab` — cycle aggregate view types
  - `s` — cycle sort field
  - `r` — reverse sort direction
  - `t` — jump to Time view
  - `a` — account filter
  - `/` — focus search input
  - `?` — help overlay
  - `q` — no-op in browser (avoid accidental tab close)

### PR #176 Adoption
- Start from PR #176 as base implementation, fill parity gaps
- Directory-copy strategy (not cherry-pick) — already decided in STATE.md
- `templ` CLI pinned to v0.3.1001 — already decided in STATE.md
- Generated `_templ.go` files committed to repo so `go build` works without templ CLI
- HTMX `hx-select` pattern for partials — already decided in STATE.md

### Artifact Cleanup
- Delete `internal/api/` entirely (server.go, handlers.go, middleware.go, tests, serve command)
- Delete `web/` entirely (React SPA, package.json, node_modules, Vite config, tsconfig)
- Normal delete commit (preserve git history, no squash)
- JSON API can be re-added in v2 if MCP/mobile needs arise (deferred as API-01)

### Search Behavior
- Debounced live search: `hx-trigger="input changed delay:500ms, keyup[key=='Enter']"`
- Loading indicator during DuckDB query
- Results replace below the search input (no separate results page)
- Two-tier: DuckDB fast path first, FTS5 deep fallback if no results

### Aggregate Interaction
- Filter bar above aggregate table — debounced input to filter displayed rows (e.g., type "amazon" in Senders view)
- Clickable column headers for sort — click to sort, click again to reverse, arrow indicator shows direction
- Keyboard sort: `s` cycles fields, `r` reverses — matches TUI behavior
- 7 view types preserved: Senders, Sender Names, Recipients, Recipient Names, Domains, Labels, Time

### Deletion Staging
- Inline stage via `hx-post` — no separate confirmation page
- Badge count in navbar updates via `hx-swap-oob` (out-of-band swap) without full page reload
- Staged rows get visual indicator (strikethrough or muted styling)
- Cancel via `hx-delete` with row fade-out animation

### Pagination
- Offset pagination, 50 rows per page
- URL params: `?page=1&limit=50`
- Prev/Next links rendered server-side
- `hx-replace-url` keeps URL bookmarkable
- Total count displayed: "Showing 1-50 of 12,847 messages"

### Dashboard
- Summary stat cards at top: total messages, accounts, total size, date range
- Top 5 senders and top 5 domains lists below stats
- Time-series chart deferred to Phase 9 (POLISH-02) — placeholder or omit for now
- Recent sync info if available

### Claude's Discretion
- Exact CSS specifics (spacing, font sizes, border-radius) within Solarized Dark constraints
- Error page design (404, 500)
- Loading indicator style (spinner vs skeleton vs text)
- Help overlay design for `?` shortcut
- How to handle empty states (no messages, no search results)
- Whether to show message snippet in message list rows

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `query.Engine` interface: All aggregate, list, search, and stats methods — used as-is, no changes needed
- `internal/web/handlers.go`: Parameter parsing helpers (`parseViewType`, `parseSortField`, `parseMessageFilter`) — extract to `params.go`
- `internal/web/middleware.go`: `loggingMiddleware`, `recoveryMiddleware` — reusable, keep as-is. Remove `corsMiddleware` (not needed for server-rendered UI)
- `internal/web/embed.go`: Change `//go:embed all:dist` → `//go:embed static/*`
- `deletion.Manager`: `CreateManifest`, `SaveManifest`, `ListPending/InProgress/Completed/Failed`, `CancelManifest` — all used identically

### Established Patterns
- Handler struct with injected dependencies (`handlers` struct with `query.Engine`, `deletion.Manager`, `logger`)
- `query.Engine` is the fast path (DuckDB/Parquet) for all list/aggregate/search queries
- `store.Store` only for PK lookups (message detail body) — never JOIN in list queries
- Content-addressed attachment storage with hash-based file paths

### Integration Points
- `web.NewServer()` gains `*store.Store` parameter (for message detail body access in later phases)
- `cmd/msgvault/cmd/web.go` already calls `initQueryEngine()` which returns `*store.Store` — just pass it through
- chi router replaces `http.ServeMux` (chi already in go.mod via `internal/api/`)
- Static assets served at `/static/*` via `http.FileServer(http.FS(staticSub))`

</code_context>

<specifics>
## Specific Ideas

- Look and feel should be "GitHub dark mode meets Raycast" — clean, developer-friendly, Solarized Dark
- Tables should feel like the TUI — compact, data-dense, functional over decorative
- Keyboard shortcuts mirror TUI exactly where applicable — users muscle-memory should transfer
- Dashboard stat cards similar to Grafana/monitoring dashboards — numbers prominently displayed

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 06-foundation*
*Context gathered: 2026-03-10*
