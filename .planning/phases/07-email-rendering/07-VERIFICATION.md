---
phase: 07-email-rendering
verified: 2026-03-10T20:15:00Z
status: human_needed
score: 8/8 automated must-haves verified
re_verification: false
human_verification:
  - test: "Open a message with an HTML body in a browser and confirm the email renders inside an iframe without breaking Solarized Dark app layout (navbar, sidebar remain intact)"
    expected: "Email content displays in a white-background iframe with a subtle border; app chrome is unaffected"
    why_human: "CSS isolation between iframe and parent document cannot be verified programmatically"
  - test: "Confirm the amber 'External images blocked. [Load images]' banner appears above the iframe on first load"
    expected: "Banner styled in Solarized yellow (--yellow on --base02) is visible above the email iframe"
    why_human: "Visual appearance of styled HTML cannot be verified without a browser"
  - test: "Click 'Load images' and inspect the Network tab in DevTools"
    expected: "One XHR to /messages/{id}/body-wrapper?showImages=true occurs (HTMX outerHTML swap); the banner disappears; the iframe reloads with external images; NO full page reload occurs; NO direct iframe src change via JavaScript"
    why_human: "HTMX swap behavior and network-level distinction between XHR and full reload require a browser"
  - test: "Confirm iframe auto-resizes to email content height (no internal scrollbar visible)"
    expected: "Iframe height adjusts to fit email content; no scrollbar appears inside the iframe"
    why_human: "postMessage resize behavior is dynamic and runtime-dependent"
  - test: "Check a message with inline CID images — confirm they display as local images from /attachments/{id}/inline"
    expected: "Inline images render without broken-image icons; image URLs resolve to /attachments/N/inline"
    why_human: "Requires real archive data with CID attachments and a browser to observe"
---

# Phase 7: Email Rendering Verification Report

**Phase Goal:** HTML email bodies render correctly and securely — sanitized before reaching the browser, isolated in sandboxed iframes so email CSS cannot break application layout, with inline images resolved from local attachments and external images blocked by default
**Verified:** 2026-03-10T20:15:00Z
**Status:** human_needed (all automated checks passed; visual/browser verification still required per Plan 02 Task 3 checkpoint)
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Email HTML with `<script>` tags has scripts stripped after sanitization | VERIFIED | `TestSanitizeEmailHTML_ScriptStripping` passes; bluemonday policy does not whitelist `script` element |
| 2 | Email HTML with onclick/onerror event handlers has them stripped after sanitization | VERIFIED | `TestSanitizeEmailHTML_EventHandlerStripping` passes (onclick, onerror, onload subtests) |
| 3 | Email HTML tables, inline styles, and `<style>` blocks are preserved after sanitization | VERIFIED | `TestSanitizeEmailHTML_TablePreservation`, `TestSanitizeEmailHTML_InlineStylePreservation`, `TestSanitizeEmailHTML_StyleBlockPreservation` all pass |
| 4 | CID image references in email HTML are replaced with `/attachments/{id}/inline` URLs | VERIFIED | `TestSubstituteCIDImages_ReplacesWithLocalURL` and `TestSubstituteCIDImages_AngleBracketWrapped` pass |
| 5 | External http/https image sources are replaced with empty src when showImages=false | VERIFIED | `TestSanitizeEmailHTML_ShowImagesFalse_BlocksExternal` and `TestBlockExternalImages_HTTP/HTTPS` pass |
| 6 | External image sources are preserved when showImages=true | VERIFIED | `TestSanitizeEmailHTML_ShowImagesTrue_PreservesExternal` passes |
| 7 | Viewing a message with HTML body shows rendered email in a sandboxed iframe | VERIFIED | `message.templ` renders `<iframe ... sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox">` for `msg.BodyHTML != ""`; no `allow-same-origin`; `TestMessageBodyEndpoint` passes |
| 8 | HTMX-powered image toggle works via outerHTML swap (no full page reload, no JS src mutation) | VERIFIED | `TestMessageBodyWrapperEndpoint` confirms `hx-get`, `hx-target="#email-body-wrapper"`, `hx-swap="outerHTML"` attributes; `TestMessageBodyWrapperShowImages` confirms banner absent when `showImages=true` |

**Score:** 8/8 truths verified (automated)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/web/sanitize_email.go` | sanitizeEmailHTML pipeline, CID substitution, external image blocking, bluemonday email policy | VERIFIED | 159 lines; exports `sanitizeEmailHTML`, `substituteCIDImages`, `blockExternalImages`; `getEmailPolicy()` singleton via `sync.Once`; bluemonday `AllowUnsafe(true)` for style blocks |
| `internal/web/sanitize_email_test.go` | Unit tests covering RENDER-01, RENDER-03, RENDER-04 behaviors | VERIFIED | 198 lines (min 80 required); 13 test functions covering all sanitization behaviors; all pass |
| `internal/query/models.go` | AttachmentInfo with ContentID field | VERIFIED | Line 80: `ContentID string \`json:"contentId,omitempty"\`` present in `AttachmentInfo` struct |
| `internal/store/schema.sql` | content_id column in attachments CREATE TABLE | VERIFIED | Line 210: `content_id TEXT, -- MIME Content-ID for inline images` |
| `internal/web/handlers_messages.go` | messageBody and messageBodyWrapper handlers | VERIFIED | Both handlers present and substantive; messageBody calls `sanitizeEmailHTML`, sets CSP, writes standalone HTML with postMessage resize script; messageBodyWrapper returns HTMX fragment |
| `internal/web/server.go` | GET /messages/{id}/body and GET /messages/{id}/body-wrapper routes | VERIFIED | Lines 59-60: both routes registered |
| `internal/web/backfill.go` | Background CID backfill goroutine | VERIFIED | 143 lines; `BackfillContentIDs` function queries `content_id IS NULL`, re-parses raw MIME, updates via content_hash lookup |
| `internal/web/templates/message.templ` | Iframe + external images banner with HTMX hx-get on Load images link | VERIFIED | Lines 59-75: full iframe with `email-body-wrapper` div, `email-images-banner`, `hx-get`, `hx-target`, `hx-swap="outerHTML"`, sandbox attributes |
| `internal/web/static/style.css` | CSS for iframe, email banner, blocked image placeholder | VERIFIED | Lines 741-772: `.email-render-wrapper`, `.email-images-banner`, `.email-images-banner a`, `.email-iframe` all present |
| `internal/web/static/keys.js` | postMessage listener for iframe auto-resize | VERIFIED | Lines 3-11: `window.addEventListener('message', ...)` with `msgvault-resize` type check |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/web/sanitize_email.go` | `internal/query/models.go` | `att.ContentID` used in CID lookup map | VERIFIED | `att.ContentID` referenced at line 112 of sanitize_email.go in `cidToID` map construction |
| `internal/sync/sync.go` | `internal/store/messages.go` | `UpsertAttachment` passes `att.ContentID` | VERIFIED | Line 676: `s.store.UpsertAttachment(messageID, att.Filename, att.ContentType, storagePath, att.ContentHash, att.ContentID, len(att.Content))` |
| `internal/query/shared.go` | `internal/store/schema.sql` | `fetchAttachmentsShared` SELECTs `content_id` | VERIFIED | Line 122: `COALESCE(content_id, '')` in SELECT |
| `internal/web/handlers_messages.go` | `internal/web/sanitize_email.go` | `messageBody` calls `sanitizeEmailHTML` | VERIFIED | Line 56: `sanitized := sanitizeEmailHTML(msg.BodyHTML, msg.Attachments, showImages)` |
| `internal/web/templates/message.templ` | `internal/web/handlers_messages.go` | iframe src points to `/messages/{id}/body`; Load images uses `hx-get` to `/messages/{id}/body-wrapper` | VERIFIED | Lines 63-69 of message.templ confirm both routes referenced |
| `internal/web/static/keys.js` | `internal/web/handlers_messages.go` | postMessage listener receives resize events from body endpoint's injected script | VERIFIED | keys.js listens for `msgvault-resize`; body endpoint injects postMessage script with same type |
| `internal/web/backfill.go` | `internal/store/messages.go` | Backfill queries `content_id IS NULL` and updates via content_hash | VERIFIED | backfill.go line 21: `WHERE (a.content_id IS NULL OR a.content_id = '')`; line 116-120: UPDATE via content_hash |
| `cmd/msgvault/cmd/web.go` | `internal/web/backfill.go` | BackfillContentIDs goroutine launched on server start | VERIFIED | Line 67: `go web.BackfillContentIDs(store.DB(), webLogger)` |
| `internal/importer/ingest.go` | `internal/store/messages.go` | `UpsertAttachment` passes `att.ContentID` | VERIFIED | Line 305: `att.ContentID` as 6th argument |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| RENDER-01 | 07-01 | Email HTML bodies are sanitized server-side with bluemonday before rendering (XSS prevention) | SATISFIED | `sanitize_email.go` uses bluemonday; scripts/event handlers stripped; 13 passing unit tests |
| RENDER-02 | 07-02 | Email HTML bodies render in sandboxed iframes so email CSS cannot break application layout | SATISFIED (automated) / NEEDS HUMAN (visual) | iframe with `sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"` (no `allow-same-origin`); visual CSS isolation requires browser check |
| RENDER-03 | 07-01 | CID image references in emails are substituted with local attachment URLs server-side | SATISFIED | `substituteCIDImages` function; `content_id` column flows from MIME parse through store to query layer; `TestSubstituteCIDImages_*` passes |
| RENDER-04 | 07-01, 07-02 | External images in emails are blocked by default with an opt-in toggle to load them | SATISFIED (automated) / NEEDS HUMAN (visual toggle) | `blockExternalImages` blocks by default; `/messages/{id}/body-wrapper` provides HTMX toggle; CSP header restricts `img-src`; visual behavior requires browser check |

All 4 requirements (RENDER-01, RENDER-02, RENDER-03, RENDER-04) are claimed by Plan 07-01 and/or 07-02. No orphaned requirements found — REQUIREMENTS.md maps only these 4 IDs to Phase 7.

### Anti-Patterns Found

No anti-patterns detected in phase 07 files:

- No TODO/FIXME/placeholder comments in `sanitize_email.go`, `handlers_messages.go`, or `backfill.go`
- No empty implementations or stub returns
- The old HTML body placeholder (`"HTML body available -- full rendering coming in a future update."`) has been removed from `message.templ`
- `go vet ./internal/web/... ./internal/store/... ./internal/query/... ./internal/sync/... ./internal/importer/...` is clean

Note: Pre-existing `go vet` errors exist in `internal/mbox`, `internal/export`, and `cmd/msgvault/cmd` — confirmed out of scope for this phase (present before Phase 7 work).

### Human Verification Required

#### 1. Email Renders in Sandboxed Iframe (RENDER-02)

**Test:** Build the binary (`make build`) and open `http://localhost:PORT/messages`, click a message with an HTML body (marketing email, newsletter, etc.)
**Expected:** Email content displays in a white-background iframe with a 1px border; app chrome (navbar, sidebar, Solarized Dark theme) is completely unaffected by email CSS
**Why human:** CSS isolation between iframe and parent document cannot be verified programmatically — requires visual inspection

#### 2. Amber External Images Banner (RENDER-04)

**Test:** Navigate to an HTML email message detail page
**Expected:** An amber banner above the iframe reads "External images blocked." with a "Load images" link styled in Solarized yellow
**Why human:** Visual appearance of themed HTML cannot be verified without a browser

#### 3. HTMX Load Images Toggle (RENDER-04)

**Test:** Click "Load images" link while watching the DevTools Network tab
**Expected:** A single XHR to `/messages/{id}/body-wrapper?showImages=true` fires (HTMX outerHTML swap); the banner disappears; the iframe reloads with external images; there is NO full page reload and NO JavaScript iframe src mutation
**Why human:** Distinguishing XHR from full reload and observing the DOM swap requires a browser with DevTools

#### 4. Iframe Auto-Resize (RENDER-02)

**Test:** View an HTML email with varying content length; observe iframe height
**Expected:** Iframe height adjusts automatically to fit email content height; no scrollbar appears inside the iframe
**Why human:** postMessage resize behavior is a runtime dynamic interaction that cannot be verified statically

#### 5. CID Image Resolution (RENDER-03)

**Test:** Find a message with inline CID images in the archive (e.g., email with inline logo); view message detail
**Expected:** Inline images display without broken-image icons; checking the image src in DevTools shows `/attachments/N/inline` URLs
**Why human:** Requires real archive data with CID-bearing attachments and browser observation

### Gaps Summary

No gaps. All automated must-haves are verified. All artifacts exist, are substantive, and are wired. All 4 requirements have implementation evidence. The phase is in human_needed state because Plan 02 explicitly included a blocking human checkpoint (Task 3) for visual verification — this is expected and by design.

---

_Verified: 2026-03-10T20:15:00Z_
_Verifier: Claude (gsd-verifier)_
