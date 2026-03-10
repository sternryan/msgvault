# Feature Research

**Domain:** Server-rendered email archive browser (Templ + HTMX rebuild)
**Researched:** 2026-03-10
**Confidence:** HIGH (existing React SPA provides ground truth; HTMX patterns well-documented)

## Feature Landscape

This is a milestone rebuild, not a greenfield project. The React SPA (`web/src/pages/`) defines the baseline — every page there must reach feature parity in the Templ + HTMX implementation. The two genuinely new features are thread view with inline attachments (partially implemented in React but designated new in the spec).

The question for each feature is not "should we build it" but "how does the server-rendered approach differ from the SPA approach, and what complexity does the migration introduce?"

---

### Table Stakes (Users Expect These)

Features that must exist at parity with the current React SPA. Regression = broken product.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Dashboard stats overview | Entry point — total messages, top senders, time series chart | MEDIUM | Chart requires server-side SVG generation or simple ASCII/CSS bar charts. No Recharts equivalent in pure Go. SVG via template is the path; no JS charting library. |
| Message list with sort/filter | Core data browsing — sort by date, sender, size | LOW | Offset pagination with `?page=N` URL params. `hx-replace-url` keeps URL bookmarkable. Table rows are pure HTML. |
| Aggregate browse with drill-down | 7 views: senders, senderNames, recipients, recipientNames, domains, labels, time | HIGH | The most complex HTMX pattern. Drill-down replaces the aggregate table partial via `hx-target`. All filter state encoded in hidden form fields + `hx-include`. View cycle (`Tab` key) triggers HTMX request with updated `groupBy` param. |
| Full-text search | Users expect instant search across 20+ years of email | MEDIUM | Debounced active search: `hx-trigger="input changed delay:500ms, keyup[key=='Enter']"` → returns results partial. Two-tier: DuckDB fast path, FTS5 deep path. `hx-indicator` shows loading state. |
| Message detail view | Read full message headers, body, attachments list | MEDIUM | No HTMX needed — full page render. HTML body rendered via sandboxed `<iframe srcdoc="...">` with bluemonday-sanitized content. CID image references resolved server-side before iframe injection. |
| Deletion staging and management | Stage messages for deletion, review, cancel, execute | MEDIUM | HTMX pattern: stage via `hx-post=/deletions/stage`, list updates via `hx-target`, confirm/cancel inline. `hx-confirm` for destructive execution action. Row fade-out via CSS `htmx-swapping` class. |
| Vim-style keyboard navigation | Power users expect j/k navigation, shortcut keys | LOW | PR #176 implements this in `keys.js`. Pattern: `hx-trigger="keyup[key=='j'] from:body"` for HTMX-driven navigation; custom JS for DOM-side row focus. Existing `keys.js` vendored alongside `htmx.min.js`. |
| Dark/light theme | Local tool — users run it in their terminal environment | LOW | Solarized theme from PR #176 via CSS variables. `prefers-color-scheme` media query. No JS needed. |
| Multi-account filtering | Users archive multiple Gmail accounts | LOW | Account selector in nav or filter bar. `sourceId` query param threaded through all pages. Hidden form field in aggregate/message views. |
| Breadcrumb navigation | Back-button context in drill-down views | LOW | Pure HTML `<nav>` breadcrumb generated server-side from URL params. No state needed. |
| Pagination | Message list can have millions of rows | LOW | Offset pagination: `?page=N&limit=50` in URL. Prev/Next links rendered server-side. `hx-replace-url` keeps back-button working. Infinite scroll is an anti-feature here (see below). |
| Loading indicators | Debounced search and aggregate drill-down can be slow (DuckDB startup) | LOW | `hx-indicator="#spinner"` pattern. Spinner element shown during HTMX request via `htmx-request` class. |
| Error pages (404, 500) | Missing message ID, DB errors | LOW | Templ error templates. DuckDB unavailable → fallback to SQLite query engine (already exists in `query/sqlite.go`). |

---

### Differentiators (Features Specific to the Server-Rendered Approach)

These are features where the Templ + HTMX approach either enables something the React SPA couldn't do cleanly, or where the rebuild is a meaningful improvement.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Thread view with collapsible messages | Missing from PR #176; new contribution. Displays full conversation chronologically with last message expanded by default. | HIGH | Server-side: `store.Store.GetMessagesByConversationID` or `query.Engine.GetThreadMessages`. Each message is a `<details>` element (native HTML collapsing, no JS) or HTMX-driven expand on click. Last message pre-expanded via template logic. Link from message detail: "View thread (N messages)". |
| Inline attachment rendering in thread + message detail | Images render inline; other files show download link. CID references resolved server-side. | HIGH | CID substitution: `strings.ReplaceAll(html, "cid:"+att.ContentID, "/attachments/"+att.Hash+"/inline")` before bluemonday. Content-type routing in attachment handler: `image/*` → `Content-Disposition: inline`, others → `Content-Disposition: attachment`. CSP header on attachment endpoint: `script-src 'none'`. SHA-256 hash validation on download (content-addressed store already has hashes). |
| Go-side HTML sanitization with bluemonday | Sanitization happens once at render time, not in browser JS. Sanitized HTML is baked into the `srcdoc` attribute of the sandboxed iframe. | MEDIUM | `bluemonday` email policy: allowlist for email-safe tags (`a`, `b`, `blockquote`, `br`, `div`, `em`, `font`, `h1-6`, `hr`, `i`, `img`, `li`, `ol`, `p`, `pre`, `span`, `strong`, `table`, `td`, `th`, `tr`, `ul`). Block `script`, `form`, `input`, `object`. Data URIs blocked by default — acceptable since inline images are served via attachment endpoint. Run bluemonday AFTER CID substitution to avoid stripping local `img src` URLs. |
| External image blocking by default | Privacy-preserving: tracking pixels and external images don't load unless user opts in | MEDIUM | Default CSS in iframe wrapper: `img[src^="http"] { display: none }`. "Load external images" button triggers HTMX request to re-render message detail with a `?load_images=true` param. Server-side: pass flag through to template, omit blocking CSS. |
| Text/HTML body toggle | Some emails render better as plain text; power users want the option | LOW | Two links or a `<select>` that triggers `hx-get` with `?mode=text` or `?mode=html`. Server renders appropriate body variant. `hx-replace-url` preserves mode in URL. |
| URL-driven state for all views | Every view state is bookmarkable and back-button works | LOW | HTMX `hx-push-url` for navigation between distinct views (browse → message detail). `hx-replace-url` for filters, sorts, pagination within a view. All state in query params — no client-side state store. |
| Zero build step deployment | `go build` produces the complete binary including all web assets | LOW | `go:embed` for `htmx.min.js`, `style.css`, `keys.js`. Generated `_templ.go` files committed to repo. `go generate ./...` only needed when editing `.templ` files. |
| `<details>` for collapsible thread messages | Native HTML collapse/expand without JS. Accessible, keyboard-navigable, works without HTMX. | LOW | `<details open>` for last message in thread. Other messages: `<details>` without `open`. HTMX lazy-load of body content on `<details>` toggle via `hx-trigger="toggle"` if body is large. |

---

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Infinite scroll | Feels modern; no pagination clicks | Breaks back-button and bookmarks. HTMX history snapshotting with infinite scroll has known issues (#1188 in htmx repo). Email archives benefit from "I was on page 12" navigation. | Offset pagination with prev/next links and `hx-replace-url`. |
| Client-side state management (Alpine.js, localStorage) | Persistent filter state across sessions | Defeats URL-driven state principle. Two sources of truth. Breaks bookmarkability. Adds JS dependency. | All state in URL query params. Hidden form fields preserve state across HTMX partial updates. |
| Real-time sync status | Users want to know if sync is running | Requires WebSocket or SSE. Adds significant complexity for a personal local tool that users explicitly invoke sync on. | Sync runs in CLI. Web UI shows last sync timestamp from `sync_runs` table. No live updates needed. |
| Client-side sorting | Instant re-sort without server round-trip | Defeats server-side rendering value. Large datasets (millions of rows) can't be client-sorted. | Server-side sort via `?sortField=X&sortDir=Y` params. DuckDB handles this efficiently. |
| Typeahead autocomplete for search | Feels polished | For an archive tool, exact search matters more than suggestions. Adds complexity: suggestion endpoint, client-side debounce, keyboard nav in dropdown. | Debounced active search returns actual result rows, not suggestions. Users see real results, not guesses. |
| Session-based auth / login screen | "Security" | This is a local personal tool running on localhost. Auth adds friction with zero security benefit (attacker with localhost access has the SQLite DB too). | The existing React SPA had auth; it was removed from the Templ spec. Keep it removed. |
| JSON API | Separation of concerns, future mobile client | Templ handlers call `query.Engine` directly — no JSON serialization overhead. API adds ~2,300 LOC (current `internal/api/`) with zero current benefit. | Add JSON API back only when a mobile client or MCP integration actually needs it. |
| Virtualized/windowed list | Smooth scrolling for large message lists | HTMX is not a virtual DOM. Windowing requires JS. Pagination handles the same problem with less complexity and better UX for an archive tool. | Server-side pagination with 50-100 rows per page. DuckDB offset queries are fast. |

---

## Feature Dependencies

```
[Thread View]
    └──requires──> [Message Detail] (thread links from message detail page)
    └──requires──> [Inline Attachment Rendering] (thread shows expanded bodies with images)

[Inline Attachment Rendering]
    └──requires──> [Attachment Handler] (content-type routing, CSP headers)
    └──requires──> [Go-side HTML Sanitization] (CID substitution before bluemonday)
    └──requires──> [Go-side HTML Sanitization] (bluemonday runs after CID substitution)

[Go-side HTML Sanitization]
    └──requires──> [Sandboxed iframe rendering] (sanitized HTML injected into srcdoc)

[External Image Blocking]
    └──enhances──> [Go-side HTML Sanitization] (privacy layer on top of XSS protection)

[Aggregate Drill-Down]
    └──requires──> [URL-Driven State] (filter context encoded in URL params)
    └──enhances──> [Message List] (drill-down navigates to filtered message list)

[Debounced Active Search]
    └──requires──> [Loading Indicators] (search latency can be 100-500ms)

[Vim Keyboard Navigation]
    └──requires──> [keys.js] (from PR #176 — handles DOM row focus; HTMX handles requests)

[Deletion Staging]
    └──requires──> [Message List] (selection happens in message list view)
    └──requires──> [Deletions Page] (staged items reviewed before execution)

[Dashboard Stats Chart]
    └──requires──> [Server-side SVG or CSS bar chart] (no JS charting library available)

[Zero Build Step]
    └──requires──> [go:embed static assets] (htmx.min.js, style.css, keys.js vendored)
    └──requires──> [Committed _templ.go files] (go generate not required for go build)
```

### Dependency Notes

- **Thread View requires Inline Attachment Rendering:** Thread bodies can contain CID images. Building thread view without inline images produces a degraded experience that then requires a second pass to fix. Build them together.
- **Go-side HTML Sanitization must precede inline attachment rendering:** CID references must be substituted with local attachment URLs before bluemonday strips `src` attributes it doesn't recognize. Order: (1) CID substitution, (2) bluemonday, (3) inject into `srcdoc`.
- **Aggregate Drill-Down requires URL-Driven State:** The drill-down filter context (`filterKey`, `filterView`) must survive HTMX partial updates. Hidden form fields propagate this state. If URL-driven state isn't implemented first, drill-down state leaks on browser refresh.
- **Dashboard chart has no Go equivalent of Recharts:** This is a non-trivial dependency. Options: (a) server-side SVG bar chart via template, (b) CSS-only bar chart (simpler), (c) use a small vendored charting library. The spec doesn't prescribe this — it's a design decision for phase execution.

---

## MVP Definition

This is a milestone rebuild. "MVP" means: parity with the React SPA minus the Node.js build step, plus thread view and inline attachments.

### Launch With (v1.1 Web UI Rebuild)

- [x] PR #176 Templ + HTMX base (dashboard, browse, messages, search, message detail, deletions) — why essential: feature parity with existing SPA
- [x] Thread view with collapsible messages — why essential: designated new feature in milestone scope
- [x] Inline attachment rendering (images inline, others download) — why essential: designated new feature in milestone scope
- [x] Go-side HTML sanitization with bluemonday — why essential: security prerequisite for HTML email display
- [x] Sandboxed iframe for HTML email bodies — why essential: isolates email CSS/JS from app
- [x] External image blocking by default — why essential: privacy expectation for an archive tool
- [x] CID image substitution server-side — why essential: inline images in thread view won't render without it
- [x] `go:embed` static assets (zero npm) — why essential: the core goal of the rebuild

### Add After Validation (v1.1.x)

- [ ] Text/HTML body toggle with URL persistence — trigger: user feedback that they prefer plaintext for certain emails
- [ ] "Load external images" opt-in — trigger: users report needing to see tracked emails
- [ ] Attachment download with hash validation — trigger: security audit or user report of corrupted download

### Future Consideration (v2+)

- [ ] JSON API — trigger: MCP integration or mobile client work begins
- [ ] Server-side SVG time-series chart — trigger: dashboard chart looks broken without Recharts equivalent; defer until simpler CSS bar chart proves insufficient
- [ ] App-level encryption (noted in PROJECT.md as not-yet-implemented) — trigger: separate milestone

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| PR #176 base adoption (7 pages) | HIGH | MEDIUM (merge + conflict resolution) | P1 |
| Thread view | HIGH | HIGH (new handlers + template + collapsible) | P1 |
| Inline attachment rendering | HIGH | HIGH (CID substitution + content-type routing + CSP) | P1 |
| Go-side HTML sanitization | HIGH | LOW (bluemonday is drop-in) | P1 |
| Sandboxed iframe for HTML email | HIGH | LOW (template attribute) | P1 |
| External image blocking by default | MEDIUM | LOW (CSS in iframe wrapper) | P1 |
| Zero npm build step | HIGH | LOW (go:embed + committed _templ.go) | P1 |
| Debounced active search | HIGH | LOW (HTMX pattern, server already has FTS5) | P1 |
| URL-driven state (hx-push-url / hx-replace-url) | MEDIUM | LOW (HTMX attribute on forms) | P1 |
| Loading indicators | LOW | LOW (hx-indicator pattern) | P2 |
| Text/HTML body toggle | MEDIUM | LOW | P2 |
| "Load external images" opt-in | MEDIUM | LOW | P2 |
| Server-side chart on dashboard | MEDIUM | HIGH (SVG generation or CSS approach) | P2 |
| Attachment hash validation on download | LOW | LOW | P3 |

**Priority key:**
- P1: Must have for v1.1 launch
- P2: Should have, add when possible in same milestone
- P3: Nice to have, future consideration

---

## Approach Notes by Feature Type

### HTMX-Specific Implementation Patterns

**Aggregate drill-down (most complex):**
The browse page (`/browse`) maintains state across partial updates via hidden form fields. Each `<select>` for groupBy/sort emits `hx-post` with `hx-include="closest form"` to send all filter context. The server returns only the table `<tbody>` partial. `hx-replace-url` updates the URL so the state is bookmarkable. This mirrors how the React AggregatePage manages `useSearchParams` but moves all state to the URL.

**Debounced search:**
`hx-trigger="input changed delay:500ms, keyup[key=='Enter']"` on the search input. `hx-sync="this:abort"` cancels in-flight requests when user keeps typing. `hx-target="#search-results"` replaces the results div. `hx-indicator="#search-spinner"` shows loading state during DuckDB query.

**Deletion row removal:**
`hx-delete` on the cancel button, `hx-target="closest tr"`, `hx-swap="outerHTML swap:500ms"` with CSS `tr.htmx-swapping { opacity: 0; transition: opacity 500ms; }` for fade-out.

**Thread message collapse:**
Use native HTML `<details>`/`<summary>` for collapse — no JS, no HTMX needed. Last message gets `open` attribute from template logic. For lazy-loading message bodies in thread: `hx-get="/messages/{id}/body" hx-trigger="toggle once" hx-target="#body-{id}"` on the `<details>` element — loads body only on first expand.

### Dependencies on Existing Go Backend

| Feature | Existing Backend Support | Notes |
|---------|-------------------------|-------|
| Dashboard stats | `query.Engine.GetTotalStats` | EXISTS — used in current `handlers.go` |
| Message list | `query.Engine.ListMessages` + pagination | EXISTS |
| Aggregate browse | `query.Engine.Aggregate` | EXISTS — 7 view types already implemented |
| FTS search | `query.Engine` + `store.Store` FTS5 | EXISTS |
| Message detail | `store.Store.GetMessage` | EXISTS |
| Thread messages | `query.Engine.GetThreadMessages` | EXISTS — TUI uses it |
| Attachment inline serve | `internal/web/handlers.go` attachment handler | EXISTS — needs content-type routing + CSP header |
| CID substitution | NOT YET — currently done in browser JS (EmailRenderer.tsx) | NEW: must move to Go handler |
| HTML sanitization | NOT YET — currently DOMPurify in browser | NEW: bluemonday in Go handler |
| Deletion staging | `deletion.Manager` | EXISTS |

---

## Sources

- [HTMX Active Search pattern](https://htmx.org/examples/active-search/) — confirmed `hx-trigger="input changed delay:500ms"` pattern (HIGH confidence)
- [HTMX Keyboard Shortcuts examples](https://htmx.org/examples/keyboard-shortcuts/) — confirmed `from:body` global shortcut pattern (HIGH confidence)
- [HTMX hx-push-url attribute](https://htmx.org/attributes/hx-push-url/) — URL-driven state pattern (HIGH confidence)
- [Bookmarkable by Design: URL-Driven State in HTMX](https://www.lorenstew.art/blog/bookmarkable-by-design-url-state-htmx/) — hidden form field state propagation pattern (MEDIUM confidence)
- [Rendering untrusted HTML email safely — Close](https://making.close.com/posts/rendering-untrusted-html-email-safely/) — sandboxed iframe + CSP approach (HIGH confidence)
- [bluemonday Go HTML sanitizer](https://pkg.go.dev/github.com/microcosm-cc/bluemonday) — email sanitization in Go, `sanitise_html_email` example exists in repo (HIGH confidence)
- [HTMX Delete Row example](https://htmx.org/examples/delete-row/) — `htmx-swapping` CSS fade pattern (HIGH confidence)
- [HTMX infinite scroll issue #1188](https://github.com/bigskysoftware/htmx/issues/1188) — back-button breakage with infinite scroll (MEDIUM confidence)
- [Advanced Data Table with HTMX](https://benoitaverty.com/articles/en/data-table-with-htmx) — server-side sort/filter/pagination patterns (MEDIUM confidence)
- Existing React SPA (`web/src/`) — ground truth for all feature baseline requirements (HIGH confidence, primary source)

---
*Feature research for: msgvault v1.1 Web UI Rebuild (Templ + HTMX)*
*Researched: 2026-03-10*
