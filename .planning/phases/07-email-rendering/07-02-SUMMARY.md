---
phase: 07-email-rendering
plan: 02
subsystem: ui
tags: [htmx, iframe, sanitization, bluemonday, templ, csp, postmessage, backfill]

# Dependency graph
requires:
  - phase: 07-01
    provides: sanitizeEmailHTML, substituteCIDImages, blockExternalImages, AttachmentInfo.ContentID, query.MessageDetail.BodyHTML

provides:
  - GET /messages/{id}/body — sanitized standalone HTML with CSP headers and postMessage resize script
  - GET /messages/{id}/body-wrapper — HTMX-swappable fragment for external images toggle
  - BackfillContentIDs goroutine for populating missing content_id on attachments
  - Sandboxed iframe rendering in message detail template
  - External images banner with HTMX outerHTML swap (no JS src mutation)
  - postMessage iframe auto-resize listener in keys.js
  - CSS for email-render-wrapper, email-images-banner, email-iframe (Solarized Dark themed)

affects: [phase-08, phase-09]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Standalone HTML endpoint pattern: body endpoint serves complete HTML doc (not layout-wrapped), suitable for iframe src"
    - "HTMX outerHTML swap for partial replacement: hx-target outer wrapper, server returns new wrapper with updated content"
    - "Sandboxed iframe XSS defense: allow-scripts + allow-popups + allow-popups-to-escape-sandbox, explicitly NO allow-same-origin"
    - "postMessage resize protocol: iframe sends {type:'msgvault-resize', height:N}, parent listener resizes iframe height"
    - "Background backfill goroutine: non-blocking server start, processes stale DB records from raw MIME"

key-files:
  created:
    - internal/web/backfill.go
  modified:
    - internal/web/handlers_messages.go
    - internal/web/server.go
    - internal/web/templates/message.templ
    - internal/web/templates/message_templ.go
    - internal/web/static/style.css
    - internal/web/static/keys.js
    - cmd/msgvault/cmd/web.go
    - internal/web/handlers_test.go

key-decisions:
  - "HTMX hx-get outerHTML swap on Load images (not JS src mutation): hx-target=#email-body-wrapper, hx-swap=outerHTML, server returns new wrapper with showImages iframe"
  - "Standalone HTML for body endpoint: messageBody writes complete <!DOCTYPE html>...<body>... document, no layout template, designed to be iframe src"
  - "CSP on body endpoint restricts img-src to self+data (default) or * (showImages=true); X-Frame-Options omitted to allow parent framing"
  - "Backfill uses content_hash as join key to map MIME attachment ContentID to DB attachment rows"
  - "HTML body renders first (iframe), plain text fallback, then empty message (priority order change from plan 01)"

patterns-established:
  - "Standalone endpoint pattern: serve complete HTML doc via fmt.Fprintf, no templ layout, for iframe-only pages"
  - "HTMX outerHTML wrapper swap: return full wrapper div from server, client replaces outer element entirely"
  - "Background goroutine for DB maintenance: launch with go func(), log errors, non-blocking"

requirements-completed: [RENDER-02, RENDER-04]

# Metrics
duration: 8min
completed: 2026-03-11
---

# Phase 07 Plan 02: Email Rendering Pipeline Summary

**Sandboxed iframe email rendering with HTMX-powered external images toggle, postMessage auto-resize, CSP headers, and background CID backfill goroutine**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-11T06:26:00Z
- **Completed:** 2026-03-11T06:34:06Z
- **Tasks:** 3 (including auto-approved checkpoint)
- **Files modified:** 9

## Accomplishments

- `/messages/{id}/body` endpoint serves sanitized standalone HTML with CSP headers (img-src restricted by showImages param) and injected postMessage resize script
- `/messages/{id}/body-wrapper` endpoint returns HTMX-swappable fragment — default has amber external images banner with hx-get/hx-target/hx-swap="outerHTML", showImages=true returns iframe only (no banner)
- Message detail template now renders HTML email in sandboxed iframe (allow-scripts allow-popups allow-popups-to-escape-sandbox, no allow-same-origin) with plain text fallback
- `BackfillContentIDs` goroutine wired into web command startup: re-parses raw MIME to populate missing content_id on attachments from existing archive data
- postMessage resize listener in keys.js auto-sizes iframe height as email content loads/changes
- CSS for email banner (amber Solarized yellow) and iframe (white background, base01 border) integrated into Solarized Dark theme

## Task Commits

1. **Task 1: Body endpoint handler, body-wrapper endpoint, and backfill goroutine** - `63bd7a3` (feat)
2. **Task 2: Iframe template, postMessage resize, CSS styles, backfill wiring** - `b4ce09b` (feat)
3. **Task 3: Visual verification** - auto-approved checkpoint

## Files Created/Modified

- `internal/web/handlers_messages.go` - Added messageBody and messageBodyWrapper handlers
- `internal/web/server.go` - Registered /messages/{id}/body and /messages/{id}/body-wrapper routes
- `internal/web/backfill.go` - BackfillContentIDs goroutine with MIME re-parse, zlib decompression, content_hash lookup
- `internal/web/templates/message.templ` - Replaced HTML placeholder with sandboxed iframe + HTMX external images banner
- `internal/web/templates/message_templ.go` - Regenerated by templ generate
- `internal/web/static/style.css` - Added email-render-wrapper, email-images-banner, email-iframe CSS
- `internal/web/static/keys.js` - Added postMessage msgvault-resize listener for iframe auto-resize
- `cmd/msgvault/cmd/web.go` - Wire BackfillContentIDs goroutine on server start
- `internal/web/handlers_test.go` - Updated mockEngine.GetMessage with BodyHTML+Attachments, added 6 new body endpoint tests

## Decisions Made

- HTMX outerHTML swap for "Load images": `hx-target="#email-body-wrapper"` `hx-swap="outerHTML"` replaces the outer wrapper div (containing banner + iframe) with a fresh wrapper from the server (iframe only, no banner). No JavaScript src mutation needed.
- CSP policy on body endpoint: `img-src 'self' data:` by default (blocks external), `img-src * data:` with showImages=true. X-Frame-Options intentionally omitted (endpoint is designed to be framed).
- Iframe sandbox: `allow-scripts allow-popups allow-popups-to-escape-sandbox` — explicitly no `allow-same-origin` for XSS defense (iframe JS cannot access parent DOM).
- Backfill uses `content_hash` as the join key between MIME-parsed attachments and DB rows (unique per-attachment fingerprint, already stored at sync time).
- HTML body display priority: BodyHTML first (iframe), BodyText fallback (pre), empty message last.

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

- `go vet ./...` reported pre-existing errors in unrelated packages (mbox, export, cmd/validation_test.go). Verified pre-existing by stashing changes. Out of scope per deviation rules — logged to deferred-items.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Email rendering pipeline complete: sanitized HTML in sandboxed iframe, HTMX image toggle, CID image resolution, auto-resize
- BackfillContentIDs goroutine handles existing archives with missing content_id data
- Phase 08 (next) can build on this rendering foundation

---
*Phase: 07-email-rendering*
*Completed: 2026-03-11*
