# Project Research Summary

**Project:** msgvault — Web UI Rebuild (Templ + HTMX)
**Domain:** Server-rendered Go web UI replacing React SPA in offline email archive tool
**Researched:** 2026-03-10
**Confidence:** HIGH

## Executive Summary

This is a milestone rebuild, not a greenfield project. msgvault already has a working React SPA (`web/`) and a JSON API (`internal/api/`). The objective is to replace both with a server-rendered Templ + HTMX implementation that produces a single `go build` binary with no Node.js, no npm, and no CDN dependencies. PR #176 from the upstream repo (`sarcasticbird/feature-templ-ui`) provides the foundation — 7 pages of working Templ + HTMX UI — and the fork must adopt it while adding two genuinely new features: thread view with collapsible messages and inline attachment rendering. The recommended approach is to wholesale replace `internal/web/` using a directory-copy strategy rather than cherry-picking individual commits, then layer fork-specific additions on top.

The recommended stack additions are minimal and well-justified: `github.com/a-h/templ v0.3.1001` for type-safe Go templates, `github.com/microcosm-cc/bluemonday v1.0.26` for server-side HTML sanitization, and HTMX 2.0.4 vendored as a static file. All other stack elements (Go, chi/v5, DuckDB/Parquet, SQLite) are unchanged. The architecture is a clean handler struct pattern calling `query.Engine` (DuckDB) for aggregates/search and `store.Store` (SQLite PK lookup) for message bodies — no intermediate JSON API layer. The `deletion.Manager` integration is identical to the current implementation; only the HTTP verbs change from JSON to HTML form posts.

The critical risks concentrate in two areas. First, email HTML security: the templ auto-escape safety net is easily bypassed with `templ.HTML()` cast, and email bodies rendered without bluemonday sanitization + sandboxed iframe isolation create stored XSS vectors and CSS collision that destroys application layout. These mitigations are non-negotiable and must be established in Phase 1 before any email body rendering is attempted. Second, the PR #176 adoption strategy: cherry-picking individual commits produces compound conflicts between the fork's `store.Store`-augmented handlers and PR #176's `query.Engine`-only handlers. The directory-copy approach is slower upfront but avoids unresolvable conflicts.

## Key Findings

### Recommended Stack

The rebuild requires only three new dependencies added to an existing, stable Go codebase. `github.com/a-h/templ v0.3.1001` replaces `html/template` — it is the only Go library where templates compile to typed Go functions with no reflection, enabling component composition and compile-time type checking. HTMX 2.0.4 is vendored directly into `internal/web/static/` via `go:embed`, eliminating all CDN and runtime dependencies. `github.com/microcosm-cc/bluemonday v1.0.26` provides HTML sanitization at render time, server-side, before content reaches the browser.

The critical toolchain constraint is that `templ` CLI version must exactly match the `github.com/a-h/templ` runtime version in `go.mod` — version drift causes silent `_templ.go` compilation failures. Both must be pinned to `v0.3.1001`. Generated `_templ.go` files must be committed to the repository so `go build` works without the `templ` CLI installed (required for a public open source tool).

**Core technologies:**
- `github.com/a-h/templ v0.3.1001`: type-safe Go HTML templates — compile-time safety, no reflection, component composition
- HTMX 2.0.4 (vendored): partial HTML updates via HTTP attributes — no npm, no JS framework, embedded via `go:embed`
- `github.com/microcosm-cc/bluemonday v1.0.26`: server-side HTML sanitization — stops XSS from email bodies before browser receives them
- `github.com/go-chi/chi/v5 v5.2.5` (existing): HTTP router — already in `go.mod`, used by PR #176, no change

### Expected Features

The React SPA defines the baseline. Every page (dashboard, browse/aggregate, messages, search, message detail, deletions) must reach parity. The two features that make this milestone worth the rebuild investment are thread view and inline attachments — both require server-side capabilities (CID substitution, content-type routing, bluemonday post-CID-substitution) that the React SPA handled in browser JS.

**Must have (table stakes — parity with existing React SPA):**
- Dashboard stats overview with time series chart — no Recharts equivalent; CSS bar chart or server-side SVG
- Message list with sort/filter and offset pagination — pure HTML, `hx-replace-url` for bookmarkability
- Aggregate browse with 7 view types and drill-down — most complex HTMX pattern; all filter state in URL params
- Full-text search with 500ms debounce — `hx-trigger="input changed delay:500ms"`, DuckDB fast path + FTS5 fallback
- Message detail with HTML email body (sandboxed iframe + bluemonday) — security prerequisite
- Deletion staging and management — HTMX POST/DELETE with OOB swap for badge count
- Vim-style keyboard navigation via `keys.js` (from PR #176)
- Multi-account filtering throughout all views

**Should have (new in this milestone):**
- Thread view with collapsible messages — native `<details>`/`<summary>` HTML, last message pre-expanded
- Inline attachment rendering (images inline, other files as download) — CID substitution server-side before bluemonday
- External image blocking by default — CSS `img[src^="http"] { display: none }` with opt-in toggle
- Text/HTML body toggle with URL persistence
- Go-side HTML sanitization with email-safe bluemonday policy

**Defer (v2+):**
- JSON API — only needed when MCP integration or mobile client work begins
- App-level encryption — separate milestone, noted in PROJECT.md as not-yet-implemented
- Server-side SVG time-series chart — CSS bar chart sufficient to validate approach first

### Architecture Approach

The architecture is a single `internal/web/` package containing a `handlers` struct injected with `query.Engine` (DuckDB over Parquet for aggregates/lists/search), `store.Store` (SQLite direct for message body PK lookups), and `deletion.Manager`. Templates live in `internal/web/templates/` as `.templ` files compiled to `_templ.go`. Static assets (HTMX, CSS, `keys.js`) are embedded from `internal/web/static/` via `go:embed`. The existing `internal/api/` JSON API package and `web/` React SPA are deleted entirely. HTMX uses `hx-select` pattern (Option A) for partial updates — handlers always return full pages and HTMX extracts the target fragment client-side, avoiding server-side `HX-Request` header conditional logic that breaks browser back button.

**Major components:**
1. `internal/web/server.go` — chi router, middleware stack, route registration, static file serving
2. `internal/web/handlers*.go` — page handlers (dashboard, browse, messages, message_detail, thread, deletions, attachments), all methods on `handlers` struct
3. `internal/web/templates/*.templ` — type-safe Templ components compiled to `_templ.go`, including `helpers.go` with bluemonday sanitization
4. `internal/web/static/` — vendored HTMX, Solarized CSS, `keys.js`, all embedded via `go:embed`
5. `query.Engine` (unchanged) — DuckDB over Parquet for all aggregate/list/search queries
6. `store.Store` (new web usage) — SQLite PK lookup for message body access only; never JOINed in list queries

### Critical Pitfalls

1. **Raw email HTML bypassing sanitization via `templ.HTML()`** — Always run bluemonday with email-safe policy before any `templ.HTML()` cast. Order must be: charset decode → bluemonday sanitize → `templ.HTML()`. Never reverse. Establish the `sanitizeHTML` helper in `templates/helpers.go` in Phase 1 before any template touches email bodies.

2. **Cherry-pick PR #176 creating unresolvable conflicts** — The fork's `internal/web/` (React SPA + `store.Store` handlers) and PR #176's `internal/web/` (Templ, `query.Engine` only) conflict at every file. Use directory-copy strategy: delete old `internal/web/` and `internal/api/`, commit the deletion, copy PR #176's directory wholesale, then manually re-apply fork-specific additions. Attempting commit-by-commit cherry-pick costs a day of conflict resolution.

3. **Templ CLI version mismatch blocking `go build`** — Pin `TEMPL_VERSION := v0.3.1001` in Makefile before first build attempt. Use `go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)` in Makefile targets, not `templ` global. Version mismatch produces cryptic `undefined: templ.JoinURLErrs` failures.

4. **HTMX back button returning unstyled fragments** — Never use `HX-Request` header to choose between returning data vs. no data. Use it only to choose between full layout wrapper vs. fragment wrapper. Every URL pushed via `hx-push-url` must return a complete page on direct access. Use `hx-replace-url` for state changes that should not create history entries (sort toggles, search input).

5. **Email HTML breaking application CSS/layout** — Sanitization prevents XSS but not CSS collision. Email HTML uses `<table>` layouts, `!important` inline styles, and `font-size` declarations that destroy the Solarized theme. Render all HTML email bodies in `<iframe srcdoc="...">` with `sandbox="allow-popups allow-popups-to-escape-sandbox"`. Never `allow-scripts` + `allow-same-origin` together — that combination defeats the sandbox entirely.

6. **OOB HTMX swaps silently dropping elements** — HTMX discards OOB swaps with no error if the target ID doesn't exist in the DOM. Define all OOB swap target IDs in a constants section of `helpers.go`, not as string literals in multiple template files. Test deletion badge update after first, second, and third staging action.

## Implications for Roadmap

Based on research, the rebuild has clear phase dependencies driven by: (1) security prerequisites must precede content rendering, (2) PR #176 adoption must precede any new feature work, (3) the most complex HTMX patterns (aggregate drill-down, deletion HTMX flows) should be validated before building new features on top.

### Phase 1: PR #176 Adoption and Foundation

**Rationale:** Everything else depends on having the Templ + HTMX structure in place. The cherry-pick/directory-copy decision must be made and executed first. Version pinning and the `sanitizeHTML` helper must be established before any template touches email bodies. This phase has the highest integration risk (Pitfall 5) and sets the constraint for all subsequent phases.

**Delivers:** Working Templ + HTMX binary with `go build` producing a single binary (no npm), all 7 PR #176 pages serving, `keys.js` keyboard navigation, Solarized theme, bluemonday helper established in `helpers.go`, templ CLI version pinned in Makefile.

**Addresses (from FEATURES.md):** Dashboard, browse, messages, search, message detail (plain text fallback), deletions — feature parity with React SPA minus HTML email body rendering.

**Avoids (from PITFALLS.md):** Pitfall 3 (templ version mismatch), Pitfall 5 (cherry-pick conflicts), Pitfall 4 (HTMX back button patterns audited before new patterns added).

### Phase 2: HTML Email Body Rendering

**Rationale:** This is the security-critical phase. Email HTML rendering requires bluemonday sanitization + sandboxed iframe — both must be in place before any HTML body reaches a browser. CID image substitution must precede bluemonday (or bluemonday strips the local attachment `src` URLs). Phase 1's `sanitizeHTML` helper is the prerequisite. This phase also covers the attachment handler content-type routing and CSP headers needed for inline images.

**Delivers:** Message detail page renders HTML email bodies in sandboxed iframe with bluemonday sanitization, CID references resolved to local attachment URLs, external images blocked by default, inline images served with correct content-type and CSP headers.

**Uses (from STACK.md):** `bluemonday.UGCPolicy()` with email policy tightening (strip `<style>`, `http/https`-only `href`, block event attributes), `@templ.Raw()` post-sanitization, `Content-Security-Policy: script-src 'none'` on attachment inline endpoint.

**Implements (from ARCHITECTURE.md):** `handlers_attachments.go` with content-type routing, `sanitizeHTML` function used in `message_detail.templ` and `thread.templ`, `store.Store.GetMessageBody` called via PK lookup only.

**Avoids (from PITFALLS.md):** Pitfall 1 (raw HTML bypassing sanitization), Pitfall 2 (email CSS breaking app layout), MIME sniffing on attachment endpoint.

### Phase 3: Thread View

**Rationale:** Thread view depends on inline attachment rendering being complete (thread bodies contain CID images — building thread without it creates a degraded experience requiring a second pass). It also depends on the message detail page navigation pattern being established (thread link originates from message detail). Thread view uses existing `query.Engine.ListMessages` with `ConversationID` filter — no new Engine methods needed.

**Delivers:** `/messages/{id}/thread` route serving full conversation chronologically, last message pre-expanded via `<details open>`, earlier messages collapsible via native HTML `<details>`/`<summary>` (no JS), lazy-loading of message bodies on first expand via `hx-trigger="toggle once"`.

**Addresses (from FEATURES.md):** Thread view with collapsible messages (P1 differentiator), inline attachment rendering in thread context (P1 differentiator).

**Implements (from ARCHITECTURE.md):** `handlers_thread.go` (NEW), `thread.templ` + `thread_templ.go` (NEW), data flow: `engine.GetMessage` → `engine.ListMessages(ConversationID filter)` → `store.GetMessageBody × N` → `templates.Thread`.

### Phase 4: Polish and Validation

**Rationale:** After core feature parity and new features are implemented, the remaining P2 items (text/HTML toggle, external image opt-in, loading indicators) and the "looks done but isn't" checklist from PITFALLS.md need systematic validation. The dashboard chart gap (no Recharts equivalent) needs a decision: CSS bar chart (simpler) vs. server-side SVG (more capable).

**Delivers:** Text/HTML body toggle with URL persistence, "Load external images" opt-in, loading indicators via `hx-indicator`, dashboard time-series chart (CSS or SVG approach), all items on the PITFALLS.md verification checklist confirmed, React SPA artifacts (`web/`, `package.json`, `node_modules/`, npm Makefile targets) confirmed deleted.

**Addresses (from FEATURES.md):** All P2 items, anti-feature avoidance confirmed (no infinite scroll, no session auth, no JSON API).

### Phase Ordering Rationale

- Phase 1 must precede all others: structural foundation (Templ + chi routing + go:embed) is a hard prerequisite
- Phase 2 must precede Phase 3: thread view with inline images depends on attachment handler + CID substitution + bluemonday being in place; building thread without HTML body rendering means two passes through the same handler code
- Security prerequisites (bluemonday helper, iframe sandbox) are established at the earliest possible phase (Phase 1 helper + Phase 2 usage) to prevent the non-negotiable pitfalls from being deferred
- Phase 4 validation is intentionally last: it's a checkpoint pass over the full implementation, not a feature phase

### Research Flags

Phases likely needing deeper research during planning:

- **Phase 1:** The PR #176 conflict assessment requires reading the actual diff between PR #176 and the fork's current `internal/web/`. Research should produce a file-by-file conflict map before planning commits to a specific adoption strategy.
- **Phase 2:** Email iframe resizing behavior (how to handle variable-height email bodies without JS) and the exact bluemonday email policy (which tags to allowlist) benefit from testing against real email corpus before implementation.

Phases with standard patterns (skip research-phase):

- **Phase 3:** Thread view data flow is fully specified in ARCHITECTURE.md; `ListMessages` with `ConversationID` filter is already used in the TUI. Implementation is straightforward once Phase 2 attachment infrastructure is in place.
- **Phase 4:** All items are well-defined polish tasks and checklist verification; no unknown domains.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All versions verified against GitHub releases and PR #176 source. Only 3 new dependencies; no version conflicts with existing go.mod. |
| Features | HIGH | React SPA provides definitive ground truth for feature baseline. HTMX patterns sourced from official documentation and confirmed working examples. Dashboard chart approach (no Recharts equivalent) is the only unresolved design decision. |
| Architecture | HIGH | PR #176 is the primary source; existing codebase read directly. Handler struct pattern, `query.Engine` interface, `store.Store` usage all confirmed against actual code. One minor spec discrepancy: spec says `GetThreadMessages` but Engine uses `ListMessages` with `ConversationID` filter — confirmed via codebase read. |
| Pitfalls | HIGH | Templ version mismatch, HTMX back button, and cherry-pick conflict pitfalls sourced from official repos. XSS/iframe patterns sourced from production post-mortems and official MDN. One MEDIUM area: email iframe height/resizing edge cases have limited documentation. |

**Overall confidence:** HIGH

### Gaps to Address

- **Dashboard chart implementation:** No Go equivalent of Recharts. Three options identified (CSS bar chart, server-side SVG template, small vendored charting library) but no decision made. Decision needed before Phase 4 dashboard validation — CSS bar chart is recommended starting point. Validate before committing to SVG generation.
- **Email iframe resizing:** Using `scrolling="no"` with `ResizeObserver` or fixed generous `min-height` with `overflow:auto`. The `postMessage`-based approach works but adds JS complexity. Concrete approach depends on real email corpus testing; document what was chosen and why during Phase 2.
- **bluemonday policy specifics:** The exact email allowlist (which `<font>` attributes, which `<table>` attributes, whether to allow `<style>` at all) needs validation against a representative sample of archived emails. Use bluemonday's `sanitise_html_email` reference implementation as the starting point, not `UGCPolicy()` directly.
- **CSRF protection implementation:** PITFALLS.md flags CSRF on deletion POST handlers as a security requirement. `github.com/justinas/nosurf` or `gorilla/csrf` integrate with chi. Neither is currently in `go.mod`. Decision needed during Phase 2 planning.

## Sources

### Primary (HIGH confidence)
- PR #176 (`sarcasticbird/feature-templ-ui`) source code — go.mod diff, handler structure, generated `_templ.go` files, Makefile targets
- Existing codebase (`internal/web/handlers.go`, `internal/query/engine.go`, `internal/store/store.go`) — direct read
- `github.com/a-h/templ` releases — v0.3.1001 current stable, published 2026-02-28
- `github.com/go-chi/chi` releases — v5.2.5 current stable
- `github.com/microcosm-cc/bluemonday` pkg.go.dev — email sanitization reference implementation
- HTMX official documentation — Active Search, Keyboard Shortcuts, hx-push-url, hx-swap-oob patterns
- templ official documentation — HTMX integration, Fragments, template generation, committed `_templ.go` recommendation
- MDN — iframe sandbox attribute, `allow-scripts` + `allow-same-origin` defeat documented

### Secondary (MEDIUM confidence)
- [Rendering untrusted HTML email safely — Close Engineering](https://making.close.com/posts/rendering-untrusted-html-email-safely/) — sandboxed iframe approach validated
- [Bookmarkable by Design: URL-Driven State in HTMX](https://www.lorenstew.art/blog/bookmarkable-by-design-url-state-htmx/) — hidden form field state propagation
- [Advanced Data Table with HTMX](https://benoitaverty.com/articles/en/data-table-with-htmx) — server-side sort/filter/pagination
- [GoTTH stack learnings — Emily T. Burak](https://emilytburak.net/posts/2025-06-09-htmx-golang-learnings/) — practitioner post-mortem on HTMX Go integration

### Tertiary (LOW confidence)
- HTMX GitHub issues #3037, #3165, #497 — back button/history behaviors (documented but not fully resolved; design must account for them)
- HTMX GitHub issue #2790 — OOB swap outer element stripping (confirmed bug; use `innerHTML` swap strategy as workaround)

---
*Research completed: 2026-03-10*
*Ready for roadmap: yes*
