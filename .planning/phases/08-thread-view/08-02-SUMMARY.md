---
phase: 08-thread-view
plan: 02
subsystem: ui
tags: [htmx, templ, javascript, css, solarized, keyboard-shortcuts]

requires:
  - phase: 08-thread-view plan 01
    provides: ThreadPage, ThreadMessageCard, thread handler with highlight param, thread-focused class

provides:
  - "View thread link in message detail header (id=view-thread-link) when ConversationID != 0"
  - "Path-guarded t key: thread view on /messages/{id}, time view elsewhere"
  - "n/p keyboard shortcuts for thread message navigation with wrap-around on /threads/*"
  - "navigateToThread(), navigateThreadMessage(delta), setupThreadHighlight() functions"
  - "setupThreadHighlight: scrolls to highlighted message on load, idempotent via data-highlightApplied"
  - "Multi-iframe resize: matches sending contentWindow across all .email-iframe elements"
  - "Thread CSS with Solarized Dark: muted collapsed, full-contrast expanded, 2px cyan focus border"

affects:
  - "phase 09-polish: thread page fully styled and navigable, keyboard shortcuts complete"

tech-stack:
  added: []
  patterns:
    - "Path-guard keyboard shortcuts with window.location.pathname.match/startsWith before routing"
    - "One-time initialization guard: data-highlightApplied attribute prevents double scroll"
    - "Multi-iframe postMessage matching by contentWindow with fallback to getElementById"

key-files:
  created: []
  modified:
    - internal/web/templates/message.templ
    - internal/web/templates/message_templ.go
    - internal/web/static/keys.js
    - internal/web/static/style.css

key-decisions:
  - "View thread link always shows when ConversationID != 0 (no extra count query for single-message threads)"
  - "setupThreadHighlight uses data-highlightApplied guard so re-running on htmx:afterSettle is idempotent"
  - "Multi-iframe resize uses contentWindow matching (e.source), with getElementById fallback for legacy support"
  - "n/p wrap-around: (idx + delta + len) % len handles both ends cleanly"

patterns-established:
  - "Path-guard pattern: if (window.location.pathname.match(...)) for context-sensitive key bindings"
  - "Thread focus: classList.add('thread-focused') + el.open = true + scrollIntoView for expand-and-scroll"

requirements-completed: [THREAD-04, THREAD-05]

duration: 4min
completed: 2026-03-11
---

# Phase 08 Plan 02: Thread Navigation and Polish Summary

**Thread navigation loop complete: 'View thread' link on message detail, path-guarded t/n/p shortcuts, setupThreadHighlight scroll-on-load, multi-iframe resize, and full Solarized Dark thread CSS**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-11T07:19:44Z
- **Completed:** 2026-03-11T07:24:42Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- "View thread" link appears in message detail header whenever ConversationID is non-zero, using HTMX hx-get with outerHTML swap
- Keyboard shortcuts t/n/p fully wired: t is path-guarded (thread on /messages/{id}, time view elsewhere), n/p navigate and expand thread messages with wrap-around
- setupThreadHighlight runs on DOMContentLoaded and htmx:afterSettle with idempotency guard, scrolls to highlighted message after 150ms delay for iframe height to settle
- Thread CSS adds Solarized Dark theming: muted --base01 collapsed state, full-contrast --base0 expanded state, 2px --cyan left border on focused message, flex summary layout

## Task Commits

1. **Task 1: View thread link and keys.js keyboard shortcuts** - `ba0cc764` (feat)
2. **Task 2: Thread CSS styling** - `8fb4bfcd` (feat)

## Files Created/Modified

- `internal/web/templates/message.templ` - Added "View thread" link block when ConversationID != 0
- `internal/web/templates/message_templ.go` - Regenerated from message.templ
- `internal/web/static/keys.js` - Path-guarded t key, n/p handlers, navigateToThread, navigateThreadMessage, setupThreadHighlight, multi-iframe resize, updated help overlay
- `internal/web/static/style.css` - Thread View CSS section appended (thread-view, thread-header, thread-message, thread-message-summary, thread-msg-*, thread-link, loading-placeholder)

## Decisions Made

- View thread link renders for any non-zero ConversationID without a thread-size check — single-message threads show the link gracefully (per RESEARCH.md recommendation)
- setupThreadHighlight uses `data-highlightApplied` attribute as a one-time guard so calling it again on htmx:afterSettle is safe
- Multi-iframe resize uses `e.source` (contentWindow) matching across all `.email-iframe` elements, with `getElementById('email-body-frame')` fallback for robustness
- n/p wrap-around formula `(idx + delta + len) % len` handles both ends without conditionals

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

Pre-existing `go vet` failures in `internal/mbox`, `internal/export`, and `cmd/msgvault/cmd` test files (undefined function, wrong call arity, duplicate test name) confirmed present before this plan's changes. Not in scope. Logged to deferred-items tracking.

## Next Phase Readiness

- Full thread navigation loop is complete: message list → message detail → thread view → n/p navigation within thread
- Phase 09 polish can build on complete thread UX: all CSS classes, keyboard shortcuts, and data attributes are in place
- No blockers

---
*Phase: 08-thread-view*
*Completed: 2026-03-11*

## Self-Check: PASSED

- FOUND: 08-02-SUMMARY.md
- FOUND: internal/web/templates/message.templ
- FOUND: internal/web/static/keys.js
- FOUND: internal/web/static/style.css
- FOUND commit ba0cc764: feat(08-02): view thread link, t/n/p keyboard shortcuts, multi-iframe resize
- FOUND commit 8fb4bfcd: feat(08-02): thread CSS styling with Solarized Dark theme
