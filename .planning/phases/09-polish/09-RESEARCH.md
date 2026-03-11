# Phase 9: Polish - Research

**Researched:** 2026-03-11
**Domain:** HTMX partial updates, CSS-only bar charts, Templ toggle patterns (Go/Templ/HTMX stack)
**Confidence:** HIGH

## Summary

Phase 9 is a pure UI polish phase requiring three targeted additions to the existing Templ+HTMX web server. All three requirements build directly on patterns already established in earlier phases — no new libraries, no architectural decisions, no ambiguity about the stack.

POLISH-01 (text/HTML body toggle) extends the existing `messageBodyWrapper` handler and email toolbar area. The toggle is a server-rendered format switch: when `?format=text` is in the URL, the handler renders a `<pre>` block instead of the iframe pipeline. Both `message.templ` and `thread.templ` get a unified toolbar strip above the email body. The key design insight is that text mode bypasses the entire iframe/sandbox/CSP/resize pipeline — it renders directly in the page as a `<pre>` element.

POLISH-02 (CSS bar chart) uses no external library. The `dashboard` handler calls `h.engine.Aggregate(ctx, query.ViewTime, opts)` with `TimeGranularity: query.TimeMonth` — that function already exists and returns `[]AggregateRow` with `Key` (period string like "2024-01") and `Count`. The chart is a flex-layout div-per-row where bar width is a `style="width: Xpx"` inline percentage. The only new Go code is the `maxCount` calculation in the template helper (or passed from the handler).

POLISH-03 (loading indicators) is a pattern already proven in the search page (`hx-indicator="#search-indicator"` + `<span class="htmx-indicator">`). The work is mechanical: audit every `hx-get`/`hx-post`/`hx-delete` in every template, add `hx-indicator` pointing to a nearby indicator span, and place that span. The `.htmx-indicator` CSS class is already defined in `style.css` lines 336-348.

**Primary recommendation:** Execute as three independent waves. Each requirement is self-contained with zero inter-dependencies.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Text/HTML Body Toggle (POLISH-01)
- Toggle appears in the external images banner area, forming a unified "email toolbar" strip above the iframe: `[Text] [HTML]  ·  External images blocked [Load]`
- Toggle only renders when both BodyText and BodyHTML exist on the message — hide entirely for single-format messages
- Default to HTML when both formats exist (matches email client convention)
- Text view renders plain text in a `<pre>` block (no iframe needed) — HTML view uses existing sandboxed iframe pipeline
- Persistence via URL param only: `?format=text` or `?format=html` — no cookies, no localStorage
- `hx-replace-url` updates the URL when toggling so it's bookmarkable
- Toggle works identically in message detail and thread view (per-message toolbar in thread)
- When switching from HTML to text, external images banner disappears (text has no images)

#### Dashboard CSS Bar Chart (POLISH-02)
- Horizontal bar chart showing messages per month, all time (every month with messages gets a bar)
- Bar width as CSS percentage relative to the max month's count — `width: calc(count / maxCount * 100%)`
- Month label on the left, bar in the middle, count on the right — single row per month
- Solarized Dark styling: bars in --cyan, labels in --base0, counts in --base1
- Each bar row is clickable — links to Aggregate > Time drill-down for that month (reuses existing `hx-get` drill-down pattern with `groupBy=time&filterKey=YYYY-MM`)
- Data source: `AggregateByTime` with monthly granularity from the query engine
- Chart section sits below the stat cards and above the Top Senders/Top Domains lists
- For 20+ year archives (240+ months), bars scroll naturally with the page — no height cap

#### Loading Indicators (POLISH-03)
- Inline "Loading..." text using existing `.htmx-indicator` CSS class (yellow --yellow text, hidden by default, shown during htmx-request)
- Apply to ALL HTMX partial updates: pagination, sort, filter, stage/unstage deletion, Load images, thread body lazy-load, account filter changes, aggregate drill-down
- Current content stays fully visible during loading (no opacity dim) — only the "Loading..." text appears
- Each HTMX trigger point gets its own `hx-indicator` pointing to a nearby `<span class="htmx-indicator">` element
- Matches existing search page pattern ("Searching..." text next to input)

### Claude's Discretion
- Exact placement of "Loading..." text relative to each trigger (inline, below button, next to header)
- Bar chart row height and spacing
- Whether text view uses a monospace or proportional font in the `<pre>` block
- Sort order of bar chart (chronological ascending vs descending)
- How to handle months with zero messages (skip or show empty row)

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| POLISH-01 | User can toggle between plain text and HTML rendering per message | `messageBodyWrapper` handler accepts `?format=text` param; `MessageDetail.BodyText` and `BodyHTML` already populated; toggle rendered in email toolbar strip in both `message.templ` and `thread.templ` |
| POLISH-02 | Dashboard displays time-series data as a CSS bar chart (no JS charting library) | `h.engine.Aggregate(ctx, query.ViewTime, opts)` with `TimeGranularity: query.TimeMonth` already works; `AggregateRow.Key` is "YYYY-MM", `AggregateRow.Count` is message count; max count calculation needed for percentage widths |
| POLISH-03 | Loading indicators display during HTMX partial page updates | `.htmx-indicator` CSS already defined (lines 336-348 of `style.css`); pattern proven in `search.templ` lines 29+33; mechanical addition to all `hx-get`/`hx-post`/`hx-delete` trigger points |
</phase_requirements>

---

## Standard Stack

### Core (no changes from existing phases)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| HTMX | embedded (htmx.min.js) | Partial page swaps | Already in use across all pages |
| Templ | v0.3.1001 (pinned) | Server-side HTML templates | Established in Phase 6 |
| Go chi | v5 | HTTP routing | Established in Phase 6 |

### No New Dependencies
This phase introduces zero new Go modules, npm packages, or CDN references. The CSS bar chart uses inline `style` attributes; the toggle uses existing HTMX attributes; the indicators reuse the existing `.htmx-indicator` CSS class.

## Architecture Patterns

### Pattern 1: HTMX Body Toggle via `messageBodyWrapper`
**What:** The `/messages/{id}/body-wrapper` endpoint already handles the `?showImages=true` param to switch between "blocked images" and "show images" variants. Extend this same endpoint to also accept `?format=text` to return a `<pre>` block instead of the iframe wrapper.

**When to use:** Whenever the user clicks the Text/HTML toggle buttons in the email toolbar.

**Key pattern — handler branching:**
```go
// In handlers_messages.go, messageBodyWrapper()
format := r.URL.Query().Get("format")
showImages := r.URL.Query().Get("showImages") == "true"

if format == "text" {
    // Render <pre> block with msg.BodyText
    // No iframe, no CSP, no banner
} else {
    // Existing iframe pipeline (with or without showImages)
}
```

**Key pattern — toolbar in message.templ:**
```templ
// email toolbar: only rendered when both formats exist
if msg.BodyHTML != "" && msg.BodyText != "" {
    <div class="email-toolbar">
        <a href="#"
           hx-get={ fmt.Sprintf("/messages/%d/body-wrapper?format=text", msg.ID) }
           hx-target="#email-body-wrapper"
           hx-swap="outerHTML"
           hx-replace-url={ fmt.Sprintf("/messages/%d?format=text", msg.ID) }
           class={ "email-toolbar-btn", templ.KV("active", format == "text") }
        >Text</a>
        <a href="#"
           hx-get={ fmt.Sprintf("/messages/%d/body-wrapper?format=html", msg.ID) }
           hx-target="#email-body-wrapper"
           hx-swap="outerHTML"
           hx-replace-url={ fmt.Sprintf("/messages/%d?format=html", msg.ID) }
           class={ "email-toolbar-btn", templ.KV("active", format != "text") }
        >HTML</a>
        if format != "text" {
            <span class="email-toolbar-sep">·</span>
            <span>External images blocked.</span>
            <a href="#" hx-get=...>Load</a>
        }
    </div>
}
```

**URL persistence:** The handler receives `?format=text` from the URL on initial page load (not just from HTMX swap). `messageDetail` passes `format` to the template so the initial render selects the correct view and highlights the correct toolbar button.

**Thread view complication:** `ThreadMessageCard` lazy-loads collapsed messages via `hx-get="/messages/{id}/body-wrapper"`. The format toggle must include `?format=text` in this URL if the current message URL has `format=text`. However since per-message format preference is per-message (not global), the simplest approach is: each expanded ThreadMessageCard passes its own format state. The toolbar in the thread card uses `hx-target="closest .email-render-wrapper"` (matching Phase 08-01 decision) so the swap is contained to that card.

### Pattern 2: CSS Bar Chart (no JS)
**What:** Pure HTML+CSS horizontal bar chart. Each row is a `<div class="chart-row">` with three children: label, bar fill, count. The bar fill width is computed server-side as a percentage of maxCount and written as an inline style.

**Key templ pattern:**
```templ
// In dashboard.templ — new chart component
templ BarChart(rows []query.AggregateRow) {
    if len(rows) == 0 {
        return
    }
    // Compute max on the Go side (or pass from handler)
    <div class="archive-chart">
        <h3>Archive Volume by Month</h3>
        for _, row := range rows {
            <div class="chart-row"
                 data-href={ fmt.Sprintf("/aggregate/drilldown?groupBy=time&filterKey=%s", row.Key) }
                 hx-get={ fmt.Sprintf("/aggregate/drilldown?groupBy=time&filterKey=%s", row.Key) }
                 hx-select="#main-content"
                 hx-target="#main-content"
                 hx-swap="outerHTML"
                 hx-replace-url="true"
            >
                <span class="chart-label">{ row.Key }</span>
                <div class="chart-bar-track">
                    <div class="chart-bar-fill" style={ fmt.Sprintf("width: %.1f%%", pct(row.Count, maxCount)) }></div>
                </div>
                <span class="chart-count">{ FormatNum(row.Count) }</span>
            </div>
        }
    </div>
}
```

**Handler change:** `handlers_dashboard.go` adds one `Aggregate` call for the chart data:
```go
chartOpts := query.AggregateOptions{
    SourceID:        aggSourceID,
    SortField:       query.SortByName,  // chronological key sort
    SortDirection:   query.SortAsc,     // oldest to newest
    TimeGranularity: query.TimeMonth,
    Limit:           0,  // no limit — all months
}
chartData, err := h.engine.Aggregate(ctx, query.ViewTime, chartOpts)
```

**DashboardPage signature change:** Add `chartData []query.AggregateRow` parameter, or pass `maxCount int64` + rows. Simplest: pass both rows and pre-computed maxCount.

**Max count computation — Go helper in `helpers.go`:**
```go
// MaxAggregateCount returns the highest Count from a slice of AggregateRows.
func MaxAggregateCount(rows []query.AggregateRow) int64 {
    var max int64
    for _, r := range rows {
        if r.Count > max {
            max = r.Count
        }
    }
    return max
}
```

### Pattern 3: HTMX Loading Indicators (mechanical audit)
**What:** Add `hx-indicator="#{id}"` to every HTMX trigger element plus a `<span id="{id}" class="htmx-indicator">Loading...</span>` nearby.

**Proven pattern from search.templ (lines 29+33):**
```templ
// Trigger element:
hx-indicator="#search-indicator"

// Indicator span (sibling, outside the swapped target):
<span id="search-indicator" class="htmx-indicator">Searching...</span>
```

**CRITICAL constraint:** The indicator span must NOT be inside the `hx-target` element that gets swapped out. If the span is inside the swap target, it disappears when the swap happens, causing a flash. Place indicators as siblings of the trigger or in a stable parent.

**Full audit of HTMX trigger points requiring indicators:**

| Template | Trigger | Suggested indicator placement |
|----------|---------|-------------------------------|
| `components.templ` Pagination | `hx-get` on Prev/Next links | `<span class="htmx-indicator">` in `.pagination` div |
| `components.templ` SortHeader | `hx-get` on `<th>` | `<span class="htmx-indicator">` in table header area |
| `messages.templ` message rows | `hx-get` on `<tr>` | inline after row click — or a page-level indicator |
| `aggregate.templ` view tabs | `hx-get` on tab links | inline after tab strip |
| `aggregate.templ` drilldown rows | `hx-get` on `<tr>` | page-level indicator |
| `aggregate.templ` stagingForm | `hx-post` on submit | `<span>` in `#stage-result` area (already exists) |
| `aggregate.templ` breadcrumbs | `hx-get` on crumb links | inline after breadcrumbs |
| `dashboard.templ` sender/domain rows | `hx-get` on `<tr>` | page-level indicator |
| `message.templ` Load images | `hx-get` on Load link | inline in email toolbar |
| `thread.templ` Load images | `hx-get` on Load link | inline in email toolbar |
| `thread.templ` lazy body load | `hx-trigger="toggle once"` | inside `.thread-message-body` before placeholder |
| `deletions.templ` unstage button | `hx-delete` on button | inline next to button |

**Page-level indicator pattern:** For row clicks where per-row indicators would be noisy, a single page-level indicator at the top of `#main-content` works: `<span id="page-indicator" class="htmx-indicator">Loading...</span>`. All row `hx-get` attributes point to `hx-indicator="#page-indicator"`.

**Account filter (keys.js htmx.ajax):** The JS-driven account filter in `keys.js` doesn't use declarative HTMX attributes — it calls `htmx.ajax()` directly. HTMX still fires `htmx:beforeRequest` / `htmx:afterSettle` events on the target element, so a page-level indicator works here too if the target is `#main-content`. No indicator needed in the template; optionally add one via JS `htmx:beforeRequest` / `htmx:afterSettle` event handlers.

### Anti-Patterns to Avoid
- **Indicator inside swap target:** The indicator span must be outside (or an ancestor of) the swapped element. If `hx-target="#main-content"` and `hx-swap="outerHTML"`, the indicator cannot be inside `#main-content`.
- **Per-row unique IDs for row indicators:** Don't generate `id="row-indicator-{id}"` for every message row — this creates dozens of DOM elements. Use a single page-level indicator for table row clicks.
- **Iframe for text view:** Text mode uses `<pre>`, never an iframe. The iframe pipeline (sandboxing, CSP headers, resize postMessage) is HTML-specific and irrelevant for plain text.
- **JS charting library for bar chart:** The CONTEXT.md explicitly prohibits this. CSS percentage widths on a div are sufficient for the archive history visualization use case.
- **Limit=100 for chart data:** `DefaultAggregateOptions()` has `Limit: 100`. A 20-year archive has 240 months. Set `Limit: 0` (or a large value like 1000) when fetching chart data to get all months.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Text escaping in `<pre>` | Custom HTML escaper | Templ auto-escapes `{ }` expressions | Templ's `{ msg.BodyText }` is already HTML-safe |
| URL state for toggle | localStorage, cookies, JS | `hx-replace-url` + URL param read in handler | Already pattern from Phase 6 |
| Time series chart | SVG or Canvas renderer | CSS `width` percentage on div | POLISH-02 spec; VIS-01 (SVG upgrade) is v2 |
| Bar chart max calculation | Complex stats library | Simple Go loop over `[]AggregateRow` | 4-line helper in helpers.go |

## Common Pitfalls

### Pitfall 1: `hx-replace-url` on body-wrapper swap doesn't update full page URL
**What goes wrong:** The toggle does `hx-target="#email-body-wrapper"` (swap the wrapper div), but also needs `hx-replace-url` to persist `?format=text` in the URL. `hx-replace-url="true"` replaces with the `hx-get` URL, which is `/messages/{id}/body-wrapper?format=text` — not the message detail URL.
**Why it happens:** `hx-replace-url="true"` uses the `hx-get` URL literally. The body-wrapper endpoint URL is an internal fragment endpoint.
**How to avoid:** Use `hx-replace-url="/messages/{id}?format=text"` (a literal URL string) instead of `"true"`. This explicitly sets the browser URL to the canonical message page URL with the format param.
**Warning signs:** URL bar shows `/messages/123/body-wrapper?format=text` after toggle.

### Pitfall 2: Initial page load doesn't respect `?format=text`
**What goes wrong:** User bookmarks `/messages/123?format=text`, reloads — page shows HTML view because `messageDetail` handler ignores the `format` param.
**Why it happens:** `messageDetail` renders `message.templ` without reading `?format`. The template always defaults to HTML when `BodyHTML != ""`.
**How to avoid:** `messageDetail` reads `r.URL.Query().Get("format")` and passes it to the template (or calls `messageBodyWrapper` logic to render the initial body wrapper with the correct format). The template must receive `format` to (1) render the initial body correctly and (2) highlight the active toolbar button.
**Warning signs:** Toggle works interactively but breaks on page reload.

### Pitfall 3: Thread lazy-load loses format state
**What goes wrong:** Collapsed thread messages lazy-load via `hx-get="/messages/{id}/body-wrapper"` on toggle. After a user clicks "Text" on one expanded message, collapsed messages still load in HTML mode.
**Why it happens:** The lazy-load URL in `thread.templ` (line 152) doesn't carry a format param.
**How to avoid:** Per CONTEXT.md decision, format toggle in thread view is per-message (not global). Each per-message toolbar fires its own `hx-get` with the format param. The initial lazy-load always uses HTML (correct default). Once a thread message is loaded, its toolbar allows switching that specific message's format. No global thread format state needed.

### Pitfall 4: CSS bar chart — percentage of 0 for max=0
**What goes wrong:** If `maxCount == 0` (empty archive), `width: Inf%` or divide-by-zero panic in the template helper.
**Why it happens:** Integer division by zero in `pct(row.Count, maxCount)`.
**How to avoid:** Guard in the helper: `if maxCount == 0 { return 0 }`. Also guard the chart section: only render when `len(chartData) > 0`.

### Pitfall 5: `Limit: 0` interpreted as "use default" by Aggregate
**What goes wrong:** `AggregateOptions{Limit: 0}` triggers the `DefaultAggregateOptions()` default of `Limit: 100`, cutting off archives older than ~8 years.
**Why it happens:** Check the `executeAggregate` function — it may substitute a default when `opts.Limit == 0`.
**How to avoid:** Check `sqlite.go`'s `executeAggregate` for `if opts.Limit == 0 { opts.Limit = 100 }` pattern. If present, use a large explicit limit (e.g., `Limit: 10000`) for the chart query. Verify in the code before setting.
**Warning signs:** Archive chart shows only the most recent ~100 months.

### Pitfall 6: `hx-indicator` ID collision with swapped content
**What goes wrong:** Pagination indicator span gets ID `page-indicator` but is inside `#main-content`. On swap, the indicator span is removed from the DOM. On next request, HTMX can't find `#page-indicator` and silently skips the indicator.
**Why it happens:** `hx-swap="outerHTML"` on `#main-content` replaces the entire element. Any indicator inside it is gone.
**How to avoid:** For `hx-target="#main-content"` swaps, place the indicator span in the layout template (`layout.templ`) outside `#main-content` — for example in the navbar or just before `#main-content`. Or use a body-level element that persists across swaps.

## Code Examples

### Bar chart CSS
```css
/* Add to style.css */
.archive-chart {
    margin-bottom: 1.5rem;
}

.chart-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.2rem 0;
    cursor: pointer;
    font-size: 0.85rem;
}

.chart-row:hover .chart-bar-fill {
    background-color: var(--blue);
}

.chart-label {
    width: 6rem;
    flex-shrink: 0;
    text-align: right;
    color: var(--base0);
    font-variant-numeric: tabular-nums;
}

.chart-bar-track {
    flex: 1;
    height: 0.9rem;
    background-color: var(--base02);
    border-radius: 2px;
    overflow: hidden;
}

.chart-bar-fill {
    height: 100%;
    background-color: var(--cyan);
    border-radius: 2px;
    min-width: 2px;  /* visible even for 1-message months */
    transition: width 0.1s ease;
}

.chart-count {
    width: 4rem;
    flex-shrink: 0;
    color: var(--base1);
    font-variant-numeric: tabular-nums;
    font-size: 0.8rem;
}
```

### Email toolbar CSS
```css
/* Add to style.css */
.email-toolbar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.75rem;
    margin-bottom: 0.5rem;
    border: 1px solid var(--base01);
    border-radius: 4px;
    background: var(--base02);
    font-size: 0.85rem;
    color: var(--yellow);
}

.email-toolbar-btn {
    color: var(--base01);
    text-decoration: none;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    border: 1px solid transparent;
    font-size: 0.8rem;
    cursor: pointer;
}

.email-toolbar-btn:hover {
    color: var(--base0);
    text-decoration: none;
}

.email-toolbar-btn.active {
    color: var(--base0);
    border-color: var(--base01);
    background-color: var(--base03);
}

.email-toolbar-sep {
    color: var(--base01);
    padding: 0 0.25rem;
}

.body-text-pre {
    white-space: pre-wrap;
    word-wrap: break-word;
    font-family: monospace;
    font-size: 0.85rem;
    line-height: 1.5;
    color: var(--base0);
    padding: 0.75rem;
    margin: 0;
    background: var(--base03);
    border: 1px solid var(--base01);
    border-radius: 4px;
}
```

### Checking Aggregate Limit=0 behavior
```bash
grep -n "Limit\|opts.Limit" /Users/ryanstern/msgvault/internal/query/sqlite.go | head -20
```
Run this before implementing POLISH-02 to confirm whether Limit=0 uses a default.

## State of the Art

| Old Approach | Current Approach | Applies To |
|--------------|------------------|------------|
| React SPA with Recharts/Chart.js | CSS-only bar chart | POLISH-02 (by design — v2 VIS-01 will add SVG) |
| Format stored in localStorage | URL param `?format=text` | POLISH-01 (bookmarkable, stateless) |
| No loading feedback | `.htmx-indicator` spans | POLISH-03 |

## Open Questions

1. **`Aggregate` Limit=0 behavior**
   - What we know: `DefaultAggregateOptions()` has `Limit: 100`; the executeAggregate function in `sqlite.go` may substitute this default when `opts.Limit == 0`
   - What's unclear: Whether setting `Limit: 0` in `AggregateOptions` gets a default applied, or passes 0 to SQL (which would mean no LIMIT clause = all rows)
   - Recommendation: Read `sqlite.go` `executeAggregate` before implementing POLISH-02; use an explicit large limit (10000) if zero triggers default substitution

2. **`messageBodyWrapper` raw HTML output vs templ component**
   - What we know: The current `messageBodyWrapper` writes raw HTML via `fmt.Fprintf` (not a templ component). Adding the email toolbar to this handler requires either converting it to templ or adding the toolbar logic in the raw HTML output.
   - What's unclear: Whether converting to templ would conflict with any lazy-load template expectations
   - Recommendation: Keep `messageBodyWrapper` as raw HTML output for the wrapper div; add toolbar buttons as raw HTML in `fmt.Fprintf`. The initial render (via `message.templ` / `thread.templ`) uses templ for the toolbar — just ensure both codepaths produce consistent HTML structure.

3. **DuckDB engine vs SQLite engine for chart data**
   - What we know: `h.engine.Aggregate(ctx, query.ViewTime, ...)` works on both `SQLiteEngine` and the DuckDB/Parquet engine. The TUI uses the Parquet path; the web server uses `SQLiteEngine` based on Phase 6 implementation.
   - What's unclear: Whether `ViewTime` with `TimeMonth` granularity performs acceptably on SQLite for a 20-year archive (~300K+ messages). Test shows it works.
   - Recommendation: Use the existing engine call — if slow, it will be caught during development. No architectural change needed.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` package + `net/http/httptest` |
| Config file | none — standard `go test ./...` |
| Quick run command | `cd /Users/ryanstern/msgvault && go test ./internal/web/... -run TestHandler -timeout 30s` |
| Full suite command | `cd /Users/ryanstern/msgvault && go test ./... -timeout 120s` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| POLISH-01 | `messageBodyWrapper?format=text` returns `<pre>` block | unit | `go test ./internal/web/... -run TestMessageBodyWrapper -timeout 30s` | ❌ Wave 0 |
| POLISH-01 | `messageBodyWrapper?format=html` returns iframe wrapper | unit | `go test ./internal/web/... -run TestMessageBodyWrapper -timeout 30s` | ❌ Wave 0 |
| POLISH-01 | toolbar renders only when both BodyText and BodyHTML present | unit | `go test ./internal/web/... -run TestMessageDetail -timeout 30s` | ❌ Wave 0 |
| POLISH-02 | dashboard handler fetches ViewTime data and passes maxCount | unit | `go test ./internal/web/... -run TestDashboard -timeout 30s` | ❌ Wave 0 |
| POLISH-02 | bar chart renders with correct percentage widths | unit | `go test ./internal/web/... -run TestBarChart -timeout 30s` | ❌ Wave 0 |
| POLISH-03 | HTMX indicator attributes present on pagination links | unit | `go test ./internal/web/... -run TestIndicators -timeout 30s` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/web/... -timeout 30s`
- **Per wave merge:** `go test ./... -timeout 120s`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/handlers_test.go` already exists — add new test cases for body-wrapper format param and dashboard chart data
- [ ] No separate file needed; extend existing test file

## Sources

### Primary (HIGH confidence)
- Direct codebase inspection — `internal/web/templates/*.templ`, `internal/web/handlers_messages.go`, `internal/web/handlers_dashboard.go`, `internal/web/static/style.css`, `internal/query/models.go`
- Existing `.htmx-indicator` implementation confirmed at `style.css` lines 336-348
- `AggregateRow` struct and `ViewTime`/`TimeGranularity` types confirmed in `internal/query/models.go`
- Search page indicator pattern confirmed at `search.templ` lines 29+33

### Secondary (MEDIUM confidence)
- HTMX `hx-replace-url` behavior with explicit URL string vs `"true"` — verified from HTMX docs knowledge (standard documented behavior)
- Templ `templ.KV` for conditional class names — verified from prior phase usage in `thread.templ` line 85

### Tertiary (LOW confidence)
- `Aggregate` Limit=0 behavior — must verify in `sqlite.go` before implementing

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies; all patterns proven in existing phases
- Architecture: HIGH — all three features extend existing handlers/templates with well-understood patterns
- Pitfalls: HIGH — sourced directly from codebase inspection of real code paths
- Open questions: LOW for Limit=0 behavior; requires 5-minute code read to resolve

**Research date:** 2026-03-11
**Valid until:** 2026-04-11 (stable stack, no fast-moving dependencies)
