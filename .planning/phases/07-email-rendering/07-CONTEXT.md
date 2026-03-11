# Phase 7: Email Rendering - Context

**Gathered:** 2026-03-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Sanitize and sandbox HTML email bodies so they render correctly and securely. Serve inline attachments via CID substitution and block external images by default with an opt-in toggle. This phase does NOT include thread/conversation view (Phase 8) or text/HTML toggle (Phase 9).

</domain>

<decisions>
## Implementation Decisions

### External Image Blocking
- External images (http/https) blocked by default in rendered emails
- Blocked images show a placeholder icon with alt text if available (Thunderbird/Outlook style)
- Amber info banner above the email iframe: "External images blocked. [Load images]" using Solarized --yellow (#b58900) text/border on --base02 background
- Clicking "Load images" triggers HTMX hx-get with `?showImages=true` to the body endpoint, server re-sanitizes with images allowed, returns updated iframe content
- CID-resolved images (local attachments) always display regardless of external image blocking — they're already in the archive, no privacy risk

### Iframe Rendering
- Email HTML served via a separate endpoint: `/messages/{id}/body?showImages=false`
- Endpoint serves sanitized HTML as a standalone page, iframe loads via `src=` attribute
- Iframe auto-resizes to match content height via postMessage from iframe to parent — no internal scrollbar, page itself scrolls
- No height cap — let iframe grow to full content height regardless of email length
- Subtle 1px --base01 border around the iframe for clear visual separation from Solarized Dark app chrome
- Email body uses its own background (usually white) — not forced to Solarized Dark; iframe border provides visual boundary

### CID Image Resolution
- Add `content_id TEXT` column to the attachments table via schema migration
- Sync populates content_id going forward from `mime.Attachment.ContentID`
- Auto-backfill on web server start: detect attachments with NULL content_id, queue background re-parse of affected messages' raw MIME to populate
- Pre-sanitization replacement pipeline: replace all `cid:XXXX` src attributes with `/attachments/{id}/inline` URLs before bluemonday runs, so bluemonday sees local URLs

### Sanitization Policy
- Preserve email layout fidelity — allow tables, inline styles, `<style>` blocks, common formatting tags (b, i, span, div, p, h1-h6, ul, ol, li, a, img, br, hr, blockquote, table/tr/td/th/thead/tbody)
- Strip scripts, event handlers (onclick etc.), forms, iframes-within-iframes, object/embed tags
- `<style>` blocks preserved — iframe sandbox prevents CSS leaking to parent page
- All `<a>` tags get `target="_blank"` and `rel="noopener noreferrer"` injected during sanitization — links open in new tab
- Custom bluemonday policy (not UGCPolicy) — purpose-built for email HTML rendering

### Claude's Discretion
- Exact bluemonday policy construction details (which attributes to allow on which tags)
- PostMessage resize implementation specifics (debounce, MutationObserver vs ResizeObserver)
- Placeholder icon design for blocked images
- Auto-backfill progress reporting (silent vs log output)
- CSP headers on the body endpoint

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `/attachments/{id}/inline` route: Already exists in server.go:66, serves binary attachment files inline — used directly for CID-resolved image URLs
- `serveAttachment` handler: Content-hash based file resolution already working (handlers.go:89-136)
- `MessageDetail.BodyHTML`: Populated from query layer, ready to use
- `mime.Attachment.ContentID`: Already parsed during sync (mime/parse.go:45) — just not stored in DB
- `message_raw` table: Compressed raw MIME available for backfill re-parsing

### Established Patterns
- HTMX `hx-select` pattern for partials — server always returns full pages, HTMX extracts fragments client-side
- OOB swap pattern from Phase 6 deletion staging — can reuse for image toggle banner updates
- Solarized CSS custom properties defined in style.css
- `renderPage` centralizes account listing and deletion count on every page

### Integration Points
- `message.templ`: Currently shows placeholder for HTML bodies (line 61) — replace with iframe + banner
- New route needed: `/messages/{id}/body` for sanitized HTML endpoint
- `internal/store/schema.sql`: Migration needed to add `content_id` column to attachments table
- `internal/sync/sync.go`: Needs to populate `content_id` when storing attachments during sync
- `keys.js`: May need new shortcut for image toggle if desired

</code_context>

<specifics>
## Specific Ideas

- Pipeline: CID substitution -> bluemonday sanitize -> serve as standalone HTML page in iframe
- External image toggle is a "trust this email" action, not per-image — keeps it simple
- Auto-backfill should be non-blocking on server start — serve immediately, CID resolution improves as backfill completes

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 07-email-rendering*
*Context gathered: 2026-03-10*
