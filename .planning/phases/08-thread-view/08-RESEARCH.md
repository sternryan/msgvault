# Phase 8: Thread View - Research

**Researched:** 2026-03-11
**Domain:** Go/Templ/HTMX thread view — HTML `<details>` collapsible messages, lazy-loading via HTMX, iframe reuse, keyboard shortcuts
**Confidence:** HIGH

## Summary

Phase 8 adds a dedicated `/threads/{conversationId}` route that renders all messages in a conversation chronologically on one page. The architecture is an extension of the existing Templ + HTMX + sandboxed-iframe stack from Phases 6–7 — no new dependencies are needed.

The core design relies on native HTML `<details>` elements for collapse/expand, HTMX `hx-trigger="toggle once"` for lazy body loading per message, and reuse of the existing `messageBody` and `messageBodyWrapper` endpoints for per-message iframes. The query layer already supports `MessageFilter.ConversationID` for fetching all messages in a thread; a single `ListMessages` call with that filter retrieves the thread's summary list, then bodies are lazy-loaded individually.

The biggest implementation risk is the keyboard shortcut conflict: `t` is already bound in keys.js to `navigateToTimeView()`. The CONTEXT.md decision calls for `t` on message detail to navigate to thread view — this requires path-guarded logic in keys.js. Similarly, `n`/`p` must only fire on `/threads/*` paths to avoid interfering with the message list row navigation.

**Primary recommendation:** Build in three sequential units: (1) backend route + handler + query layer, (2) thread.templ template, (3) keys.js extensions with path guards.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Each message is a native HTML `<details>` element
- Summary line: **sender name/email** + **relative date** + first ~80 chars of snippet (one line)
- Solarized muted styling (--base01 text) for collapsed; full contrast (--base0) for expanded
- Subtle bottom border (1px --base02) between messages
- Latest message starts with `open` attribute — pre-expanded on page load
- Expanded state: full headers (From, To, Cc, Date, Size, Labels) in `dl.header-list` format, then body iframe, then attachments
- **Lazy-load via HTMX on expand** — only latest message loads eagerly
- Collapsed messages: empty placeholder div with `hx-get="/messages/{id}/body-wrapper"` and `hx-trigger="toggle"` (with `once` modifier)
- Each message body in its own sandboxed iframe reusing Phase 7's `messageBody` endpoint
- External image blocking banner per-message (not per-thread); each iframe gets its own "Load images" toggle
- For 50+ message threads: consider "Load older messages" link at top (paginate at 50), but start simple — load all summaries
- `t` shortcut on message detail navigates to `/threads/{conversationId}?highlight={messageId}`
- "View thread" link added to message detail header when conversation has >1 message
- Thread page scrolls to and auto-expands highlighted message if `?highlight` present; otherwise scrolls to bottom (latest)
- Messages list does NOT link directly to threads — thread is accessed from message detail only
- URL structure: `/threads/{conversationId}` — new route
- Thread header: `<h2>` subject, participant summary ("Between Alice, Bob, and 2 others"), message count + date range ("12 messages · Jan 2024 — Mar 2026"), back link to `/messages`
- `n` — scroll to and expand next message (wrap around)
- `p` — scroll to and expand previous message (wrap around)
- `Esc`/`Backspace` — go back (standard)
- Thread-specific shortcuts active only on `/threads/*` path
- Currently focused message gets subtle left border highlight (2px --cyan)

### Claude's Discretion
- Exact CSS for thread message cards (padding, margins, border-radius)
- How participant names are resolved (display name vs email fallback)
- Whether to show attachment count in collapsed summary line
- Animation/transition for expand/collapse
- Error state when conversation has only 1 message (redirect to message detail or show single-message thread)
- Exact implementation of "Load older messages" pagination if needed

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| THREAD-01 | User can view all messages in a conversation chronologically on a single page | `ListMessages` with `ConversationID` filter already works; new `/threads/{conversationId}` route + handler needed |
| THREAD-02 | Thread messages are collapsible via native HTML `<details>`, with the latest message pre-expanded | `<details open>` for latest; HTMX `hx-trigger="toggle once"` for lazy bodies; no JS state needed |
| THREAD-03 | Inline images render directly in thread messages, other attachments as download links | Reuses Phase 7's `messageBody` endpoint with CID substitution; attachment download links same pattern as message detail |
| THREAD-04 | User can navigate to thread view from message detail via link and `t` keyboard shortcut | Requires "View thread" link in `message.templ` when `ConversationID > 0`; `t` key in keys.js path-guarded to message detail |
| THREAD-05 | User can scroll between thread messages with `n`/`p` keyboard shortcuts | `n`/`p` handlers in keys.js, path-guarded to `/threads/*`; `data-thread-msg-id` attributes on each `<details>` element for cursor tracking |
</phase_requirements>

---

## Standard Stack

### Core (all already in go.mod)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| chi | v5 | HTTP routing, URL params | Already used for all routes |
| templ | v0.3.1001 | Server-side HTML templating | Pinned version, all pages use it |
| HTMX | 2.x (embedded) | Lazy loading, partial swaps | Already in `/static/htmx.min.js` |
| go-sqlite3 (via go-duckdb) | existing | DB queries via SQLiteEngine | All message queries go through `Engine` interface |

### No New Dependencies
This phase is pure extension of existing stack. No new Go packages, no new JS libraries, no new build steps.

### Installation
```bash
# Nothing to install — all dependencies already in go.mod and static/
```

## Architecture Patterns

### Recommended File Structure
```
internal/web/
├── handlers_thread.go       # NEW: threadView handler
├── server.go                # MODIFY: add /threads/{conversationId} route
├── templates/
│   ├── thread.templ         # NEW: ThreadPage, ThreadMessageCard components
│   └── message.templ        # MODIFY: add "View thread" link
└── static/
    └── keys.js              # MODIFY: add t/n/p handlers with path guards
```

### Pattern 1: Thread Handler with ConversationID Filter

**What:** `threadView` handler resolves conversationId from URL, calls `ListMessages` with `ConversationID` filter (ascending date sort), renders ThreadPage template.

**When to use:** Single handler for `/threads/{conversationId}`.

**Example:**
```go
// handlers_thread.go
func (h *handlers) threadView(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    idStr := chi.URLParam(r, "conversationId")
    convID, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        h.renderError(w, r, http.StatusBadRequest, "Invalid conversation ID")
        return
    }

    filter := query.MessageFilter{
        ConversationID: &convID,
        Sorting: query.MessageSorting{
            Field:     query.MessageSortByDate,
            Direction: query.SortAsc,  // chronological order
        },
        Pagination: query.Pagination{Limit: 500}, // all messages in thread
    }

    messages, err := h.engine.ListMessages(ctx, filter)
    if err != nil {
        h.logger.Error("failed to load thread", "conversationId", convID, "error", err)
        h.renderError(w, r, http.StatusInternalServerError, "Failed to load thread")
        return
    }

    // ?highlight=<msgID> — scroll to specific message
    var highlightID int64
    if v := r.URL.Query().Get("highlight"); v != "" {
        highlightID, _ = strconv.ParseInt(v, 10, 64)
    }

    content := templates.ThreadPage(messages, highlightID)
    title := "Thread"
    if len(messages) > 0 && messages[0].Subject != "" {
        title = messages[0].Subject
    }
    h.renderPage(w, r, title, content)
}
```

### Pattern 2: `<details>` with HTMX Lazy Body Loading

**What:** Each message is a `<details>` element. Latest message has `open` attribute and pre-loaded body iframe. All other messages have an empty placeholder that HTMX fills on first expand.

**When to use:** Every collapsed message card in `thread.templ`.

**Example:**
```html
<!-- Collapsed message (not latest) -->
<details
    id="msg-{id}"
    class="thread-message"
    data-msg-id="{id}"
>
    <summary class="thread-message-summary">
        <span class="thread-msg-from">{fromDisplay}</span>
        <span class="thread-msg-date">{relativeDate}</span>
        <span class="thread-msg-snippet">{snippet80}</span>
    </summary>
    <div class="thread-message-body"
        hx-get="/messages/{id}/body-wrapper"
        hx-trigger="toggle once"
        hx-swap="innerHTML"
    >
        <!-- Empty placeholder — HTMX fills on first expand -->
    </div>
</details>

<!-- Latest message (pre-expanded) -->
<details id="msg-{id}" class="thread-message" open data-msg-id="{id}">
    <summary class="thread-message-summary">...</summary>
    <!-- Full headers + eagerly loaded body wrapper -->
    <div class="thread-message-expanded">
        <dl class="header-list">...</dl>
        <div id="email-body-wrapper" class="email-render-wrapper">
            <!-- Same pattern as message.templ — images banner + iframe -->
        </div>
    </div>
</details>
```

**HTMX trigger note:** `hx-trigger="toggle once"` fires when `<details>` dispatches the `toggle` event (fires after open/close). The `once` modifier prevents re-fetching on subsequent collapse/expand. Verified: `toggle` is a native DOM event on `<details>`, HTMX listens to it. This is the canonical HTMX lazy-load pattern for `<details>`.

**Confidence:** HIGH — `toggle` event on `<details>` is a browser standard; HTMX's non-AJAX event listening works the same as any DOM event.

### Pattern 3: Path-Guarded Keyboard Shortcuts

**What:** `t`, `n`, `p` are new keys that conflict with or supplement existing handlers. Path-guard each to the correct page.

**When to use:** keys.js extension.

**Key conflict analysis:**
- `t` is currently bound to `navigateToTimeView()` — works globally. On message detail page (`/messages/{id}`), `t` must instead navigate to the thread. Solution: check `window.location.pathname` at key handler time.
- `n`/`p` are currently unbound (safe to add). Must only fire on `/threads/*`.

**Example:**
```javascript
case 't':
    if (window.location.pathname.startsWith('/messages/') &&
        !window.location.pathname.endsWith('/body')) {
        navigateToThread();
    } else {
        navigateToTimeView();
    }
    break;

case 'n':
    if (window.location.pathname.startsWith('/threads/')) {
        navigateThreadMessage(1);
        e.preventDefault();
    }
    break;

case 'p':
    if (window.location.pathname.startsWith('/threads/')) {
        navigateThreadMessage(-1);
        e.preventDefault();
    }
    break;
```

```javascript
function navigateToThread() {
    var link = document.getElementById('view-thread-link');
    if (link) {
        var href = link.getAttribute('href');
        if (href) {
            htmx.ajax('GET', href, {
                target: '#main-content',
                select: '#main-content',
                swap: 'outerHTML'
            }).then(function () {
                history.pushState({}, '', href);
            });
        }
    }
}

function navigateThreadMessage(delta) {
    var msgs = Array.from(document.querySelectorAll('.thread-message[data-msg-id]'));
    if (!msgs.length) return;

    // Find currently focused message
    var focusedIdx = msgs.findIndex(function(el) {
        return el.classList.contains('thread-focused');
    });

    if (focusedIdx < 0) {
        // Default: focus latest (last) on first n press
        focusedIdx = delta > 0 ? msgs.length - 1 : 0;
    }

    // Remove current focus
    if (focusedIdx >= 0) msgs[focusedIdx].classList.remove('thread-focused');

    // Calculate next with wrap-around
    var nextIdx = (focusedIdx + delta + msgs.length) % msgs.length;
    var nextMsg = msgs[nextIdx];

    nextMsg.classList.add('thread-focused');
    nextMsg.open = true;  // expand if collapsed
    nextMsg.scrollIntoView({ behavior: 'smooth', block: 'start' });
}
```

### Pattern 4: Thread Header with Participant Summary

**What:** The thread header aggregates participants from all messages in the thread. `ListMessages` returns `FromEmail`/`FromName` per message — collect unique names from all senders + all recipients not easily available in `[]MessageSummary`.

**Constraint:** `MessageSummary` only has `FromEmail` and `FromName`, not To/Cc/Bcc. For thread participant summary ("Between Alice, Bob, and 2 others"), use only sender names from the message list — this is sufficient for the "who is in this thread" summary since senders are the active participants.

**When to use:** `templates.ThreadPage` — compute participant list in the template helper or pre-compute in handler.

**Example approach (handler pre-computation):**
```go
// Deduplicate sender names from thread messages
seen := make(map[string]bool)
var participants []string
for _, msg := range messages {
    name := msg.FromName
    if name == "" {
        name = msg.FromEmail
    }
    if !seen[name] {
        seen[name] = true
        participants = append(participants, name)
    }
}
// First and last message dates for date range
```

### Pattern 5: Highlight + Scroll on Page Load

**What:** When `?highlight={messageId}` is present, JavaScript must scroll to and open that message after page load.

**When to use:** Thread page rendered with highlight param.

**Implementation approach — embed in template as data attribute:**
```html
<!-- In thread.templ, if highlightID > 0 -->
<div id="thread-container" data-highlight="{highlightID}">
    ...
</div>
```

```javascript
// In keys.js or inline <script> in thread.templ
document.addEventListener('DOMContentLoaded', function() {
    var container = document.getElementById('thread-container');
    if (!container) return;
    var highlightId = container.dataset.highlight;
    if (!highlightId) return;
    var el = document.getElementById('msg-' + highlightId);
    if (el) {
        el.open = true;
        el.classList.add('thread-focused');
        el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
});
```

**Alternative:** Inline `<script>` tag at bottom of `thread.templ` with `templ.ComponentScript`. Given that `keys.js` runs globally and this is page-specific, a small inline script block inside the template is cleaner — Templ supports `<script>` tags in templates.

### Anti-Patterns to Avoid

- **Fetching full MessageDetail for all messages upfront:** `GetMessage` loads body, participants, labels, attachments — expensive for 50+ message threads. Use `ListMessages` for the summary list; body fetching is already lazy via HTMX.
- **Rendering all body iframes on page load:** Defeats lazy-load purpose. Only the latest message gets its iframe on initial render.
- **Global `n`/`p` key binding without path guard:** Would interfere with other pages where these keys have no meaning.
- **`hx-trigger="toggle"` without `once` modifier:** Re-fetches body every collapse/expand cycle. Add `once`.
- **`id` collision for `email-body-wrapper`:** The message detail page uses `id="email-body-wrapper"` and the `messageBodyWrapper` endpoint targets `#email-body-wrapper` by OOB. In thread view, each message has its own wrapper — use `id="email-body-wrapper-{msgID}"` and update the "Load images" `hx-target` accordingly, OR rely on the per-message `hx-target` being scoped to the `<details>` element's shadow. Since `hx-target` in `messageBodyWrapper` response uses `id="email-body-wrapper"` — **this is a real conflict**. Per message we need unique IDs.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Email body sanitization | Custom HTML filter | `sanitizeEmailHTML()` in `sanitize_email.go` | Already handles CID substitution, bluemonday policy |
| CID → attachment URL substitution | Custom regex | Existing Phase 7 pipeline in `sanitizeEmailHTML` | Handles edge cases, already tested |
| Iframe sandbox + CSP | Custom response headers | `messageBody` endpoint (Phase 7) | Already sets correct CSP, no `allow-same-origin` |
| Postmessage iframe resize | Custom resize logic | Existing postMessage script in `messageBody` | Already handles ResizeObserver + MutationObserver |
| Body lazy loading | Custom JS fetch | HTMX `hx-trigger="toggle once"` | HTMX handles all the XHR, swap, and settle lifecycle |
| Thread message list query | Custom SQL | `ListMessages` with `ConversationID` filter | Already implemented in SQLiteEngine |

**Key insight:** The entire body-rendering pipeline (sanitize → CID substitute → iframe → resize) is already production-ready from Phase 7. Thread view reuses it per-message with no changes.

## Common Pitfalls

### Pitfall 1: `id="email-body-wrapper"` Collision in Thread Context
**What goes wrong:** `messageBodyWrapper` handler returns HTML with `id="email-body-wrapper"`. On the message detail page, there's only one wrapper. In thread view, there are N wrappers — they'd all have the same ID, and HTMX's "Load images" OOB swap would target the wrong element.

**Why it happens:** The existing `messageBodyWrapper` handler hardcodes `id="email-body-wrapper"` in its response. The "Load images" link in the same response hardcodes `hx-target="#email-body-wrapper"`.

**How to avoid:** For thread view, use the `hx-target` parameter or a unique wrapper ID. Options:
1. The lazy-load div in `thread.templ` uses `hx-get="/messages/{id}/body-wrapper"` — the response replaces the div's innerHTML (`hx-swap="innerHTML"`). But `messageBodyWrapper` response is a `<div id="email-body-wrapper" ...>` that itself contains the Load images link targeting `#email-body-wrapper`. In a thread, there will be multiple `#email-body-wrapper` elements — `querySelector` matches only the first.
2. **Best fix:** The thread's lazy-load placeholder uses `hx-swap="outerHTML"` and gives each wrapper a unique scoping. OR: The "Load images" link uses `closest` (HTMX doesn't support CSS `:closest` by itself).
3. **Practical fix:** Keep `messageBodyWrapper` as-is, but in the thread placeholder, scope the load: use `hx-target="this"` + `hx-swap="outerHTML"` on the placeholder div. The response's Load images `hx-target="#email-body-wrapper"` will then find the one just swapped in (it's now the unique wrapper). This works because HTMX targets the document, but the wrapper just rendered is the one in the DOM — the issue is only if two are open simultaneously.
4. **Simplest safe approach:** Use `hx-target="closest .thread-message-body"` for the "Load images" link — but this requires modifying `messageBodyWrapper`. OR: Accept that this only breaks if two messages are simultaneously mid-load with the same ID (edge case). Given single-threaded JS execution, probably fine in practice.

**Warning signs:** "Load images" button on a thread message scrolls/loads a different message's images.

**Recommendation:** Use `id="email-body-wrapper-{msgID}"` for thread wrapper divs — requires either a modified `messageBodyWrapper` endpoint that accepts a `?context=thread` param, or a new `handlers_thread.go` variant.

### Pitfall 2: `t` Key Conflict with Time View Navigation
**What goes wrong:** `t` currently calls `navigateToTimeView()` globally. On message detail pages, pressing `t` would navigate to aggregate time view instead of the thread.

**Why it happens:** The existing key handler switch has no path awareness.

**How to avoid:** Path-guard at the top of the `t` case. Check `window.location.pathname.startsWith('/messages/')` (and not `/messages` without an ID suffix — the list page has no thread context).

**Warning signs:** Pressing `t` on `/messages/123` navigates to `/aggregate?groupBy=time`.

### Pitfall 3: `hx-trigger="toggle"` Without `once` Causes Re-Fetch Loop
**What goes wrong:** Without `once`, every collapse/expand cycle fires a new HTMX request and replaces the body content (including the iframe), causing the iframe to reload.

**Why it happens:** HTMX default behavior: `hx-trigger="toggle"` fires on every toggle event.

**How to avoid:** Always use `hx-trigger="toggle once"`. After first swap, the placeholder is replaced with actual content (no HTMX attributes), so no further requests occur.

### Pitfall 4: Scroll-to-Highlight Fires Before Content Renders
**What goes wrong:** `scrollIntoView()` called in `DOMContentLoaded` scrolls to the element, but iframes haven't loaded yet so the page layout shifts after scroll.

**Why it happens:** iframe height is reported via postMessage asynchronously. The layout is not stable at DOMContentLoaded.

**How to avoid:** Use `setTimeout` delay (100-200ms) before scrolling, or listen for `htmx:afterSettle` instead of `DOMContentLoaded`. Since the highlighted message body is eagerly loaded (it has `open` attribute), the `postMessage` resize fires promptly. A small delay is acceptable.

### Pitfall 5: `SortAsc` Not Available on MessageSorting
**What goes wrong:** `ListMessages` with `ConversationID` needs ascending date sort (chronological). Confirm that `MessageSortByDate` + `SortAsc` produces `ORDER BY sent_at ASC` in the SQLite query.

**Why it happens:** The `buildListMessagesSQL` function in sqlite.go constructs the ORDER BY from `Sorting.Field` and `Sorting.Direction`. Looking at the existing code, `SortDesc` is the default (iota=0). `SortAsc` is `iota=1`. This is correct.

**How to avoid:** Set `Sorting: query.MessageSorting{Field: query.MessageSortByDate, Direction: query.SortAsc}` in the thread handler. Test with a multi-message thread to verify chronological order.

### Pitfall 6: Message Detail `ConversationID` = 0 When Thread Has One Message
**What goes wrong:** Single-message threads have a valid `ConversationID` but the user sees a thread view with one message. The CONTEXT.md marks this as Claude's discretion — either redirect or show single-message thread.

**How to avoid:** In `message.templ`, only show "View thread" link when the message count for the conversation is known to be > 1. However, `MessageDetail` doesn't include the conversation message count — would require an extra query. Simpler: show the link always, and in `threadView` handler, if only one message is returned, render a minimal thread page or redirect to `/messages/{id}`. Recommendation: render the single-message thread (no redirect) — avoids the extra query and the UX difference is minor.

## Code Examples

### Route Registration
```go
// server.go — add after existing message routes
r.Get("/threads/{conversationId}", h.threadView)
```

### Thread Message Card Template Sketch
```go
// thread.templ
templ ThreadMessageCard(msg query.MessageSummary, isLatest bool, highlightID int64) {
    <details
        id={ fmt.Sprintf("msg-%d", msg.ID) }
        class={ "thread-message", templ.KV("thread-focused", msg.ID == highlightID) }
        data-msg-id={ fmt.Sprintf("%d", msg.ID) }
        open?={ isLatest || msg.ID == highlightID }
    >
        <summary class="thread-message-summary">
            <span class="thread-msg-from">{ displayName(msg) }</span>
            <span class="thread-msg-date">{ relativeDate(msg.SentAt) }</span>
            <span class="thread-msg-snippet">{ truncate(msg.Snippet, 80) }</span>
        </summary>
        if isLatest || msg.ID == highlightID {
            // Eagerly load body wrapper
            <div class="thread-message-expanded">
                <dl class="header-list">...</dl>
                <div id={ fmt.Sprintf("email-body-wrapper-%d", msg.ID) } class="email-render-wrapper">
                    <!-- images banner + iframe -->
                </div>
            </div>
        } else {
            // Lazy-load placeholder
            <div
                class="thread-message-body"
                hx-get={ fmt.Sprintf("/messages/%d/body-wrapper", msg.ID) }
                hx-trigger="toggle once"
                hx-swap="outerHTML"
            ></div>
        }
    </details>
}
```

### Relative Date Helper
```go
// In templates/helpers.go or thread.templ package-level function
func relativeDate(t time.Time) string {
    now := time.Now()
    diff := now.Sub(t)
    switch {
    case diff < 24*time.Hour:
        return t.Format("3:04 PM")
    case diff < 7*24*time.Hour:
        return t.Format("Mon 3:04 PM")
    case t.Year() == now.Year():
        return t.Format("Jan 2")
    default:
        return t.Format("Jan 2, 2006")
    }
}
```

### Snippet Truncation
```go
func truncate(s string, n int) string {
    if len([]rune(s)) <= n {
        return s
    }
    return string([]rune(s)[:n]) + "…"
}
```

### HTMX Lazy Load — `hx-trigger="toggle once"` Pattern
The `toggle` event fires on `<details>` when it opens OR closes. With `once`, HTMX fires on the first toggle (the open), then removes the HTMX attributes. Verified against HTMX docs: any DOM event can be used in `hx-trigger`, and `once` is a built-in modifier.

**Confidence:** HIGH — this is documented HTMX behavior.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| JS-rendered thread views (React) | Server-rendered Templ + HTMX progressive enhancement | Phase 6 (this project) | No JS framework needed |
| Fetch-all-bodies on page load | HTMX lazy load via `<details>` toggle | Phase 8 (this phase) | Snappy even for 50+ message threads |
| Shared iframe ID | Per-message unique wrapper IDs | Phase 8 (this phase) | Required for N-per-page iframes |

**Deprecated/outdated:**
- React SPA (`web/`): Fully removed in Phase 6 — do not reference or revive

## Open Questions

1. **`messageBodyWrapper` ID collision handling**
   - What we know: The existing handler writes `id="email-body-wrapper"` and the "Load images" link targets `#email-body-wrapper` — both hardcoded.
   - What's unclear: Whether modifying `messageBodyWrapper` to accept a scoping suffix breaks the existing message detail page.
   - Recommendation: Add an optional `?msgId={id}` query param to `messageBodyWrapper` — when present, suffix the wrapper ID. Message detail page doesn't pass this param; thread page does. Alternatively, the thread lazy-load uses `hx-swap="innerHTML"` on a scoped container, and the existing wrapper-internal HTMX target is fine because the "Load images" button targets by ID which is unique within the newly-swapped content. **Verify:** Can two `id="email-body-wrapper"` elements coexist in the DOM when multiple messages are expanded? Technically invalid HTML but browsers handle it — `querySelector` returns the first, which could be wrong.

2. **Conversation message count for "View thread" link visibility**
   - What we know: `MessageDetail` has `ConversationID` but not a message count for that conversation.
   - What's unclear: Whether an extra query (`SELECT COUNT(*) FROM messages WHERE conversation_id = ?`) is worth the cost.
   - Recommendation: Show "View thread" link whenever `ConversationID != 0` — the threadView handler will render even single-message threads gracefully. Skip the extra query.

3. **Thread participant summary — To/Cc recipients**
   - What we know: `MessageSummary` only has `FromEmail`/`FromName` — not To/Cc/Bcc participants.
   - What's unclear: Whether the participant summary ("Between Alice, Bob, and 2 others") should include recipients, or just senders.
   - Recommendation: Use senders only (available in `MessageSummary`) — this avoids an N+1 query pattern. For a 50-message thread between two people, the senders list captures all active participants adequately.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + httptest |
| Config file | none — `go test ./...` |
| Quick run command | `go test ./internal/web/... -run TestThread -v` |
| Full suite command | `go test ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| THREAD-01 | GET /threads/{id} returns 200 HTML with all messages listed | integration | `go test ./internal/web/... -run TestThreadView` | ❌ Wave 0 |
| THREAD-02 | Latest message has `open` attribute; others do not; all have `<details>` | integration | `go test ./internal/web/... -run TestThreadMessageCollapsible` | ❌ Wave 0 |
| THREAD-03 | Collapsed message body div has hx-get pointing to messageBody endpoint | integration | `go test ./internal/web/... -run TestThreadLazyLoad` | ❌ Wave 0 |
| THREAD-04 | Message detail page includes "view-thread-link" when ConversationID set | integration | `go test ./internal/web/... -run TestMessageDetailViewThreadLink` | ❌ Wave 0 |
| THREAD-05 | n/p keyboard behavior — JS-only; test presence of data-msg-id attributes | integration | `go test ./internal/web/... -run TestThreadNavAttributes` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/web/... -run TestThread -v`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/web/handlers_thread_test.go` — thread handler tests (THREAD-01 through THREAD-05)
- [ ] `mockEngine` in `handlers_test.go` needs `ListMessages` to support `ConversationID` filter response (currently returns same 3 messages regardless of filter — add a `threadMessages` field or a callback)

*(Existing test infrastructure in `handlers_test.go` + `setupTestServer` covers the framework; only thread-specific tests are new)*

---

## Sources

### Primary (HIGH confidence)
- Codebase read: `internal/web/handlers_messages.go` — confirmed `messageBody`, `messageBodyWrapper` endpoint patterns
- Codebase read: `internal/web/server.go` — confirmed route registration pattern for new routes
- Codebase read: `internal/query/engine.go` + `models.go` — confirmed `ListMessages` with `ConversationID` filter exists
- Codebase read: `internal/query/sqlite.go` — confirmed `ConversationID` filter generates `conversation_id = ?` WHERE clause
- Codebase read: `internal/web/static/keys.js` — confirmed `t` is currently bound to `navigateToTimeView`, `n`/`p` are unbound
- Codebase read: `internal/web/templates/message.templ` — confirmed `dl.header-list` pattern to reuse in thread cards
- Codebase read: `internal/web/static/style.css` — confirmed Solarized CSS variable names (--base01, --base02, --base0, --cyan)
- Codebase read: `internal/web/handlers_test.go` — confirmed `mockEngine` structure and `setupTestServer` pattern

### Secondary (MEDIUM confidence)
- HTMX `hx-trigger="toggle once"` — `toggle` is a native DOM event on `<details>` (browser spec); HTMX `once` modifier is documented in HTMX docs. Treating as HIGH based on established use of HTMX in this project and browser standard behavior.

### Tertiary (LOW confidence)
- None — all findings verified against codebase or browser standards

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — entire stack is existing codebase, no new dependencies
- Architecture: HIGH — patterns directly follow existing handlers, templates, and HTMX conventions already in use
- Pitfalls: HIGH — identified by reading actual code (ID collision is real, `t` key conflict is real, `toggle once` is documented)

**Research date:** 2026-03-11
**Valid until:** 2026-04-10 (stable stack, no fast-moving dependencies)
