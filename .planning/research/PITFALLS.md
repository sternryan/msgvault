# Pitfalls Research

**Domain:** Templ + HTMX Web UI rebuild — replacing React SPA in Go email archive tool
**Researched:** 2026-03-10
**Confidence:** HIGH (templ/HTMX behavior, XSS patterns, cherry-pick mechanics), MEDIUM (email iframe sandboxing edge cases)

---

## Critical Pitfalls

### Pitfall 1: Raw Email HTML Passed to Templ as `templ.HTML` Bypasses All Sanitization

**What goes wrong:**
Templ auto-escapes all string values. To render email HTML bodies, developers reach for `templ.HTML()` to mark content as safe — but this completely disables escaping. If the email body has not been sanitized before this cast, any script, event handler, or CSS in the email renders directly into the page context. This is not theoretical: email HTML routinely contains `<script>`, `onclick=`, `style=` with CSS injection payloads, and `<img onerror=>` vectors.

**Why it happens:**
The path of least resistance. The dev sees broken HTML output, googles "templ render raw html", finds `templ.HTML()`, applies it, and moves on. The auto-escape safety net is gone and nothing warns you.

**How to avoid:**
Run bluemonday with a strict email-safe policy **before** casting to `templ.HTML()`. The chain must be: raw email bytes → charset decode → bluemonday sanitize → `templ.HTML()`. Never `templ.HTML()` first.

Use bluemonday's `UGCPolicy()` as a starting point but tighten it further for email: strip `<style>` entirely (bluemonday's own docs say style is fundamentally unsafe with no CSS sanitizer), allow `<a href>` only with `http/https` schemes, allow inline images via `<img src>` only with `data:` or `cid:` or your `/attachments/` prefix, and strip all event attributes.

bluemonday has a reference implementation at `cmd/sanitise_html_email/main.go` — read it before writing your own policy.

**Warning signs:**
- Template file contains `templ.HTML(message.Body)` without a sanitize call nearby
- `bluemonday` not in `go.mod` by Phase 2
- HTML body rendered directly in a template that also contains HTMX attributes (script injection could hijack HTMX requests)

**Phase to address:** Phase 1 (adopting PR #176) — establish the sanitize helper before any template touches email bodies. Do not defer to Phase 2.

---

### Pitfall 2: Email HTML Rendered Inline Breaks Application CSS and Layout

**What goes wrong:**
Even after sanitizing, injecting email HTML directly into the page DOM causes email CSS (inline styles, `<style>` blocks if you allowed them, font declarations) to leak into the application layout. Email HTML is written for email clients, not browsers — it uses `<table>` layouts, fixed widths, inline `font-size`, background colors, and `!important` overrides. These bleed into your Templ layout and destroy the Solarized theme.

**Why it happens:**
Developers think "I sanitized it, it's safe to inject." Sanitization prevents XSS; it does not prevent CSS collision. The DOM is flat — there is no style boundary between email content and page content without explicit isolation.

**How to avoid:**
Render email HTML bodies in a sandboxed `<iframe srcdoc="...">` rather than injecting directly into the page. The iframe creates a true style boundary. Use the srcdoc approach (not `src=` pointing to a separate endpoint) to avoid a separate attachment endpoint for email bodies.

Iframe sandbox attribute: `sandbox="allow-popups allow-popups-to-escape-sandbox"` — this blocks scripts and prevents same-origin access. Do NOT add `allow-scripts` or `allow-same-origin` together — that combination defeats the sandbox entirely.

Add a meta CSP inside the srcdoc HTML: `<meta http-equiv="Content-Security-Policy" content="script-src 'none';">` as defense in depth.

**Warning signs:**
- Message detail template uses `<div>@content.Body</div>` for email HTML
- Application layout breaks when viewing an HTML email
- Email CSS classes appear in browser dev tools outside the email content area

**Phase to address:** Phase 2 (message detail + thread view) — before any HTML email body is rendered in a browser.

---

### Pitfall 3: Templ CLI Version Mismatch Causes Compilation Failures

**What goes wrong:**
Templ has two version surfaces: the `templ` CLI (installed via `go install`) and the `github.com/a-h/templ` runtime in `go.mod`. When these versions diverge, `go build` fails with cryptic errors like `undefined: templ.JoinURLErrs` or type mismatches. This is especially likely when cherry-picking PR #176's code: the PR was written against a specific templ version that may differ from whatever is installed locally.

**Why it happens:**
`go install github.com/a-h/templ/cmd/templ@latest` installs the latest CLI. The cherry-picked code's `go.mod` pins a different runtime version. The generated `_templ.go` files use runtime APIs from the version used when they were generated — and those APIs may not exist in the locally installed version.

**How to avoid:**
Pin the templ CLI version in the Makefile explicitly:
```makefile
TEMPL_VERSION := v0.3.x  # match go.mod
.PHONY: install-tools
install-tools:
    go install github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)
```

Use `go run github.com/a-h/templ/cmd/templ@$(go list -m -f '{{.Version}}' github.com/a-h/templ)` to guarantee CLI matches go.mod. Commit the generated `_templ.go` files so `go build` works without the CLI installed. Add a `make generate` target that re-runs templ only when `.templ` files change.

**Warning signs:**
- `go build` fails after cherry-pick with "undefined: templ." errors
- `templ --version` output does not match version in `go.mod`
- PR #176 generated `_templ.go` files fail to compile locally

**Phase to address:** Phase 1 (cherry-pick adoption) — pin version before the first `go build` attempt.

---

### Pitfall 4: HTMX History / Back Button Breaks When Handlers Return Partials Unconditionally

**What goes wrong:**
HTMX partial updates work by detecting the `HX-Request` header. A common pattern returns a partial HTML fragment when `HX-Request: true` and a full page when the header is absent. The bug: when the user presses the browser back button, HTMX restores history from its cache. On cache miss, it re-fetches the URL — but the re-fetch sets `HX-Request: true`, so the handler returns a partial. The user sees a fragment rendered as a full page — broken layout, missing nav, no CSS.

This is a documented HTMX issue (GitHub #3037, #3165, #497) that has not been fully resolved.

**Why it happens:**
The intuitive handler pattern `if r.Header.Get("HX-Request") == "true" { renderPartial } else { renderFull }` seems correct but breaks on history restoration cache miss. The history restoration request looks identical to a live HTMX request.

**How to avoid:**
Use `hx-push-url="true"` only on navigation actions (drill-down, pagination changes) that update the full meaningful URL. For every URL that gets pushed, the server handler must be able to return a complete page at that URL regardless of headers.

Design: the `HX-Request` conditional should only determine which template wrapper to use (full layout vs. fragment), not whether to return usable content. The full page must always be servable at the canonical URL.

Alternatively, use `hx-replace-url` for partial updates that do not represent navigable states — search input debounce, sort toggles — so they never enter the browser history at all.

**Warning signs:**
- Pressing browser back button after drill-down or pagination shows unstyled HTML fragments
- The `HX-Request` header check is used to choose between returning data vs. no data (rather than choosing wrapper template)
- `hx-push-url` is on every HTMX-enhanced element including sort buttons and search inputs

**Phase to address:** Phase 1 (adopting PR #176) — audit existing history push patterns before adding new ones.

---

### Pitfall 5: Cherry-Picking PR #176 with Conflicting `internal/web/` Package

**What goes wrong:**
The fork already has `internal/web/` (React SPA server with `handlers.go`, `server.go`, `middleware.go`) and `internal/api/` (JSON API). PR #176 also introduces `internal/web/` with a completely different structure (Templ handlers, no JSON API). Cherry-picking the PR branch directly will produce conflicts in every file in that package. Worse: the fork's `internal/web/handlers.go` references `store.Store` directly (for thread/attachment handlers added in the fork), while PR #176 uses `query.Engine`. These are not just naming conflicts — they are structural conflicts that require understanding both sides before resolving.

Additionally, PR #176 may have been rebased multiple times. Cherry-picking individual commits rather than the branch tip means dependency chains break — a commit that adds a handler may depend on an earlier commit that added the route registration.

**Why it happens:**
The instinct to cherry-pick individual commits to "take only what we need" creates hidden dependency chains. File-level conflicts obscure which side's intent is correct.

**How to avoid:**
Use the directory-copy strategy from the design spec: `git checkout sarcasticbird/feature-templ-ui -- internal/web/` to copy the directory wholesale rather than cherry-picking individual commits. This avoids git merge conflict markers and lets you diff the two versions manually.

Order of operations: (1) delete the old `internal/web/` and `internal/api/` directories, commit the deletion, (2) copy PR #176's `internal/web/` in, (3) manually re-apply fork-specific additions (store.Store for thread handler, attachment inline rendering). This is more work upfront but avoids compound conflicts.

Keep a diff of what the fork added on top of upstream before starting — `git diff upstream/main...fork/main -- internal/` — so you know exactly what needs to be re-applied.

**Warning signs:**
- More than 5 files have merge conflict markers after cherry-pick
- `go build` fails with "package redeclared" or "undefined" errors on packages that existed before
- The old React `embed.go` / `embed_dev.go` pattern still exists alongside Templ templates

**Phase to address:** Phase 1 (cherry-pick adoption) — this is the primary risk of Phase 1.

---

### Pitfall 6: OOB (Out-of-Band) HTMX Swaps Silently Drop Elements

**What goes wrong:**
`hx-swap-oob` swaps require matching element IDs between the response HTML and the current DOM. If the ID in the response doesn't match any element on the page, HTMX silently discards the OOB element with no error. This is especially problematic for the deletion count badge in the nav — a common pattern is to return the updated badge HTML as an OOB swap alongside a deletion confirmation response. If the nav badge ID in the template doesn't exactly match what the handler returns, the count never updates.

A related issue (GitHub #2790): when using OOB with `beforeend` swap strategy, the outer element with `hx-swap-oob` is stripped, leaving only its children.

**Why it happens:**
OOB swaps fail silently. There is no browser console error, no network error — the swap just doesn't happen. Developers assume the server code is wrong and debug the wrong layer.

**How to avoid:**
Define IDs for all OOB swap targets in a single constants file (or `helpers.go`) and reference them in both templates and handlers. Never hardcode OOB target IDs as string literals in two places.

When adding OOB swaps: test by checking element existence first. A quick test: does the page currently contain `<element id="that-id">`? If not, the OOB swap will be silently dropped.

For deletion badge: use `innerHTML` swap strategy (the default), not `beforeend` — this avoids the outer element stripping bug.

**Warning signs:**
- Deletion count in nav does not update after staging despite 200 response
- Inspecting network tab shows correct HTML in response but DOM doesn't update
- OOB IDs defined inline as string literals in multiple template files

**Phase to address:** Phase 2 (deletion management with HTMX partial updates).

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Skip bluemonday, use `templ.HTML()` directly on email body | Faster to ship message detail | Stored XSS — email bodies can contain arbitrary scripts | Never |
| Allow `<style>` elements in bluemonday policy | Preserves email formatting | CSS injection, layout destruction — bluemonday has no CSS sanitizer | Never |
| No iframe sandbox for email HTML | Simpler template | Email CSS destroys app layout; scripts execute in page context | Never |
| Commit `_templ.go` without pinning templ CLI version | Works on dev machine now | Other contributors get build failures with different CLI versions | Only if you are the sole developer with a locked environment |
| Return same partial response regardless of `HX-Request` | Simpler handler logic | Back button and direct URL access return unstyled fragments | Never for navigable URLs; acceptable for non-history-pushing partials |
| Copy PR #176 generated `_templ.go` files without re-running templ locally | Faster adoption | Files may be stale if PR was updated; version mismatch on next template edit | Only during initial import if immediately followed by version pin verification |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Templ + `go:embed` | Embedding `templates/` directory instead of generated `_templ.go` files — templates must be compiled, not embedded as raw files | Embed `static/` (CSS, JS) only; `_templ.go` files compile directly into the binary via standard `go build` |
| Templ + `go generate` | Running `go generate ./...` from root when only `internal/web/templates/` has `.templ` files — works but slow and confusing | Add `//go:generate templ generate` comment in `internal/web/templates/` and run from that package |
| HTMX + chi router | Forgetting HTMX sends `HX-Request: true` on all requests including GET — chi middleware that checks auth may not recognize HTMX requests as browser requests | Test auth middleware with HTMX-originated requests; ensure session cookie flows correctly |
| bluemonday + charset-decoded email | Running bluemonday on raw bytes before charset conversion — produces incorrect sanitization on non-UTF-8 content | Charset decode first (existing `mime/parse.go` logic), then sanitize the decoded UTF-8 string |
| Inline attachment + CSP headers | Setting `Content-Security-Policy` on the attachment endpoint that prevents the parent page from embedding it in an iframe | The CSP on `/attachments/{id}/inline` should allow `frame-ancestors 'self'`, not deny it |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Rendering email body HTML inline with bluemonday on every page load | Message list page slow when showing snippets; thread view slow for long threads | Sanitize once on ingest (store sanitized HTML in DB) or cache result; do not sanitize on every render | With 20+ year archives, thread pages with 100+ messages will be noticeably slow if sanitizing each body per request |
| Query.Engine (DuckDB) called from HTMX partial handlers without connection pooling | Partial updates slow; concurrent HTMX requests (search debounce fires multiple times) cause DuckDB contention | DuckDB has limited concurrent write support; verify the Engine is initialized once and shared, not re-opened per request | Visible with debounced search generating 3-5 concurrent DuckDB queries |
| Templ rendering to `http.ResponseWriter` without buffering | Partial template render failure writes incomplete HTML to client before error is detected | Render to `bytes.Buffer` first, then write on success; or use Templ's built-in error handling | Every request; partial HTML causes HTMX to swap broken fragments |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Serving email attachment inline without content-type validation | MIME sniffing — browser may execute a file as HTML even if served with wrong content-type | Always set `X-Content-Type-Options: nosniff` and validate content-type against actual file magic bytes; never trust the stored content-type alone |
| `<iframe sandbox="allow-scripts allow-same-origin">` for email HTML | Defeats the iframe sandbox entirely — document can remove sandbox attribute programmatically | Never combine `allow-scripts` + `allow-same-origin` in the same sandbox; use `allow-popups allow-popups-to-escape-sandbox` only |
| Reflecting user-supplied search query back into HTMX response without escaping | Stored/reflected XSS via search — attacker could send a crafted URL | Templ auto-escapes string values; do not cast search query to `templ.HTML()` or `template.HTML` anywhere |
| No CSRF protection on deletion POST handlers | Attacker page tricks user's browser into staging deletions | Add CSRF token to all POST forms; chi middleware (`github.com/justinas/nosurf` or `gorilla/csrf`) integrates cleanly |
| Attachment download endpoint without hash validation | Attachment ID enumeration exposes other users' attachments (irrelevant in single-user tool, but worth noting for future multi-account expansion) | Validate SHA-256 hash param against stored content hash on every inline/download request — already noted in design spec |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| HTMX replaces full page content on search while user is typing | Jarring mid-type refreshes; cursor position lost | Use `hx-trigger="input delay:300ms"` for debounced search, `hx-target` pointing to results container only, not full page |
| Email HTML iframe too short — email content clipped | User thinks message body is empty or truncated | Use `scrolling="no"` iframe with a `ResizeObserver` or `postMessage`-based height detection; or use a fixed generous min-height with `overflow:auto` |
| Deletion confirmation has no undo path in HTMX UI | Accidental deletions are permanent (permanent delete mode) | Soft-delete staging already exists in the architecture — ensure the HTMX UI makes the two-step flow (stage → confirm) obvious and reversible at stage step |
| Keyboard shortcut `t` for thread nav conflicts with TUI's `t` for Time view | Users who switch between TUI and Web UI are confused | Not a direct conflict (different interfaces), but document keyboard shortcuts consistently in both `?` help overlays |

---

## "Looks Done But Isn't" Checklist

- [ ] **HTML email rendering:** HTML body displays, but verify sanitization actually stripped scripts — test with `<img src=x onerror=alert(1)>` in a test fixture message
- [ ] **iframe sandbox:** Email body iframe renders, but verify `allow-scripts` is NOT in sandbox attribute and sandbox attribute is present at all (easy to drop from template)
- [ ] **Templ version pin:** `go build` works locally, but verify a fresh clone with `go install templ@latest` also builds without errors
- [ ] **Back button navigation:** Drill-down and pagination work forward, but verify back button returns to correct previous state with full layout, not a partial fragment
- [ ] **CSRF protection:** Deletion staging POST responds correctly, but verify a cross-origin POST from a different origin is rejected
- [ ] **OOB swap targets:** Deletion badge updates after staging, but verify it updates after the second and third staging action (not just the first)
- [ ] **Attachment CSP:** Inline image renders in email iframe, but verify clicking an attachment from a different message does not serve a cached incorrect file
- [ ] **go:embed static files:** CSS loads in `go build` binary, but verify it also loads after `go build` on a machine that has never run `make web-build` (no leftover `dist/` directory from old React build)
- [ ] **React removal:** `go build` completes, but verify `web/` directory, `package.json`, `node_modules/`, and all `npm` Makefile targets are actually deleted — not just inert

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| XSS via unsanitized email HTML (deployed) | HIGH | Add bluemonday immediately; audit all templates for `templ.HTML()` usage; no data loss but credibility damage |
| Templ CLI version mismatch blocking build | LOW | `go install github.com/a-h/templ/cmd/templ@<version-in-go.mod>`; re-run `templ generate`; fix takes minutes |
| Cherry-pick created unresolvable conflicts | MEDIUM | Abort cherry-pick strategy; use directory-copy approach instead; lose ~1 day |
| Back button shows broken partial | MEDIUM | Audit every `hx-push-url` usage; add full-page fallback in handlers; requires touching all HTMX-enhanced handlers |
| HTMX OOB swap silently failing | LOW | Add ID constants file; fix template IDs; test with browser network inspector |
| Email CSS destroying app layout | LOW-MEDIUM | Wrap in iframe srcdoc; existing bluemonday policy may already strip style blocks |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Raw email HTML bypassing sanitization via `templ.HTML()` | Phase 1 (adopt PR #176) — establish sanitize helper before any template touches email body | `grep -r 'templ.HTML' internal/web/` must show only pre-sanitized content; test with XSS fixture |
| Email HTML breaking app CSS/layout (no iframe isolation) | Phase 2 (message detail + thread view) — before first browser render of email body | Open an HTML email; application nav and theme must be intact |
| Templ CLI version mismatch | Phase 1 — pin version in Makefile before first `go build` | Fresh clone + `make install-tools` + `go build` must succeed |
| HTMX back button broken by partial responses | Phase 1 — audit PR #176 patterns; enforce in Phase 2 additions | Manual test: navigate drill-down 3 levels, press back 3 times, verify full layout each time |
| Cherry-pick conflicts with existing `internal/web/` | Phase 1 — use directory-copy strategy, not commit cherry-pick | `go build` with no conflict markers; no React imports remaining |
| OOB swap silently dropping elements | Phase 2 (deletion HTMX flows) | Stage a deletion; verify badge count updates; stage again; verify count increments |
| CSRF on deletion endpoints | Phase 2 (deletion management) | Send POST to `/deletions/stage` from curl without CSRF token; must receive 403 |
| MIME sniffing on attachment inline endpoint | Phase 2 (inline attachments) | Serve a `.html` file via attachment endpoint; browser must not execute it as HTML |
| Templ rendering partial HTML on error | Phase 1 (adopt server structure) | Trigger a handler error; verify 500 page renders complete HTML, not truncated fragment |

---

## Sources

- [bluemonday HTML sanitizer — Go Packages](https://pkg.go.dev/github.com/microcosm-cc/bluemonday) — HIGH confidence (official)
- [bluemonday email sanitization example](https://github.com/microcosm-cc/bluemonday/blob/main/cmd/sanitise_html_email/main.go) — HIGH confidence (official)
- [Rendering untrusted HTML email safely — Close Engineering](https://making.close.com/posts/rendering-untrusted-html-email-safely/) — MEDIUM confidence (production post-mortem)
- [templ version mismatch discussion #592](https://github.com/a-h/templ/discussions/592) — HIGH confidence (official repo)
- [templ version mismatch discussion #460](https://github.com/a-h/templ/discussions/460) — HIGH confidence (official repo)
- [templ version warning issue #249](https://github.com/a-h/templ/issues/249) — HIGH confidence (official repo)
- [HTMX hx-push-url back button issue #854](https://github.com/bigskysoftware/htmx/issues/854) — HIGH confidence (official repo)
- [HTMX history restore behavior change issue #3037](https://github.com/bigskysoftware/htmx/issues/3037) — HIGH confidence (official repo)
- [HTMX history not working on partial content issue #3165](https://github.com/bigskysoftware/htmx/issues/3165) — HIGH confidence (official repo)
- [HTMX hx-push-url full reload on cache miss issue #497](https://github.com/bigskysoftware/htmx/issues/497) — HIGH confidence (official repo)
- [HTMX hx-swap-oob strips outer element issue #2790](https://github.com/bigskysoftware/htmx/issues/2790) — HIGH confidence (official repo)
- [GoTTH stack learnings — Emily T. Burak](https://emilytburak.net/posts/2025-06-09-htmx-golang-learnings/) — MEDIUM confidence (practitioner post-mortem)
- [templ context propagation docs](https://templ.guide/syntax-and-usage/context/) — HIGH confidence (official)
- [XSS in Go — Semgrep cheat sheet](https://semgrep.dev/docs/cheat-sheets/go-xss) — MEDIUM confidence (security reference)
- [iframe sandbox + allow-scripts + allow-same-origin defeat — MDN](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/iframe) — HIGH confidence (official)
- [bluemonday CVE-2021-29272 Cyrillic bypass](https://vulert.com/vuln-db/go-github-com-microcosm-cc-bluemonday-92966) — MEDIUM confidence (CVE record; fixed in 1.0.5)

---

*Pitfalls research for: Templ + HTMX Web UI rebuild in Go email archive tool*
*Researched: 2026-03-10*
