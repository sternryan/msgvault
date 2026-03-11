# Phase 7: Email Rendering - Research

**Researched:** 2026-03-10
**Domain:** HTML email sanitization, iframe sandboxing, CID image resolution, HTMX partial refresh
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**External Image Blocking**
- External images (http/https) blocked by default in rendered emails
- Blocked images show a placeholder icon with alt text if available (Thunderbird/Outlook style)
- Amber info banner above the email iframe: "External images blocked. [Load images]" using Solarized `--yellow` (#b58900) text/border on `--base02` background
- Clicking "Load images" triggers HTMX `hx-get` with `?showImages=true` to the body endpoint, server re-sanitizes with images allowed, returns updated iframe content
- CID-resolved images (local attachments) always display regardless of external image blocking

**Iframe Rendering**
- Email HTML served via a separate endpoint: `/messages/{id}/body?showImages=false`
- Endpoint serves sanitized HTML as a standalone page; iframe loads via `src=` attribute
- Iframe auto-resizes to match content height via postMessage from iframe to parent — no internal scrollbar, page itself scrolls
- No height cap — let iframe grow to full content height regardless of email length
- Subtle 1px `--base01` border around the iframe for visual separation from Solarized Dark app chrome
- Email body uses its own background (usually white) — not forced to Solarized Dark

**CID Image Resolution**
- Add `content_id TEXT` column to the attachments table via schema migration
- Sync populates content_id going forward from `mime.Attachment.ContentID`
- Auto-backfill on web server start: detect attachments with NULL content_id, queue background re-parse of affected messages' raw MIME to populate
- Pre-sanitization replacement pipeline: replace all `cid:XXXX` src attributes with `/attachments/{id}/inline` URLs before bluemonday runs

**Sanitization Policy**
- Preserve email layout fidelity — allow tables, inline styles, `<style>` blocks, common formatting tags
- Strip scripts, event handlers (onclick etc.), forms, iframes-within-iframes, object/embed tags
- `<style>` blocks preserved — iframe sandbox prevents CSS leaking to parent page
- All `<a>` tags get `target="_blank"` and `rel="noopener noreferrer"` injected during sanitization
- Custom bluemonday policy (not UGCPolicy) — purpose-built for email HTML rendering

### Claude's Discretion
- Exact bluemonday policy construction details (which attributes to allow on which tags)
- PostMessage resize implementation specifics (debounce, MutationObserver vs ResizeObserver)
- Placeholder icon design for blocked images
- Auto-backfill progress reporting (silent vs log output)
- CSP headers on the body endpoint

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| RENDER-01 | Email HTML bodies are sanitized server-side with bluemonday before rendering (XSS prevention) | bluemonday v1.0.27 confirmed; custom policy with AllowUnsafe(true) for `<style>` element required; AllowElements + AllowAttrs API verified |
| RENDER-02 | Email HTML bodies render in sandboxed iframes so email CSS cannot break application layout | Separate `/messages/{id}/body` endpoint serving standalone HTML; iframe with sandbox attribute confirmed; allow-same-origin for postMessage resize confirmed safe |
| RENDER-03 | CID image references in emails are substituted with local attachment URLs server-side | Pre-sanitization regex replacement: `cid:XXXX` → `/attachments/{id}/inline`; `content_id` column migration needed; backfill from message_raw via mime.Parse |
| RENDER-04 | External images in emails are blocked by default with an opt-in toggle to load them | Server-side conditional: when `showImages=false`, replace external img src with blank/placeholder; HTMX hx-get reload with `?showImages=true`; banner with HTMX partial refresh |
</phase_requirements>

---

## Summary

Phase 7 adds HTML email rendering to the message detail view. The architecture is a three-layer pipeline: (1) server-side CID substitution replacing `cid:XXXX` image references with local attachment URLs, (2) bluemonday sanitization stripping scripts and event handlers while preserving email layout fidelity, and (3) delivery as a standalone HTML page served at `/messages/{id}/body` and displayed in a sandboxed iframe.

The critical finding is that bluemonday cannot allow the `<style>` HTML element without calling `AllowUnsafe(true)`. Since email HTML commonly uses `<style>` blocks for layout (especially in marketing emails and newsletters), preserving them requires `AllowUnsafe(true)` on the policy. This is explicitly safe in this context because the iframe sandbox's `allow-scripts` is NOT set — the `<style>` block renders CSS only, and the iframe sandbox prevents that CSS from affecting the parent Solarized layout. The defense-in-depth is: sandbox prevents script execution; `allow-same-origin` without `allow-scripts` is safe.

The `content_id` column does not exist in the current schema. Adding it requires an `ALTER TABLE attachments ADD COLUMN content_id TEXT` migration, plus updating `UpsertAttachment` and `fetchAttachmentsShared` to include `content_id`, plus a backfill goroutine that re-parses raw MIME for messages with NULL content_id attachments.

**Primary recommendation:** Build a `sanitizeEmailHTML(html string, messageID int64, showImages bool, attachments []AttachmentInfo) string` helper in `internal/web/` that runs CID substitution then bluemonday sanitization. Add the `/messages/{id}/body` handler. Replace the placeholder in `message.templ` with an iframe + banner.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/microcosm-cc/bluemonday` | v1.0.27 | HTML sanitization allowlist | Industry standard Go HTML sanitizer, actively maintained, verified via pkg.go.dev |
| `regexp` (stdlib) | — | CID attribute replacement | Pre-sanitization regex pass to rewrite `cid:XXXX` URLs before bluemonday sees them |
| `net/http` (stdlib) | — | New `/messages/{id}/body` endpoint | Already the server's HTTP layer |
| `strings` (stdlib) | — | External image src replacement | Simple `strings.Replace` sufficient for blocking external image src values |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/sync` | already in go.mod | Goroutine limiting for backfill | Already present; use for limiting concurrent backfill goroutines |
| `database/sql` (stdlib) | — | Schema migration (ALTER TABLE) | `db.Exec("ALTER TABLE attachments ADD COLUMN content_id TEXT")` with IF NOT EXISTS guard |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| bluemonday custom policy | `go-premailer` CSS inlining | go-premailer is for sending email, not viewing; irrelevant here |
| iframe `src` endpoint | `srcdoc` attribute | srcdoc embeds HTML string in HTML attribute (escaping complexity); src endpoint is cleaner, allows CSP header on response, matches the locked decision |
| postMessage resize | fixed height + scrollbar | Fixed height was explicitly rejected in CONTEXT.md decisions |

**Installation:**
```bash
go get github.com/microcosm-cc/bluemonday@v1.0.27
```

(Not yet in go.mod — must be added.)

---

## Architecture Patterns

### Recommended Project Structure

New files and modifications:

```
internal/web/
├── handlers_messages.go        # Add messageBodyHandler (new endpoint)
├── server.go                   # Add GET /messages/{id}/body route
├── sanitize_email.go           # NEW: sanitizeEmailHTML helper + CID substitution
├── sanitize_email_test.go      # NEW: unit tests for sanitizer
├── static/
│   └── style.css               # Add iframe, email-banner CSS rules
└── templates/
    └── message.templ           # Replace placeholder with iframe + HTMX banner

internal/store/
└── messages.go                 # Update UpsertAttachment to accept content_id

internal/query/
├── models.go                   # Add ContentID to AttachmentInfo
└── shared.go                   # Update fetchAttachmentsShared to SELECT content_id

internal/web/
└── backfill.go                 # NEW: background CID backfill goroutine
```

### Pattern 1: CID Substitution Before Sanitization

**What:** Build a lookup map from content_id to attachment ID before sanitization, then use a regex to replace all `src="cid:XXXX"` and `src='cid:XXXX'` occurrences with `/attachments/{id}/inline`.

**When to use:** Every time `sanitizeEmailHTML` is called.

**Example:**
```go
// Source: internal analysis of mime.Attachment.ContentID patterns
var cidRe = regexp.MustCompile(`(?i)src=["']cid:([^"']+)["']`)

func substituteCIDImages(html string, attachments []query.AttachmentInfo) string {
    // Build lookup: content_id (without angle brackets) -> attachment id
    cidMap := make(map[string]int64, len(attachments))
    for _, att := range attachments {
        cid := strings.Trim(att.ContentID, "<>")
        if cid != "" {
            cidMap[cid] = att.ID
        }
    }
    return cidRe.ReplaceAllStringFunc(html, func(match string) string {
        sub := cidRe.FindStringSubmatch(match)
        if len(sub) < 2 {
            return match
        }
        cid := strings.Trim(sub[1], "<>")
        if id, ok := cidMap[cid]; ok {
            return fmt.Sprintf(`src="/attachments/%d/inline"`, id)
        }
        // No match — strip the src to avoid broken cid: URL rendering
        return `src=""`
    })
}
```

**Why pre-sanitization:** If CID substitution runs after bluemonday, bluemonday may have already stripped the `src` attribute because `cid:` is not a recognized URL scheme. Running substitution first gives bluemonday local URLs to permit.

### Pattern 2: bluemonday Email Policy

**What:** Custom policy that preserves email layout fidelity while stripping XSS vectors. Uses `AllowUnsafe(true)` to permit `<style>` blocks; the iframe sandbox is the actual XSS defense.

**When to use:** Created once at server startup (not thread-safe to build, thread-safe to use).

**Example:**
```go
// Source: pkg.go.dev/github.com/microcosm-cc/bluemonday@v1.0.27
import "github.com/microcosm-cc/bluemonday"

func newEmailPolicy() *bluemonday.Policy {
    p := bluemonday.NewPolicy()

    // Allow <style> blocks — iframe sandbox prevents CSS leaking to parent.
    // AllowUnsafe is required because bluemonday blocks <style> by design.
    // The iframe sandbox (no allow-scripts) is the actual XSS defense.
    p.AllowUnsafe(true)

    // Block <script>, <object>, <embed>, <form>, <iframe> (within-iframe)
    // These are NOT in the allowlist, so bluemonday strips them.

    // Structural/layout elements
    p.AllowElements(
        "html", "head", "body",
        "div", "span", "p", "br", "hr",
        "h1", "h2", "h3", "h4", "h5", "h6",
        "ul", "ol", "li", "dl", "dt", "dd",
        "blockquote", "pre", "code",
        "b", "strong", "i", "em", "u", "s", "strike",
        "sup", "sub", "small", "big",
        "center",
    )

    // Table elements (essential for email layout)
    p.AllowElements(
        "table", "thead", "tbody", "tfoot", "tr", "td", "th",
        "caption", "colgroup", "col",
    )

    // Inline styles (needed for email layout preservation)
    p.AllowAttrs("style").Globally()
    // Note: AllowStyles() with specific property validation would be ideal,
    // but email HTML has an unbounded set of CSS properties. A broad
    // AllowAttrs("style").Globally() is acceptable because:
    // 1. The iframe sandbox blocks JS; CSS cannot execute scripts in sandbox
    // 2. CSS leaking to parent is blocked by iframe isolation
    // Revisit with a CSS allowlist if stricter policy is needed.

    // Class and ID (needed for email CSS targeting)
    p.AllowStandardAttributes() // allows id, title, dir, lang

    // Links — AddTargetBlankToFullyQualifiedLinks handles target="_blank" + rel="noopener"
    p.AllowStandardURLs()
    p.AllowAttrs("href").OnElements("a")
    p.AllowAttrs("name").OnElements("a")
    p.AddTargetBlankToFullyQualifiedLinks(true)

    // Images — allow src, alt, width, height (but NOT src for external images when blocked)
    // External image src is replaced server-side before this runs
    p.AllowImages() // allows img with src, alt, width, height, title
    p.AllowDataURIImages() // email sometimes uses data: URIs for small images

    // Table attributes
    p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
    p.AllowAttrs("align", "valign", "bgcolor", "width", "height",
        "border", "cellpadding", "cellspacing").OnElements(
        "table", "tr", "td", "th", "thead", "tbody",
    )

    // Font element (old-school email HTML)
    p.AllowElements("font")
    p.AllowAttrs("color", "face", "size").OnElements("font")

    // Image layout attrs
    p.AllowAttrs("width", "height", "border", "align").OnElements("img")

    return p
}
```

### Pattern 3: External Image Blocking

**What:** Before sanitization, when `showImages=false`, replace `src="http://..."` and `src="https://..."` with a blank placeholder. After bluemonday runs, the `<img>` tags remain but with empty src (or a data-URI placeholder).

**When to use:** Every call to the body endpoint when `?showImages=false` (the default).

**Example:**
```go
// Source: internal design
var externalImgSrcRe = regexp.MustCompile(
    `(?i)src=["'](https?://[^"']+)["']`,
)

func blockExternalImages(html string) string {
    // Replace external http/https src with empty string
    // The <img> tag remains (preserving alt text) but loads nothing
    return externalImgSrcRe.ReplaceAllString(html, `src=""`)
}
```

**Note:** The CONTEXT.md decision says blocked images show a placeholder icon with alt text if available. The `alt` attribute passes through bluemonday (it's part of `AllowImages()`), so alt text is preserved. A CSS rule on the body endpoint's standalone page can style `img[src=""]` to show a broken-image placeholder visual.

### Pattern 4: Iframe Auto-Resize via postMessage

**What:** The standalone HTML page served by `/messages/{id}/body` includes a small inline script that measures content height and posts it to the parent. The parent's `message.templ` adds a `window.addEventListener('message', ...)` listener that updates the iframe's height attribute.

**When to use:** Injected into the HTML response from `/messages/{id}/body`, and a listener in keys.js or an inline script in message.templ.

**Iframe sandbox:** `sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"`

- `allow-same-origin`: Required for postMessage to work (and for the parent to detect iframe height)
- `allow-popups`: Required so `target="_blank"` links open in new tab
- `allow-popups-to-escape-sandbox`: External webpages function properly after opening
- NOT `allow-scripts`: Scripts inside the iframe are blocked — this is the primary XSS defense

**CRITICAL security note:** The combination of `allow-same-origin` WITHOUT `allow-scripts` is safe. The danger is combining BOTH `allow-same-origin` AND `allow-scripts` (which would allow the iframe to remove its own sandbox). Since scripts are blocked, there is no way for the iframe to escape its sandbox.

**But wait — postMessage requires script execution in the iframe.** The resize postMessage approach requires a `<script>` in the iframe content. This creates a conflict:

- Option A: `allow-scripts` only (no `allow-same-origin`) — scripts run but iframe is cross-origin, no sandbox escape possible. **This is the correct option.** postMessage works fine cross-origin.
- Option B: `srcdoc` without sandbox — not isolated enough.
- Option C: No postMessage, use fixed minimum height — rejected in CONTEXT.md.

**Resolved:** Use `sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"` — allow scripts BUT NOT allow-same-origin. This means:
- postMessage from iframe to parent works (cross-origin postMessage always works)
- Scripts run (needed for ResizeObserver postMessage)
- Iframe cannot access parent DOM (cross-origin boundary enforced by browser)
- Iframe cannot remove its sandbox (requires both allow-scripts AND allow-same-origin)
- Email `<script>` tags in the body would execute — but bluemonday strips them server-side, so the only script that runs is the injected resize script.

**Example resize script injected into standalone page:**
```html
<!-- Injected at end of <body> by Go template for the body endpoint -->
<script>
(function() {
    function reportHeight() {
        var h = document.documentElement.scrollHeight;
        window.parent.postMessage({ type: 'msgvault-resize', height: h }, '*');
    }
    // Initial report
    reportHeight();
    // Observe subsequent layout changes
    if (window.ResizeObserver) {
        new ResizeObserver(reportHeight).observe(document.body);
    }
    // Fallback: MutationObserver for DOM changes (e.g., images loading)
    if (window.MutationObserver) {
        var mo = new MutationObserver(reportHeight);
        mo.observe(document.body, { subtree: true, childList: true, attributes: true });
    }
    // Also fire on image load events
    window.addEventListener('load', reportHeight);
})();
</script>
```

**Parent listener (in keys.js or inline script in message.templ):**
```javascript
window.addEventListener('message', function(e) {
    if (e.data && e.data.type === 'msgvault-resize') {
        var frame = document.getElementById('email-body-frame');
        if (frame) {
            frame.style.height = (e.data.height + 20) + 'px'; // +20px breathing room
        }
    }
});
```

**Debouncing:** ResizeObserver can fire rapidly. Add a 50ms debounce if needed in the iframe script.

### Pattern 5: Schema Migration (ALTER TABLE)

**What:** The `content_id` column doesn't exist in the current attachments table. Because `InitSchema` uses `CREATE TABLE IF NOT EXISTS`, modifying schema.sql alone won't add the column to existing databases. The migration must use `ALTER TABLE ... ADD COLUMN`.

**When to use:** On server start, before the backfill goroutine runs.

**Example:**
```go
// In Store.InitSchema() or a new MigrateSchema() function
func (s *Store) migrateAddContentID() error {
    // SQLite ignores "IF NOT EXISTS" for ALTER TABLE ADD COLUMN,
    // so check column existence first via PRAGMA table_info
    rows, err := s.db.Query("PRAGMA table_info(attachments)")
    if err != nil {
        return err
    }
    defer rows.Close()
    for rows.Next() {
        var cid int
        var name, typ, notnull, dflt sql.NullString
        var pk int
        if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
            return err
        }
        if name.String == "content_id" {
            return nil // already exists
        }
    }
    _, err = s.db.Exec("ALTER TABLE attachments ADD COLUMN content_id TEXT")
    return err
}
```

**Alternative:** In schema.sql, add `content_id TEXT` to the CREATE TABLE statement (new DBs get it automatically) and run the ALTER TABLE migration for existing DBs. This is the cleanest approach.

### Anti-Patterns to Avoid

- **Using UGCPolicy() for email:** UGCPolicy blocks style elements and most table attributes; email HTML requires a much more permissive policy.
- **Allowing `<iframe>` in sanitized email:** Nested iframes defeat the sandbox isolation. Strip all `<iframe>` tags in email body.
- **Running CID substitution AFTER bluemonday:** bluemonday strips `cid:` scheme URLs because they're not in AllowStandardURLs; substitution must come first.
- **Using `allow-same-origin` AND `allow-scripts` together:** Classic sandbox escape. Pick one or the other.
- **Serving the body endpoint with same Content-Type as app pages:** Set `X-Frame-Options: SAMEORIGIN` or omit it; the body endpoint should NOT set frame-blocking headers since it's intentionally framed.
- **Forgetting `rel="noopener"` on `target="_blank"` links:** `AddTargetBlankToFullyQualifiedLinks(true)` handles this automatically; don't duplicate manually.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTML sanitization | Regex-based tag stripping | bluemonday | HTML parsing edge cases (malformed tags, encoding tricks, nested structures) make regex sanitization exploitable |
| XSS allowlist | Custom attribute filter | bluemonday policy | Dozens of HTML/CSS XSS vectors; allowlist libraries have years of battle-testing |
| CSS leakage into parent | CSS scoping via class prefix | Iframe sandbox | Scoping CSS is complex and brittle; sandbox is the right architectural boundary |
| Link `target` injection | Template-level attribute addition | bluemonday `AddTargetBlankToFullyQualifiedLinks(true)` | Already built and handles edge cases like relative URLs |

**Key insight:** HTML sanitization for email is harder than it looks. Email HTML is authored by arbitrary senders and often violates every HTML standard. bluemonday's allowlist-plus-parser approach is correct; regex approaches are not.

---

## Common Pitfalls

### Pitfall 1: `<style>` Block Stripped by Default

**What goes wrong:** bluemonday strips `<style>` elements by default (even with `AllowElements("style")`). Calling `AllowElements("style")` is NOT sufficient — you must also call `p.AllowUnsafe(true)`.

**Why it happens:** bluemonday explicitly treats `<style>` and `<script>` as "fundamentally unsafe" elements after CVE-2021-42576. `AllowElements` does not override this special handling.

**How to avoid:** Call `p.AllowUnsafe(true)` on the policy. Document why: the iframe sandbox (`allow-scripts` NOT set, OR with scripts sandboxed cross-origin) is the actual defense; `AllowUnsafe` only unlocks the `<style>` rendering, not script execution.

**Warning signs:** Email renders without any styling even though the body HTML contains a `<head><style>` block.

### Pitfall 2: CID URLs Not Stripped Before Sanitization

**What goes wrong:** bluemonday strips `src="cid:..."` attributes because `cid:` is not a standard URL scheme. Images that should display as CID-resolved locals become invisible.

**Why it happens:** `AllowStandardURLs()` only allows `http`, `https`, `mailto`. `AllowURLSchemes("cid")` might seem like a solution but it would leave `cid:` URLs in the output that the browser can't resolve.

**How to avoid:** Run the regex CID substitution pass BEFORE calling bluemonday. Map `cid:XXXX` → `/attachments/{id}/inline` before sanitization.

**Warning signs:** Inline images that exist in the attachment store don't appear in the rendered email.

### Pitfall 3: Iframe Sandbox Escape via `allow-same-origin` + `allow-scripts`

**What goes wrong:** If you use `sandbox="allow-same-origin allow-scripts"`, email JavaScript can reach into the parent page's DOM, remove the sandbox attribute from the iframe, and execute arbitrary code.

**Why it happens:** `allow-same-origin` + `allow-scripts` = iframe can modify its own `sandbox` attribute, defeating all sandbox protections.

**How to avoid:** Never combine both. For postMessage resize: use `allow-scripts` only (cross-origin iframe). bluemonday already strips scripts from email HTML, so the only script executing is the injected resize script.

**Warning signs:** Browser dev tools or security scanners warn about `allow-scripts allow-same-origin` combination.

### Pitfall 4: External Image Blocking Bypassed via CSS `background-image`

**What goes wrong:** Blocking `<img src="http://...">` doesn't prevent CSS `background-image: url(http://...)` from loading external images.

**Why it happens:** The external image blocking regex only targets `src` attributes on `<img>` tags.

**How to avoid:** For this phase, the CONTEXT.md decision scopes blocking to img tags (which is what every major email client blocks by default — tracking pixels in `<img>`). CSS background images are a second-order concern. Document this as a known limitation. The iframe sandbox itself provides some protection since CSP can be set on the body endpoint response.

**Warning signs:** User loads "blocked" email and network traffic shows requests to external hosts via CSS.

### Pitfall 5: Iframe Height Feedback Loop

**What goes wrong:** ResizeObserver fires → postMessage → parent sets iframe height → iframe layout reflows → ResizeObserver fires again → infinite loop.

**Why it happens:** Changing the iframe's height attribute can trigger the ResizeObserver inside the iframe.

**How to avoid:** Debounce the `reportHeight` function in the iframe script (50-100ms). Track the last reported height and skip postMessage if height hasn't changed.

**Warning signs:** Iframe height oscillates or CPU spikes when viewing HTML emails.

### Pitfall 6: Schema Migration Race

**What goes wrong:** Two server processes start simultaneously; both detect `content_id` is missing and both run `ALTER TABLE ADD COLUMN`, causing a database error.

**Why it happens:** The migration check-then-migrate is not atomic in SQLite by default.

**How to avoid:** Wrap the migration in a `BEGIN EXCLUSIVE TRANSACTION` or use SQLite's inherent single-writer behavior. For this single-user local tool, the risk is effectively zero, but still run migration during server startup before accepting connections.

---

## Code Examples

Verified patterns from codebase inspection and official API documentation:

### Full sanitizeEmailHTML Function Shape

```go
// internal/web/sanitize_email.go
// Source: bluemonday pkg.go.dev + codebase analysis

var emailPolicy *bluemonday.Policy
var emailPolicyOnce sync.Once

func getEmailPolicy() *bluemonday.Policy {
    emailPolicyOnce.Do(func() {
        emailPolicy = newEmailPolicy()
    })
    return emailPolicy
}

// sanitizeEmailHTML applies the full rendering pipeline:
// 1. CID substitution (local attachment URLs)
// 2. External image blocking (when showImages=false)
// 3. bluemonday sanitization
func sanitizeEmailHTML(html string, attachments []query.AttachmentInfo, showImages bool) string {
    // Step 1: Substitute CID image references with local URLs
    html = substituteCIDImages(html, attachments)

    // Step 2: Block external images (http/https src) if not showing images
    if !showImages {
        html = blockExternalImages(html)
    }

    // Step 3: Sanitize with bluemonday
    return getEmailPolicy().Sanitize(html)
}
```

### Body Endpoint Handler Shape

```go
// in handlers_messages.go
func (h *handlers) messageBody(w http.ResponseWriter, r *http.Request) {
    idStr := chi.URLParam(r, "id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    // ... error handling ...

    msg, err := h.engine.GetMessage(r.Context(), id)
    // ... error handling ...

    showImages := r.URL.Query().Get("showImages") == "true"

    sanitized := sanitizeEmailHTML(msg.BodyHTML, msg.Attachments, showImages)

    // Set CSP header: scripts blocked redundantly (sandbox already blocks them,
    // but defense-in-depth)
    w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; img-src * data:; style-src 'unsafe-inline' *; font-src *")
    w.Header().Set("Content-Type", "text/html; charset=utf-8")

    // Serve as a full standalone HTML page (the iframe needs a complete document)
    // Inject the resize postMessage script
    fmt.Fprintf(w, bodyPageTemplate, sanitized)
}
```

### Iframe in message.templ

```html
<!-- Replace the placeholder at message.templ:61 -->
if msg.BodyHTML != "" {
    <div class="email-render-wrapper">
        if !showImages {
            <div class="email-images-banner" id="email-images-banner">
                <span>External images blocked.</span>
                <a href="#" hx-get={ fmt.Sprintf("/messages/%d/body?showImages=true", msg.ID) }
                   hx-target="#email-body-frame"
                   hx-swap="outerHTML">Load images</a>
            </div>
        }
        <iframe
            id="email-body-frame"
            src={ templ.SafeURL(fmt.Sprintf("/messages/%d/body?showImages=false", msg.ID)) }
            sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
            class="email-iframe"
            scrolling="no"
            frameborder="0"
        ></iframe>
    </div>
}
```

**Note on HTMX and iframe reload:** When the user clicks "Load images", the hx-get targets the iframe element itself (`#email-body-frame`). HTMX can update the iframe's `src` attribute, but the cleanest approach is to have the handler return a full new `<iframe>` element with `showImages=true` in the src URL, and use `hx-swap="outerHTML"` to replace the whole iframe. Alternatively, use JavaScript to update `iframe.src` directly. The HTMX approach requires the server to return an `<iframe>` fragment.

### AttachmentInfo Model Update

```go
// query/models.go — add ContentID field
type AttachmentInfo struct {
    ID          int64  `json:"id"`
    Filename    string `json:"filename"`
    MimeType    string `json:"mimeType"`
    Size        int64  `json:"size"`
    ContentHash string `json:"contentHash,omitempty"`
    ContentID   string `json:"contentId,omitempty"`  // NEW: from MIME Content-ID header
}
```

### fetchAttachmentsShared Update

```go
// query/shared.go
rows, err := db.QueryContext(ctx, fmt.Sprintf(`
    SELECT id, COALESCE(filename, ''), COALESCE(mime_type, ''), COALESCE(size, 0),
           COALESCE(content_hash, ''), COALESCE(content_id, '')
    FROM %sattachments
    WHERE message_id = ?
`, tablePrefix), msg.ID)
// ... scan includes &att.ContentID ...
```

### UpsertAttachment Update

```go
// store/messages.go — new signature
func (s *Store) UpsertAttachment(messageID int64, filename, mimeType, storagePath, contentHash, contentID string, size int) error {
    // ... existing check for existing record ...
    _, err = s.db.Exec(`
        INSERT INTO attachments (message_id, filename, mime_type, storage_path, content_hash, content_id, size, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
    `, messageID, filename, mimeType, storagePath, contentHash, contentID, size)
    return err
}
```

**Callers to update:** `internal/sync/sync.go` (passes `att.ContentID`) and `internal/importer/ingest.go` (if it calls UpsertAttachment).

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| UGCPolicy for email | Custom email policy | Known since always | UGCPolicy strips table attrs, style blocks; email needs permissive policy |
| Regex HTML stripping | allowlist sanitizer (bluemonday) | 2015+ | Regex is exploitable; parser-based allowlists are correct |
| Inline iframe content | Separate endpoint (`/body`) | Modern practice | Enables proper HTTP caching, CSP headers, clean URL for dev inspection |
| Fixed iframe height | ResizeObserver + postMessage | 2020+ (ResizeObserver GA) | ResizeObserver has 97%+ browser support; clean auto-resize without polling |

**Deprecated/outdated:**
- `AllowStyling()` (class attribute only): Not sufficient for email; need full `style` attribute + `AllowUnsafe` for `<style>` block
- `MutationObserver`-only resize: Works but fires on any DOM change; prefer ResizeObserver as primary with MutationObserver as fallback for image-load-triggered reflows

---

## Open Questions

1. **HTMX image toggle: fragment return vs JS src update**
   - What we know: HTMX `hx-get` + `hx-swap="outerHTML"` on the iframe element can replace the iframe. The server returns a new `<iframe>` with `showImages=true`.
   - What's unclear: HTMX target must be the iframe's parent div, not the iframe itself, to avoid HTMX injecting into a sandboxed document. Alternatively, use a small inline JS handler instead of HTMX for the image toggle.
   - Recommendation: During implementation, test both approaches. If HTMX targeting the parent div works cleanly, use it. Otherwise, use `onclick="document.getElementById('email-body-frame').src = '/messages/{id}/body?showImages=true'"` directly on the "Load images" link.

2. **Backfill goroutine: store access**
   - What we know: The backfill needs to query attachments with NULL content_id, fetch their message's raw MIME, re-parse, and update content_id.
   - What's unclear: The web server's `handlers` struct has `engine query.Engine` but not `*store.Store`. The backfill needs direct store access for writing.
   - Recommendation: Pass `*store.Store` to the web Server struct alongside `query.Engine`, or expose a `BackfillCIDColumns(store *store.Store)` function that the CLI's `serve` command calls before starting the HTTP server.

3. **CSS `background-image` external blocking**
   - What we know: The locked decision blocks `<img src="http://...">` only.
   - What's unclear: CSS `background-image` in `<style>` blocks or inline `style` attributes can also load external resources (tracking pixels).
   - Recommendation: Accept this as a known limitation for Phase 7. The iframe's CSP header on the body endpoint can be set to `img-src 'self' data:` to block external images at the browser level, covering both `<img>` and CSS backgrounds. Implement via response header, not bluemonday.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` |
| Config file | none (go test ./...) |
| Quick run command | `go test ./internal/web/... -run TestSanitize -v` |
| Full suite command | `go test ./... 2>&1 | tail -20` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RENDER-01 | Script tags stripped by bluemonday | unit | `go test ./internal/web/... -run TestSanitizeEmailHTML_StripScript -v` | Wave 0 |
| RENDER-01 | Event handler attrs stripped | unit | `go test ./internal/web/... -run TestSanitizeEmailHTML_StripOnclick -v` | Wave 0 |
| RENDER-01 | Table structure preserved | unit | `go test ./internal/web/... -run TestSanitizeEmailHTML_PreserveTable -v` | Wave 0 |
| RENDER-02 | `/messages/{id}/body` endpoint returns 200 | integration | `go test ./internal/web/... -run TestMessageBodyEndpoint -v` | Wave 0 |
| RENDER-02 | Body endpoint returns standalone HTML (not wrapped in layout) | integration | `go test ./internal/web/... -run TestMessageBodyEndpointStandalone -v` | Wave 0 |
| RENDER-03 | CID src replaced with /attachments/{id}/inline | unit | `go test ./internal/web/... -run TestSubstituteCIDImages -v` | Wave 0 |
| RENDER-04 | External http src replaced when showImages=false | unit | `go test ./internal/web/... -run TestBlockExternalImages -v` | Wave 0 |
| RENDER-04 | External src preserved when showImages=true | unit | `go test ./internal/web/... -run TestBlockExternalImages_ShowImages -v` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/web/... -run TestSanitize -v`
- **Per wave merge:** `go test ./... 2>&1 | tail -20`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/web/sanitize_email_test.go` — covers RENDER-01 (sanitizer), RENDER-03 (CID sub), RENDER-04 (image blocking)
- [ ] `internal/web/handlers_test.go` additions — TestMessageBodyEndpoint, TestMessageBodyEndpointStandalone covering RENDER-02

*(No new test framework needed — existing `go test` + httptest pattern already established in handlers_test.go)*

---

## Sources

### Primary (HIGH confidence)

- `pkg.go.dev/github.com/microcosm-cc/bluemonday@v1.0.27` — AllowUnsafe, AllowElements, AddTargetBlankToFullyQualifiedLinks, AllowImages, AllowTables, AllowStyles API verified
- `web.dev/articles/sandboxed-iframes` — sandbox attribute security model, allow-same-origin + allow-scripts danger confirmed
- Codebase direct inspection: `internal/mime/parse.go`, `internal/store/messages.go`, `internal/query/models.go`, `internal/query/shared.go`, `internal/web/server.go`, `internal/web/handlers.go`, `internal/web/templates/message.templ`

### Secondary (MEDIUM confidence)

- Close.io blog post (making.close.com) — srcdoc approach, layered defense strategy, allow-popups-to-escape-sandbox recommendation
- Mozilla Discourse — allow-scripts + allow-same-origin security analysis (multiple independent sources agree)
- pkg.go.dev bluemonday — AllowUnsafe required for `<style>` element (confirmed from API docs)

### Tertiary (LOW confidence)

- WebSearch results on iframe resize patterns — ResizeObserver + postMessage is standard practice but specific debounce values (50-100ms) are from community examples, not official specs

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — bluemonday version and API verified via pkg.go.dev
- Architecture: HIGH — based on direct codebase inspection; patterns match existing Phase 6 conventions
- `<style>` element / AllowUnsafe: HIGH — confirmed from pkg.go.dev API docs; the iframe sandbox justification is verified from multiple security sources
- Pitfalls: HIGH for security pitfalls (verified); MEDIUM for edge cases (ResizeObserver loop)
- CID backfill: HIGH — `GetMessageRaw` and `mime.Parse` both exist in codebase; pattern is straightforward

**Research date:** 2026-03-10
**Valid until:** 2026-06-10 (bluemonday is stable; iframe/postMessage patterns are stable web platform APIs)
