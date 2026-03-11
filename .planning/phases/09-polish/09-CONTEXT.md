# Phase 9: Polish - Context

**Gathered:** 2026-03-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Final v1.1 polish: text/HTML body toggle for messages, CSS bar chart on the dashboard showing archive volume over time, and loading indicators for all HTMX partial updates. After this phase, the Web UI rebuild is complete and ready for milestone close.

</domain>

<decisions>
## Implementation Decisions

### Text/HTML Body Toggle (POLISH-01)
- Toggle appears in the external images banner area, forming a unified "email toolbar" strip above the iframe: `[Text] [HTML]  ·  External images blocked [Load]`
- Toggle only renders when both BodyText and BodyHTML exist on the message — hide entirely for single-format messages
- Default to HTML when both formats exist (matches email client convention)
- Text view renders plain text in a `<pre>` block (no iframe needed) — HTML view uses existing sandboxed iframe pipeline
- Persistence via URL param only: `?format=text` or `?format=html` — no cookies, no localStorage
- `hx-replace-url` updates the URL when toggling so it's bookmarkable
- Toggle works identically in message detail and thread view (per-message toolbar in thread)
- When switching from HTML to text, external images banner disappears (text has no images)

### Dashboard CSS Bar Chart (POLISH-02)
- Horizontal bar chart showing messages per month, all time (every month with messages gets a bar)
- Bar width as CSS percentage relative to the max month's count — `width: calc(count / maxCount * 100%)`
- Month label on the left, bar in the middle, count on the right — single row per month
- Solarized Dark styling: bars in --cyan, labels in --base0, counts in --base1
- Each bar row is clickable — links to Aggregate > Time drill-down for that month (reuses existing `hx-get` drill-down pattern with `groupBy=time&filterKey=YYYY-MM`)
- Data source: `AggregateByTime` with monthly granularity from the query engine
- Chart section sits below the stat cards and above the Top Senders/Top Domains lists
- For 20+ year archives (240+ months), bars scroll naturally with the page — no height cap

### Loading Indicators (POLISH-03)
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

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `MessageDetail.BodyText` and `MessageDetail.BodyHTML`: Both fields already populated by query layer — toggle just chooses which to render
- `.htmx-indicator` CSS: Already defined in style.css (lines 336-348) — yellow text, hidden by default, shown during htmx-request
- `messageBodyWrapper` handler: Returns HTMX-swappable email body fragment — extend to accept `?format=text` param
- `messageBody` handler: Serves sanitized HTML for iframe — text view bypasses this entirely
- `AggregateByTime`: Query engine method with `TimeGranularity` parameter — supports monthly and yearly
- `AggregateRow` struct: Has `Key` (period string), `Count`, `TotalSize` — perfect for chart data
- Existing aggregate drill-down with `hx-get` and `hx-select` pattern on Top Senders/Domains rows — reuse for chart rows

### Established Patterns
- HTMX `hx-select="#main-content"` for partial page extraction
- `hx-replace-url="true"` for bookmarkable URL state
- `hx-swap-oob` for out-of-band updates (deletion badge)
- Email toolbar area already exists (external images banner in `messageBodyWrapper`)
- `renderPage` centralizes page-level data (accounts, deletion count)

### Integration Points
- `handlers_messages.go`: `messageBodyWrapper` needs `?format=text` support — render `<pre>` block instead of iframe
- `message.templ`: Email toolbar needs toggle buttons when both formats available
- `thread.templ`: Per-message toolbar in ThreadMessageCard needs same toggle
- `dashboard.templ`: New chart section between stat-cards and top-lists
- `handlers_dashboard.go` (or dashboard handler): Needs to call `AggregateByTime` for chart data
- `style.css`: New CSS for bar chart rows and text-view `<pre>` styling
- All templ files with `hx-get`/`hx-post`/`hx-delete`: Add `hx-indicator` attributes pointing to nearby indicator spans

</code_context>

<specifics>
## Specific Ideas

- Email toolbar unifies format toggle and image controls in one strip — feels like a mini control bar above the email content
- Bar chart should feel like a sparkline/activity graph (GitHub contribution-graph vibe but horizontal bars)
- "Loading..." text is deliberately minimal — consistent with the compact, data-dense aesthetic established in Phase 6

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 09-polish*
*Context gathered: 2026-03-11*
