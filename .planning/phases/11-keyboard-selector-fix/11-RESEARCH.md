# Phase 11: Keyboard Selector Fix & Cleanup - Research

**Researched:** 2026-03-11
**Domain:** JavaScript DOM selectors, Go module dependency classification, SUMMARY frontmatter
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| PARITY-07 | User can navigate the web UI with Vim-style keyboard shortcuts | Fix two selector mismatches in keys.js: `[data-row]` → `[data-href]` for j/k/Enter; `.sort-header` → `.sortable-header` + add `data-sort-field` + `active` class for s/r |
| POLISH-03 | Loading indicators display during HTMX partial page updates (doc fix only) | Add `POLISH-03` to `requirements_completed` array in 09-02-SUMMARY.md frontmatter |
</phase_requirements>

---

## Summary

Phase 11 is a precision surgical fix: three changes to close five audit gaps. The work is almost entirely in `keys.js` and one YAML frontmatter file — no handler changes, no schema changes, no new dependencies.

**The root problem for PARITY-07:** `keys.js` was written with selectors from the React SPA era and never updated to match the attributes the Templ templates actually emit. Two mismatches exist: (1) `moveRow()` queries `[data-row]` but every navigable row in the Templ templates emits `data-href`; (2) `cycleSortField()` and `reverseSortDir()` query `.sort-header[data-sort-field]` and `.sort-header.active` but `SortHeader` in `components.templ` emits class `sortable-header` with no `data-sort-field` attribute and no `active` class on the `<th>` element.

**The fix strategy:** Update `keys.js` selectors to match what the templates emit (not the reverse) and add the missing `data-sort-field` + `active` class to the `SortHeader` Templ component. Also run `go mod tidy` to promote `bluemonday` from `// indirect` to a direct dependency. And add `POLISH-03` to the `09-02-SUMMARY.md` frontmatter.

**Primary recommendation:** Fix selectors in `keys.js` + add `data-sort-field`/`active` to `SortHeader` + `go mod tidy` + update `09-02-SUMMARY.md`. One plan, minimal scope.

---

## Exact Bugs Found (HIGH confidence — verified by reading source)

### Bug 1: INT-03 — moveRow() selector mismatch (HIGH severity)

**Location:** `internal/web/static/keys.js:165`

**Broken code:**
```javascript
var rows = document.querySelectorAll('[data-row]');
```

**What templates emit:** Every navigable `<tr>` in `messages.templ`, `aggregate.templ` (top-level rows), `search.templ`, and `dashboard.templ` (Top Senders, Top Domains) emits `data-href`. Example from `messages.templ:33`:
```html
<tr class="clickable-row message-row" data-href="/messages/1" hx-get="/messages/1" ...>
```

**What uses `data-row`:** Only `deletions.templ:35` — and those rows should NOT be j/k navigable (they have no `href`, only a Cancel button).

**Fix in keys.js:**
```javascript
// Line 165: change
var rows = document.querySelectorAll('[data-row]');
// to
var rows = document.querySelectorAll('[data-href]');
```

`activateRow()` already reads `row.dataset.href` correctly (line 178) — no change needed there.

**Side effect check:** The deletions page rows emit `data-row` (not `data-href`), so after this fix j/k will correctly do nothing on the Deletions page (no `[data-href]` rows present there). This is correct behavior — deletions rows have cancel buttons, not navigation targets.

### Bug 2: INT-04 — cycleSortField() / reverseSortDir() selector mismatch (MEDIUM severity)

**Location:** `internal/web/static/keys.js:220, 222, 236`

**Broken code:**
```javascript
var sortLinks = document.querySelectorAll('.sort-header[data-sort-field]');
var activeField = document.querySelector('.sort-header.active');
// and:
var activeSort = document.querySelector('.sort-header.active');
```

**What `SortHeader` in `components.templ:56-77` actually emits:**
```html
<th class="sortable-header" hx-get="..." ...>
  <a href="..." class="sort-link">
    Label
    <!-- arrow only when field == currentField -->
    <span class="sort-arrow">↓</span>
  </a>
</th>
```

Three problems:
1. Class is `sortable-header`, not `sort-header`
2. No `data-sort-field` attribute on the `<th>`
3. No `active` class on the `<th>` when that column is the current sort field

**Fix requires two-part approach:**

**Part A — Add `data-sort-field` and `active` class to `SortHeader` in `components.templ`:**
```
templ SortHeader(label, field, currentField, currentDir, baseURL string) {
    <th
        class={ "sortable-header", templ.KV("active", field == currentField) }
        data-sort-field={ field }
        hx-get={ ... }
        ...
    >
```

**Part B — Update keys.js selectors to use `sortable-header`:**
```javascript
// Line 220: change
var sortLinks = document.querySelectorAll('.sort-header[data-sort-field]');
// to
var sortLinks = document.querySelectorAll('.sortable-header[data-sort-field]');

// Line 222: change
var activeField = document.querySelector('.sort-header.active');
// to
var activeField = document.querySelector('.sortable-header.active');

// Line 236: change
var activeSort = document.querySelector('.sort-header.active');
// to
var activeSort = document.querySelector('.sortable-header.active');
```

**Note on `templ.KV`:** This is the correct Templ API for conditional class application. Verified against `components_templ.go` patterns in codebase. `templ.KV("active", boolExpr)` adds the key "active" to the class attribute only when the bool is true.

**CSS side effect:** `style.css:211` already defines `.sort-header` styles but NOT `.sortable-header`. After this fix, the CSS class name for sort header styling needs to match. Options:
- Keep CSS as `.sort-header` and add class `sort-header` to `SortHeader` template alongside `sortable-header` (backward-compatible)
- Change CSS `.sort-header` → `.sortable-header` (cleaner, preferred)
- Add `.sortable-header` CSS block with same styles as `.sort-header`

**Recommendation:** Change `style.css` `.sort-header` → `.sortable-header` since `.sort-header` is only in `style.css` (not referenced in JS after the fix). Avoids dead CSS.

### Bug 3: INT-05 — bluemonday misclassified as indirect (LOW severity)

**Location:** `go.mod:65`

**Current state:**
```
github.com/microcosm-cc/bluemonday v1.0.27 // indirect
```

**What `go mod tidy` produces:**
```
require (
    ...
    github.com/microcosm-cc/bluemonday v1.0.27  // moves to direct
    ...
)
```

`sanitize_email.go:9` directly imports `github.com/microcosm-cc/bluemonday`. Running `go mod tidy` is the complete fix.

**Important:** `go mod tidy` modifies only `go.mod`, not `go.sum` (the sum is already correct since the package is already present). No `go get` needed — just `go mod tidy`.

### Bug 4: POLISH-03 — 09-02-SUMMARY.md missing frontmatter entry (doc fix)

**Location:** `.planning/phases/09-polish/09-02-SUMMARY.md`

**Current frontmatter (relevant section):**
```yaml
dependency_graph:
  requires: [09-01]
  provides: [POLISH-03]
  affects: [all web templates]
```

The `provides: [POLISH-03]` is present in `dependency_graph` but the audit checks `requirements_completed` in the frontmatter — which is absent from this file entirely. Other SUMMARY files in the project use a `requirements_completed` key at the top level.

**Fix:** Add `requirements_completed: [POLISH-03]` to the frontmatter.

---

## Standard Stack

No new libraries needed. All work is in existing files.

| File | Change Type | Scope |
|------|-------------|-------|
| `internal/web/static/keys.js` | Bug fix | 4 querySelectorAll/querySelector calls |
| `internal/web/templates/components.templ` | Enhancement | SortHeader `<th>` gets `data-sort-field` + conditional `active` class |
| `internal/web/templates/components_templ.go` | Regenerated | `make templ-generate` or `templ generate` |
| `internal/web/static/style.css` | Rename | `.sort-header` → `.sortable-header` |
| `go.mod` | Tidy | `bluemonday` promoted to direct dependency |
| `.planning/phases/09-polish/09-02-SUMMARY.md` | Doc fix | Add `requirements_completed: [POLISH-03]` to frontmatter |

---

## Architecture Patterns

### Templ Conditional Class Pattern

Verified from `components_templ.go` in the codebase. The Templ `templ.KV` function is the correct API for conditional classes:

```go
// In .templ file:
<th
    class={ "sortable-header", templ.KV("active", field == currentField) }
    data-sort-field={ field }
>
```

This produces `class="sortable-header active"` when `field == currentField` and `class="sortable-header"` otherwise.

**Alternative:** A Go helper function returning the full class string — acceptable but verbose. The `templ.KV` approach is idiomatic Templ.

### Generated File Protocol

When modifying `.templ` files, `_templ.go` files must be regenerated. Per project convention (from CLAUDE.md decisions): `_templ.go` files are committed to the repo so `go build` works without `templ` CLI. The Makefile has `templ-generate` target pinned to `v0.3.1001`.

```bash
make templ-generate
```

After regeneration, `go fmt ./...` and `go vet ./...` must pass (per CLAUDE.md).

### Keys.js Pattern: querySelectorAll returns live NodeList

`document.querySelectorAll()` returns a static NodeList (not live). After HTMX content swaps, `keys.js` already resets `currentRow = -1` on `htmx:afterSwap` (line 161). The `moveRow()` function calls `querySelectorAll` fresh on every keypress, so no caching issue. The fix is purely a selector string change.

### go mod tidy Protocol

Per project CLAUDE.md: "After making any Go code changes, always run `go fmt ./...` and `go vet ./...` before committing." For `go mod tidy` specifically, the result should be committed alongside any other changes in the same plan execution.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Conditional CSS classes in Templ | String concatenation logic | `templ.KV("class-name", boolExpr)` | Idiomatic Templ API, handles escaping |
| go.mod direct/indirect classification | Manual editing | `go mod tidy` | Tool knows the full import graph |

---

## Common Pitfalls

### Pitfall 1: Fixing keys.js but not SortHeader template

**What goes wrong:** If only `keys.js` selectors are updated to `sortable-header` but `SortHeader` still doesn't emit `data-sort-field` or `active` class, the sort cycling still silently does nothing (empty NodeList for `[data-sort-field]`, no element with `active` class).

**Prevention:** Both changes are required: update `components.templ` AND update `keys.js`. The plan must include both as a single atomic unit.

### Pitfall 2: Forgetting to regenerate `_templ.go`

**What goes wrong:** `components.templ` is modified but `components_templ.go` (the generated Go file that actually runs) is not regenerated. `go test ./internal/web/...` passes because the test server uses the generated Go, which hasn't changed. The runtime behavior remains broken.

**Prevention:** Always run `make templ-generate` after `.templ` file changes. Verify `components_templ.go` is in `git diff --staged` before committing.

### Pitfall 3: CSS class rename leaves dead styles

**What goes wrong:** `.sort-header` is renamed in templates to `sortable-header` but `style.css` still defines `.sort-header` styles. The style is never applied — sort headers lose their cursor:pointer styling.

**Prevention:** When renaming the class, update `style.css` in the same plan step. After the fix, grep confirms no remaining `.sort-header` references in templates.

### Pitfall 4: `data-row` removal breaks deletions page

**What goes wrong:** After changing `moveRow()` to query `[data-href]`, a developer notices deletions rows use `data-row` and wonders if that also needs updating.

**Clarification:** Deletions rows intentionally use `data-row` and should NOT be j/k navigable. The `activateRow()` function reads `row.dataset.href` — since deletions rows have no `data-href`, they would never navigate even if selected. The current separation is correct: `data-row` = metadata identifier for the deletion manifest; `data-href` = navigation target for keyboard nav. No change needed to `deletions.templ`.

### Pitfall 5: `go mod tidy` changes go.sum

**What goes wrong:** Running `go mod tidy` on some modules also modifies `go.sum`. In this case it should not, because `bluemonday` is already present as a dependency (just misclassified). But the executor should check `git diff go.sum` after `go mod tidy` and stage any changes.

**Prevention:** After `go mod tidy`, check `git status` for any `go.sum` changes and include them in the commit.

---

## Code Examples

### SortHeader with data-sort-field and conditional active class

```go
// Source: components.templ — add data-sort-field and conditional active class
templ SortHeader(label, field, currentField, currentDir, baseURL string) {
    <th
        class={ "sortable-header", templ.KV("active", field == currentField) }
        data-sort-field={ field }
        hx-get={ sortURL(baseURL, field, currentField, currentDir) }
        hx-select="#main-content"
        hx-target="#main-content"
        hx-swap="outerHTML"
        hx-replace-url="true"
        hx-indicator="#page-indicator"
    >
        <a href={ templ.SafeURL(sortURL(baseURL, field, currentField, currentDir)) } class="sort-link">
            { label }
            if field == currentField {
                if currentDir == "asc" {
                    <span class="sort-arrow">&uarr;</span>
                } else {
                    <span class="sort-arrow">&darr;</span>
                }
            }
        </a>
    </th>
}
```

### Updated keys.js moveRow function

```javascript
// Source: keys.js — change selector from [data-row] to [data-href]
function moveRow(delta) {
    var rows = document.querySelectorAll('[data-href]');  // was [data-row]
    if (!rows.length) return;
    if (currentRow >= 0 && currentRow < rows.length) {
        rows[currentRow].classList.remove('row-focused');
    }
    currentRow = Math.max(0, Math.min(rows.length - 1, currentRow + delta));
    rows[currentRow].classList.add('row-focused');
    rows[currentRow].scrollIntoView({ block: 'nearest' });
}
```

### Updated keys.js cycleSortField and reverseSortDir

```javascript
// Source: keys.js — change .sort-header to .sortable-header throughout
function cycleSortField() {
    var sortLinks = document.querySelectorAll('.sortable-header[data-sort-field]');  // was .sort-header
    if (!sortLinks.length) return;
    var activeField = document.querySelector('.sortable-header.active');  // was .sort-header.active
    var activeIdx = -1;
    sortLinks.forEach(function (link, i) {
        if (link === activeField) activeIdx = i;
    });
    var nextIdx = (activeIdx + 1) % sortLinks.length;
    var nextLink = sortLinks[nextIdx];
    if (nextLink) {
        nextLink.click();
    }
}

function reverseSortDir() {
    var activeSort = document.querySelector('.sortable-header.active');  // was .sort-header.active
    if (activeSort) {
        activeSort.click();
    }
}
```

### 09-02-SUMMARY.md frontmatter addition

```yaml
# Add this key at the top level of the frontmatter (alongside phase:, plan:, etc.)
requirements_completed: [POLISH-03]
```

### style.css rename

```css
/* Change .sort-header to .sortable-header — these are the only two occurrences */

/* ---- Sort headers ---- */

.sortable-header {   /* was .sort-header */
    cursor: pointer;
    user-select: none;
}

.sortable-header:hover {   /* was .sort-header:hover */
    color: var(--base0);
}
```

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + httptest |
| Config file | none — standard `go test` |
| Quick run command | `go test ./internal/web/... -run TestHandlersReturnHTML` |
| Full suite command | `go test ./internal/web/...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PARITY-07 | j/k moves row highlight; Enter navigates; s/r cycles sort | manual-only | n/a — DOM interaction requires browser | N/A |
| PARITY-07 | SortHeader emits `data-sort-field` and `active` class | unit | New test: `TestSortHeaderEmitsSortField` | ❌ Wave 0 |
| PARITY-07 | Messages page rows have `data-href` (not `data-row`) | unit | `go test ./internal/web/... -run TestMessages` (extend body check) | ✅ (extend) |
| POLISH-03 | 09-02-SUMMARY.md has `requirements_completed: [POLISH-03]` | manual | inspect file | N/A |

**Note on keyboard behavior:** The actual j/k/Enter/s/r behavior is JavaScript executing in a browser — it cannot be tested by Go httptest. The automated test coverage for this phase is: (1) verify that the HTML emitted by templates contains the expected attributes (`data-href` on rows, `data-sort-field` + `active` on sort headers), and (2) verify that `go test ./internal/web/...` passes. Browser-level verification is manual.

### Sampling Rate

- **Per task commit:** `go test ./internal/web/... -run TestHandlersReturnHTML -v`
- **Per wave merge:** `go test ./internal/web/...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/web/handlers_test.go` — add `TestSortHeaderEmitsSortField`: GET `/messages`, parse body, assert `data-sort-field` attribute present on `<th class="sortable-header">` elements

*(Existing tests already verify `data-href` appears on message rows implicitly through 200 responses; a targeted body-content assertion would be stronger but is not strictly required to close PARITY-07)*

---

## Scope Boundaries

**In scope for Phase 11:**
- `keys.js` selector fixes (INT-03, INT-04)
- `components.templ` SortHeader attribute additions
- `components_templ.go` regeneration
- `style.css` class rename
- `go.mod` tidy (INT-05)
- `09-02-SUMMARY.md` frontmatter fix (POLISH-03)

**Out of scope (pre-existing tech debt, not Phase 11):**
- `internal/mbox/client_test.go` missing helper definitions
- `cmd/msgvault/cmd` duplicate `TestEmailValidation`
- `internal/export/attachments_test.go` arity mismatch

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| React SPA with `[data-row]` for row selection | Templ templates emit `data-href` for navigation | Phase 6 (Foundation) | keys.js was never updated; j/k became no-ops |
| SortHeader had `sort-header` CSS class | SortHeader emits `sortable-header` CSS class | Phase 6 (Foundation) | keys.js `.sort-header` selectors never matched |

**Root cause:** keys.js was migrated from the React SPA era. The Templ template attributes were implemented differently (using `data-href` instead of `data-row` for navigation, `sortable-header` instead of `sort-header` for sort columns). No cross-file integration check caught the mismatch during Phases 6-10.

---

## Sources

### Primary (HIGH confidence)

- Direct code inspection of `internal/web/static/keys.js` — selector strings verified at lines 165, 220, 222, 236
- Direct code inspection of `internal/web/templates/components.templ` — `SortHeader` class at line 58
- Direct code inspection of `internal/web/templates/messages.templ` — row `data-href` at line 33
- Direct code inspection of `internal/web/templates/aggregate.templ` — row `data-href` at lines 191, 349
- Direct code inspection of `internal/web/templates/dashboard.templ` — row `data-href` at lines 73, 107
- Direct code inspection of `internal/web/templates/search.templ` — row `data-href` at line 77
- Direct code inspection of `internal/web/templates/deletions.templ` — row `data-row` (different attribute) at line 35
- `go mod tidy -diff` output — confirms bluemonday moves from indirect to direct
- `.planning/v1.1-MILESTONE-AUDIT.md` — INT-03, INT-04, INT-05 documented with exact line references

### Secondary (MEDIUM confidence)

- `internal/web/static/style.css:211-218` — `.sort-header` CSS rules present but class never emitted by templates

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies, verified existing file contents
- Architecture: HIGH — all selector values read directly from source files
- Pitfalls: HIGH — derived from exact mismatches verified in source, not speculation

**Research date:** 2026-03-11
**Valid until:** Until keys.js or any template file is modified (stable until then)
