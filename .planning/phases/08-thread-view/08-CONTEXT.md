# Phase 8: Thread View - Context

**Gathered:** 2026-03-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Full conversation view — all messages in a Gmail thread displayed chronologically on a single page. Messages are collapsible via `<details>`, with the most recent pre-expanded. Inline images render using Phase 7's iframe pipeline. Keyboard shortcuts `t` (enter thread), `n`/`p` (navigate messages) provide efficient navigation. No new attachment infrastructure — reuse existing CID substitution and download endpoints.

</domain>

<decisions>
## Implementation Decisions

### Collapsed Message Appearance
- Each message is a native HTML `<details>` element
- Summary line (collapsed state): **sender name/email** + **relative date** + first ~80 chars of snippet, all on one line
- Solarized muted styling (--base01 text) for collapsed messages; full contrast (--base0) for expanded
- Subtle bottom border (1px --base02) separates messages in the thread
- The latest (most recent) message starts with `open` attribute — pre-expanded on page load
- Expanded state shows full headers (From, To, Cc, Date, Size, Labels) in the same `dl.header-list` format as message detail page, followed by the body iframe and attachments

### Body Loading Strategy
- **Lazy-load via HTMX on expand** — only the pre-expanded (latest) message loads its body eagerly
- Collapsed messages contain an empty body placeholder div with `hx-get="/messages/{id}/body-wrapper"` and `hx-trigger="toggle"` (fires when `<details>` opens)
- The `hx-trigger` uses the `once` modifier so re-collapsing/expanding doesn't re-fetch
- Each message body renders in its own sandboxed iframe (reusing Phase 7's `messageBody` endpoint and postMessage resize)
- External image blocking banner per message (not per thread) — each iframe gets its own "Load images" toggle
- For threads with 50+ messages, consider a "Load older messages" link at the top (paginate at 50), but start simple — load all summaries, lazy-load bodies

### Thread Entry & Scroll Behavior
- `t` shortcut on message detail page navigates to `/threads/{conversationId}?highlight={messageId}`
- "View thread" link added to message detail page header (near subject), visible when conversation has >1 message
- Thread page scrolls to and auto-expands the highlighted message (if `?highlight` param present); otherwise scrolls to bottom (latest message)
- Messages list does NOT link directly to threads — thread is accessed from message detail only (keeps message list simple)
- URL structure: `/threads/{conversationId}` — new route, separate from message detail

### Thread Header & Context
- Thread subject as `<h2>` at the top
- Below subject: participant summary line — "Between **Alice**, **Bob**, and **2 others**" (show first 2-3 names, collapse rest)
- Message count + date range: "12 messages · Jan 2024 — Mar 2026"
- Back link: "Back to messages" (same as message detail page) — always links to `/messages` since thread can be entered from multiple paths

### Keyboard Navigation
- `n` — scroll to and expand next message in thread (wrap around at bottom)
- `p` — scroll to and expand previous message in thread (wrap around at top)
- `Esc` / `Backspace` — go back (standard behavior from keys.js)
- `t` on message detail — navigate to thread view
- Thread-specific shortcuts only active when on `/threads/*` path (check in keys.js)
- Currently focused message gets a subtle left border highlight (2px --cyan)

### Claude's Discretion
- Exact CSS for thread message cards (padding, margins, border-radius)
- How participant names are resolved (display name vs email fallback)
- Whether to show attachment count in collapsed summary line
- Animation/transition for expand/collapse
- Error state when conversation has only 1 message (redirect to message detail or show single-message thread)
- Exact implementation of the "Load older messages" pagination if needed

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `MessageFilter.ConversationID`: Already supports filtering messages by conversation — use with `ListMessages` for thread message list
- `messageBody` + `messageBodyWrapper` handlers: Serve sanitized HTML in sandboxed iframes — reuse directly per thread message
- `message.templ` MessageDetailPage: Header layout (dl.header-list, formatAddresses, attachment list) — extract reusable components for thread message cards
- `keys.js`: Existing keyboard shortcut framework — extend with `t`, `n`, `p` handlers
- `formatAddresses()` helper in message.templ: Reuse for thread message headers
- TUI `loadThreadMessages`: Reference implementation for thread loading by conversation ID

### Established Patterns
- HTMX `hx-select` pattern: Server returns full pages, HTMX extracts `#main-content` fragment
- `renderPage` centralizes account listing and deletion count
- OOB swap pattern available for dynamic updates
- PostMessage iframe resize already handles variable-height email bodies
- Chi URL params: `chi.URLParam(r, "id")` for route parameters

### Integration Points
- New route: `r.Get("/threads/{conversationId}", h.threadView)` in server.go
- New handler: `threadView` in a new `handlers_thread.go` file
- New template: `thread.templ` for thread page layout
- `keys.js`: Add `t` handler on message detail (navigate to thread), `n`/`p` handlers on thread page
- `message.templ`: Add "View thread" link when `ConversationID` indicates multi-message thread
- Query layer: `ListMessages` with `ConversationID` filter returns `[]MessageSummary` for the summary list; individual `GetMessage` calls for expanded bodies (lazy-loaded)

</code_context>

<specifics>
## Specific Ideas

- Lazy-load is the key UX decision — keeps 50+ message threads snappy by only loading iframes on expand
- Thread page should feel like reading a conversation, not a list of separate messages — visual continuity matters
- Reuse as much of the message detail rendering as possible (headers, body iframe, attachments) to avoid duplicating templates
- The `<details>` approach is deliberately simple — no JS state management, works with HTMX naturally, progressive enhancement friendly

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 08-thread-view*
*Context gathered: 2026-03-11*
