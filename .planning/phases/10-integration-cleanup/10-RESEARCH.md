# Phase 10: Integration Test & DOM Cleanup - Research

**Researched:** 2026-03-11
**Domain:** Go test assertions, HTMX fragment rendering, HTML duplicate ID repair
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| RENDER-04 | External images blocked by default with opt-in toggle | INT-01 fix ensures the test correctly validates the unified toolbar that implements RENDER-04 for the dual-format case |
| POLISH-01 | User can toggle between plain text and HTML rendering per message | INT-01 fix aligns the test with the Phase 9 unified toolbar that implements POLISH-01 |
</phase_requirements>

---

## Summary

Phase 10 closes two integration gaps identified in the v1.1 milestone audit:

**INT-01 (HIGH severity):** `TestMessageBodyWrapperEndpoint` in `internal/web/handlers_test.go` was written for the Phase 7 body-wrapper response shape (separate `email-images-banner` div + `hx-target="#email-body-wrapper"` attribute). Phase 9 unified the toolbar: when a message has both text and HTML bodies (`hasBothFormats=true`), the banner is absorbed into the `email-toolbar` div and `hx-target` becomes `closest .email-render-wrapper` on all action links. The mock message in `setupTestServer` returns both `BodyText` and `BodyHTML`, triggering the `hasBothFormats` code path. The test therefore fails on two assertions.

**INT-02 (LOW severity):** Thread view lazy-loaded messages (non-latest, non-highlighted) receive their body via `hx-swap="innerHTML"` into a `div.thread-message-body`. When that swap lands, the `messageBodyWrapper` handler always emits `id="email-body-wrapper"` regardless of context. A thread page with N collapsed messages results in N elements sharing the same ID, which violates the HTML spec. The thread template already uses `id="email-body-wrapper-{msgID}"` for the eagerly-rendered message card (line 114 of thread.templ), but the lazy-load path (the body-wrapper handler) does not accept or use a message-scoped ID.

**Primary recommendation:** Fix `TestMessageBodyWrapperEndpoint` by updating its two stale assertions; fix the duplicate ID by making `messageBodyWrapper` emit `id="email-body-wrapper-{msgID}"` when called from a thread context OR by always emitting the message-ID-scoped ID for all body-wrapper responses.

---

## Standard Stack

### Core (no new dependencies)

| Component | Version | Purpose | Notes |
|-----------|---------|---------|-------|
| Go testing stdlib | go1.21+ | `TestMessageBodyWrapperEndpoint` and suite | Already present |
| `net/http/httptest` | stdlib | `httptest.Server` used by `setupTestServer` | Already present |
| `strings.Contains` | stdlib | HTML fragment assertions | Already present |
| templ | v0.3.1001 | Generating `_templ.go` from `.templ` sources | Pinned in Makefile; no change needed |

**No new libraries required for this phase.**

---

## Architecture Patterns

### Pattern 1: Test-First Delta Analysis (what changed vs what the test expects)

The failing test `TestMessageBodyWrapperEndpoint` calls `GET /messages/1/body-wrapper` with no query params.

**Mock message returned by `mockEngine.GetMessage(ctx, 1)`:**
```go
&query.MessageDetail{
    ID:       1,
    BodyText: "This is the test message body.",  // non-empty
    BodyHTML: `<p>Hello <b>world</b></p>`,        // non-empty
    Attachments: []query.AttachmentInfo{
        {ID: 10, ContentID: "img1@example", ContentHash: "abc123"},
    },
    ...
}
```

Both `BodyText` and `BodyHTML` are non-empty, so `hasBothFormats = true` in `messageBodyWrapper`.

**Current handler path (hasBothFormats=true, format="", showImages=false):**
```
// handlers_messages.go lines 143-212
// hasBothFormats=true → toolbarHTML is set to the unified toolbar
// showImages=false → falls into the "else" branch at line 189
// but hasBothFormats=true → bannerHTML is set to "" and the unified toolbar path runs (line 200-212)
// Output:
<div id="email-body-wrapper" class="email-render-wrapper">
  <div class="email-toolbar">
    <a ... hx-get="/messages/1/body-wrapper?format=text"
           hx-target="closest .email-render-wrapper"
           hx-swap="outerHTML"
           hx-replace-url="/messages/1?format=text">Text</a>
    <span class="email-toolbar-btn active">HTML</span>
    <span class="email-toolbar-sep">·</span>
    <span>External images blocked.</span>
    <a href="#"
       hx-get="/messages/1/body-wrapper?showImages=true"
       hx-target="closest .email-render-wrapper"
       hx-swap="outerHTML">Load images</a>
  </div>
  <iframe id="email-body-frame" src="/messages/1/body" ...></iframe>
</div>
```

**What the test asserts (and fails on):**
1. `strings.Contains(bodyStr, "email-images-banner")` — FAILS. The banner class does not appear when `hasBothFormats=true`; the toolbar absorbs it.
2. `strings.Contains(bodyStr, `hx-target="#email-body-wrapper"`)` — FAILS. All hx-target values use `closest .email-render-wrapper`, not the ID selector.

**What the test asserts (and passes):**
1. `strings.Contains(bodyStr, `id="email-body-wrapper"`)` — passes (the wrapper div still has this ID)
2. `strings.Contains(bodyStr, "hx-get")` — passes (toolbar links have hx-get)
3. `strings.Contains(bodyStr, `hx-swap="outerHTML"`)` — passes

### Pattern 2: Correct Test Assertions for Current Behavior

The test's intent is to verify the body-wrapper endpoint returns an HTMX-swappable fragment with:
- The wrapper div present
- Some form of image-loading control present
- HTMX swap attributes

The correct updated assertions for `hasBothFormats=true` (the actual test case):

```go
// KEEP: wrapper div ID is still present
if !strings.Contains(bodyStr, `id="email-body-wrapper"`) {
    t.Errorf("body does not contain email-body-wrapper div")
}

// REPLACE: email-images-banner → email-toolbar (unified control when hasBothFormats=true)
if !strings.Contains(bodyStr, "email-toolbar") {
    t.Errorf("body does not contain email-toolbar (Phase 9 unified toolbar)")
}

// REPLACE: hx-target="#email-body-wrapper" → hx-target="closest .email-render-wrapper"
if !strings.Contains(bodyStr, `hx-target="closest .email-render-wrapper"`) {
    t.Errorf("body does not contain hx-target='closest .email-render-wrapper'")
}

// KEEP: hx-get still present
if !strings.Contains(bodyStr, "hx-get") {
    t.Errorf("body does not contain hx-get attribute")
}

// KEEP: hx-swap="outerHTML" still present
if !strings.Contains(bodyStr, `hx-swap="outerHTML"`) {
    t.Errorf("body does not contain hx-swap attribute")
}
```

### Pattern 3: Duplicate ID in Thread Lazy-Load

**Thread page structure for a 3-message thread (IDs 10, 11, 12; 12 is latest):**

```
thread-container
  msg-10: <details> (collapsed)
    <summary>...</summary>
    <div class="thread-message-body"
         hx-get="/messages/10/body-wrapper"
         hx-trigger="toggle once"
         hx-swap="innerHTML">
      Loading...
    </div>
  msg-11: <details> (collapsed, same pattern)
  msg-12: <details open> (latest, eager)
    <div id="email-body-wrapper-12" class="email-render-wrapper">  ← unique ID
      <div class="email-toolbar">...</div>
      <iframe ...></iframe>
    </div>
```

When user expands msg-10, HTMX GETs `/messages/10/body-wrapper` and swaps `innerHTML` of `div.thread-message-body`. The handler returns:

```html
<div id="email-body-wrapper" class="email-render-wrapper">
  <div class="email-toolbar">...</div>
  <iframe ...></iframe>
</div>
```

After swap, the DOM has `id="email-body-wrapper-12"` (eager) and `id="email-body-wrapper"` (lazy-loaded for msg-10). If msg-11 is also expanded, a second `id="email-body-wrapper"` appears. HTML spec violation: IDs must be unique.

**The fix options:**

**Option A — Add `?wrapperID=email-body-wrapper-{msgID}` param to hx-get in thread template:**
- Thread template passes `?wrapperID=email-body-wrapper-10` in the lazy hx-get URL
- Handler reads `wrapperID` query param, uses it as the `id` attribute if provided, falls back to `email-body-wrapper` when absent
- Backward-compatible: message detail page lazy-load still works unchanged
- Minimal change surface

**Option B — Always emit ID-scoped wrapper: `id="email-body-wrapper-{msgID}"`:**
- Handler always emits `id="email-body-wrapper-{msgID}"`
- Requires updating `TestMessageBodyWrapperEndpoint` and `TestMessageBodyWrapperShowImages` (both check `id="email-body-wrapper"`)
- Also requires updating `message.templ`: the `hx-target="#email-body-wrapper"` references in the eager-rendered message detail page must change
- Larger change, touches both the handler and the message detail template

**Option C — Thread template uses `hx-swap="outerHTML"` on an already-ID'd container:**
- The `div.thread-message-body` gets an id like `id="lazy-10"`, then the hx-get response can swap it outerHTML with the correctly IDed wrapper
- Complex, changes the thread template's swap strategy

**Recommendation: Option A** is the minimum-footprint fix. The thread template already generates the correct eager ID (`email-body-wrapper-{msgID}`), so just propagate the desired ID to the lazy-load handler via a query param.

### Pattern 4: thread.templ Lazy-Load Swap Mode

The collapsed message body container in thread.templ (line 159-166):

```templ
<div
    class="thread-message-body"
    hx-get={ fmt.Sprintf("/messages/%d/body-wrapper", msg.ID) }
    hx-trigger="toggle once"
    hx-swap="innerHTML"
    hx-indicator="#page-indicator"
>
    <p class="loading-placeholder">Loading message...</p>
</div>
```

`hx-swap="innerHTML"` means HTMX replaces the inner content of `.thread-message-body` with the response. The response is a `<div id="email-body-wrapper" ...>` — this nested div gets the duplicate ID.

With Option A, the hx-get URL becomes:
```
/messages/{msgID}/body-wrapper?wrapperID=email-body-wrapper-{msgID}
```

The handler uses that ID when emitting the wrapper div. The `hx-target` inside that wrapper uses `closest .email-render-wrapper` (already correct for the thread context), so internal navigation continues to work.

### Anti-Patterns to Avoid

- **Changing `hx-swap` from `outerHTML` to `innerHTML` in non-lazy contexts:** The message detail page body-wrapper uses `outerHTML` to swap itself. Don't change that — the fix is scoped to the lazy-load path.
- **Touching `message.templ`:** The eager-rendered message detail template already has correct behavior. Do not modify it to fix INT-02; the duplicate ID problem only occurs in the thread lazy-load path.
- **Rebuilding the templ generated files manually:** Run `make templ-generate` or `templ generate ./...` only if `.templ` files change. For the handler-only fix (Option A), no templ regeneration is needed.
- **Modifying `hasBothFormats` logic:** The Phase 9 implementation is correct. Only the test assertions need updating, not the handler logic.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead |
|---------|-------------|-------------|
| HTML assertion in tests | Custom HTML parser | `strings.Contains` (already the project pattern) |
| Unique ID generation | UUID library | `fmt.Sprintf("email-body-wrapper-%d", msgID)` (already done for eager cards) |

---

## Common Pitfalls

### Pitfall 1: Fixing Only One Test Assertion in TestMessageBodyWrapperEndpoint

**What goes wrong:** The test has two failing assertions. Fixing only `email-images-banner → email-toolbar` but leaving `hx-target="#email-body-wrapper"` causes the test to still fail.

**How to avoid:** Run `go test ./internal/web/... -run TestMessageBodyWrapperEndpoint -v` after each change. Both assertions must pass.

### Pitfall 2: TestMessageBodyWrapperShowImages False-Positive Risk

`TestMessageBodyWrapperShowImages` passes today because the `showImages=true` path with `hasBothFormats=true` renders a toolbar without a separate banner (already correct). Verify this test still passes after updating `TestMessageBodyWrapperEndpoint`.

### Pitfall 3: ID Uniqueness in hx-swap="innerHTML" vs outerHTML Context

The lazy-load uses `hx-swap="innerHTML"`, meaning the wrapper div with `id="email-body-wrapper"` lands as a child of `.thread-message-body`. If Option A is used, the query param must be read and used on ALL output paths in `messageBodyWrapper` (text format, html format with/without showImages, fallback). There are 6 `fmt.Fprintf` calls that emit the wrapper ID — all must use the parameterized ID.

**Warning signs:** A test that only checks one format path may pass while another leaves a duplicate ID.

### Pitfall 4: Regenerating templ files unnecessarily

`_templ.go` files are committed. Running `templ generate` when only `.go` handler files change is unnecessary and may produce a diff if the templ binary version differs. Only regenerate if `.templ` files are modified.

---

## Code Examples

### Current Failing Assertions (handlers_test.go lines 469-483)

```go
// Source: internal/web/handlers_test.go:452-484
if !strings.Contains(bodyStr, `id="email-body-wrapper"`) {
    t.Errorf("...does not contain email-body-wrapper div")
}
if !strings.Contains(bodyStr, "email-images-banner") {       // STALE: Phase 7 only
    t.Errorf("...does not contain email-images-banner")
}
if !strings.Contains(bodyStr, "hx-get") {
    t.Errorf("...does not contain hx-get attribute")
}
if !strings.Contains(bodyStr, `hx-target="#email-body-wrapper"`) { // STALE: Phase 7 only
    t.Errorf("...does not contain hx-target attribute")
}
if !strings.Contains(bodyStr, `hx-swap="outerHTML"`) {
    t.Errorf("...does not contain hx-swap attribute")
}
```

### Correct Assertions for Phase 9 Behavior

```go
// Source: handlers_messages.go messageBodyWrapper() — hasBothFormats=true path
// The handler emits email-toolbar (not email-images-banner) when hasBothFormats=true
// and hx-target="closest .email-render-wrapper" (not "#email-body-wrapper")

if !strings.Contains(bodyStr, `id="email-body-wrapper"`) {
    t.Errorf("...does not contain email-body-wrapper div")
}
if !strings.Contains(bodyStr, "email-toolbar") {
    t.Errorf("...does not contain email-toolbar (Phase 9 unified toolbar)")
}
if !strings.Contains(bodyStr, "hx-get") {
    t.Errorf("...does not contain hx-get attribute")
}
if !strings.Contains(bodyStr, `hx-target="closest .email-render-wrapper"`) {
    t.Errorf("...does not contain hx-target='closest .email-render-wrapper'")
}
if !strings.Contains(bodyStr, `hx-swap="outerHTML"`) {
    t.Errorf("...does not contain hx-swap attribute")
}
```

### Option A: wrapperID query param in messageBodyWrapper

```go
// In handlers_messages.go — read optional wrapperID param
wrapperID := r.URL.Query().Get("wrapperID")
if wrapperID == "" {
    wrapperID = "email-body-wrapper"
}
// Then replace all hardcoded `id="email-body-wrapper"` with fmt.Sprintf(`id="%s"`, wrapperID)
```

### Option A: Updated hx-get in thread.templ for lazy-load

```templ
// In ThreadMessageCard, the collapsed branch (else block):
<div
    class="thread-message-body"
    hx-get={ fmt.Sprintf("/messages/%d/body-wrapper?wrapperID=email-body-wrapper-%d", msg.ID, msg.ID) }
    hx-trigger="toggle once"
    hx-swap="innerHTML"
    hx-indicator="#page-indicator"
>
    <p class="loading-placeholder">Loading message...</p>
</div>
```

---

## State of the Art

| What Was True in Phase 7 | What Is True Now (Phase 9) | Impact on Phase 10 |
|--------------------------|----------------------------|--------------------|
| body-wrapper always emits `email-images-banner` div when BodyHTML is present | body-wrapper emits `email-toolbar` (unified) when hasBothFormats=true; `email-images-banner` only when BodyHTML only (no BodyText) | Test must assert `email-toolbar` not `email-images-banner` for dual-format messages |
| body-wrapper emits `hx-target="#email-body-wrapper"` | body-wrapper emits `hx-target="closest .email-render-wrapper"` | Test must assert `closest .email-render-wrapper` |
| Thread eager card rendered `id="email-body-wrapper-{msgID}"` (Phase 8) | Same (unchanged) | Lazy-load must match this pattern |

---

## Open Questions

1. **Should TestMessageBodyWrapperShowImages be updated too?**
   - What we know: It currently passes. The `showImages=true` path with `hasBothFormats=true` already emits a toolbar (not a banner).
   - What's unclear: The assertion `if strings.Contains(bodyStr, "email-images-banner")` passes because no banner is rendered in this path — but it doesn't assert the toolbar IS present.
   - Recommendation: Leave it passing as-is. Adding a positive assertion for `email-toolbar` would strengthen it, but is optional for this phase since INT-01 only requires `TestMessageBodyWrapperEndpoint` to pass.

2. **Should the duplicate-ID fix also add a test?**
   - What we know: No test currently validates ID uniqueness in thread lazy-load responses.
   - What's unclear: The phase success criterion only requires thread view DOM to use unique IDs; no test is explicitly required for this.
   - Recommendation: Add a test `TestBodyWrapperWithWrapperIDParam` that GETs `/messages/1/body-wrapper?wrapperID=email-body-wrapper-99` and asserts the response contains `id="email-body-wrapper-99"` instead of `id="email-body-wrapper"`. This directly validates INT-02 is closed.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` package |
| Config file | none (standard `go test`) |
| Quick run command | `go test ./internal/web/... -run TestMessageBodyWrapper -v` |
| Full suite command | `go test ./internal/web/...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RENDER-04 | External image toggle correctly reflected in test assertions | unit | `go test ./internal/web/... -run TestMessageBodyWrapperEndpoint -v` | Yes (fix needed) |
| POLISH-01 | Text/HTML toggle toolbar reflected in test assertions | unit | `go test ./internal/web/... -run TestMessageBodyWrapperEndpoint -v` | Yes (fix needed) |
| INT-02 | body-wrapper with ?wrapperID param emits that ID | unit | `go test ./internal/web/... -run TestBodyWrapperWithWrapperIDParam -v` | No — Wave 0 gap |

### Sampling Rate

- **Per task commit:** `go test ./internal/web/... -run TestMessageBodyWrapper`
- **Per wave merge:** `go test ./internal/web/...`
- **Phase gate:** `go test ./internal/web/...` green before close

### Wave 0 Gaps

- [ ] `TestBodyWrapperWithWrapperIDParam` in `internal/web/handlers_test.go` — covers INT-02 (duplicate ID fix verification). New test to add; does not yet exist.

---

## Sources

### Primary (HIGH confidence)

- Direct code read: `internal/web/handlers_test.go` lines 452-513 — exact failing assertions
- Direct code read: `internal/web/handlers_messages.go` lines 86-242 — current `messageBodyWrapper` handler with `hasBothFormats` logic
- Direct code read: `internal/web/templates/thread.templ` lines 148-169 — lazy-load `hx-get` and `hx-swap="innerHTML"` pattern
- Direct code read: `internal/web/templates/thread.templ` line 114 — eager card already uses `id="email-body-wrapper-{msgID}"`
- Live test run: `go test ./internal/web/... -v` — confirmed exactly 1 failing test (`TestMessageBodyWrapperEndpoint`), 2 failing assertions

### Secondary (HIGH confidence)

- `.planning/v1.1-MILESTONE-AUDIT.md` — INT-01 and INT-02 root cause analysis matches code inspection

---

## Metadata

**Confidence breakdown:**
- Exact failure: HIGH — confirmed by running `go test` and reading handler + test code
- Fix approach (test assertions): HIGH — directly derived from reading current handler output
- Fix approach (duplicate ID, Option A): HIGH — derived from thread template hx-get pattern and handler param reading
- Scope: HIGH — exactly 1 failing test, 2 failing assertions, 1 cosmetic DOM issue

**Research date:** 2026-03-11
**Valid until:** Stable; Go and templ versions pinned; no moving targets
