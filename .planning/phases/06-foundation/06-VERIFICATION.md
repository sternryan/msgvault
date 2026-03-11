---
phase: 06-foundation
verified: 2026-03-10T12:00:00Z
status: human_needed
score: 13/13 must-haves verified (automated)
re_verification: false
human_verification:
  - test: "Open http://localhost:8484/ after make build && ./msgvault web --port 8484 and verify dashboard displays stat cards with real data (message count, accounts, total size, attachment count) and top senders/domains tables"
    expected: "Stat grid with 4 cards showing real numeric data; two top-5 tables with clickable rows linking to drill-down"
    why_human: "Dashboard rendering requires a populated ~/.msgvault/msgvault.db — can't verify against real data programmatically"
  - test: "Visit /messages, click sortable column headers (Date, Subject, Size), and verify URL updates and rows re-sort"
    expected: "50 rows per page; sort arrows appear on active column; URL gains ?sortField=&sortDir= params; HTMX partial update (no full reload)"
    why_human: "Sorting requires live engine data and browser observation of HTMX partial update behavior"
  - test: "Visit /aggregate, click through all 7 view-type tabs (Senders, Sender Names, Recipients, Recipient Names, Domains, Labels, Time), click a row to drill down, verify breadcrumbs, click a sub-view tab"
    expected: "Each tab loads correct data; drill-down shows messages for the row key; breadcrumbs are navigable; sub-view tabs call SubAggregate and show a second aggregate table"
    why_human: "Seven-tab navigation, drill-down, and sub-aggregate all require real engine data and interactive browser flow"
  - test: "Visit /search, type 'from:test', wait for debounce, observe loading indicator, verify results appear without full page reload"
    expected: "500ms debounce fires; 'Searching...' indicator appears; results table renders below input; URL updates to ?q=from:test; mode badge shows '(metadata search)' or '(full-text search)'"
    why_human: "Debounce timing and htmx-indicator display require real browser interaction"
  - test: "From an aggregate drill-down messages view, click 'Stage for Deletion', accept confirmation, verify navbar badge count increments without page reload, then visit /deletions to see the manifest"
    expected: "Badge increments via HTMX OOB swap; StageResult banner appears in-place; /deletions shows the new manifest with Pending status and a Cancel button"
    why_human: "OOB swap behavior, badge update, and deletion manifest creation require a live server session"
  - test: "Test keyboard shortcuts: j/k to navigate rows, Enter to open, Esc to go back, Tab to cycle aggregate view types, / to focus search, ? to show help overlay, q to do nothing"
    expected: "j/k highlights rows (row-focused class); Enter navigates via htmx.ajax; Esc calls history.back(); Tab triggers next .view-tab htmx-get; / focuses .search-input; ? renders help overlay in DOM; q is a no-op (no accidental tab close)"
    why_human: "All keyboard behaviors are pure browser interaction, not testable via httptest"
  - test: "With multiple accounts configured, change the account filter dropdown and verify all pages filter to that account, and the URL sourceId param updates correctly"
    expected: "Changing the select triggers htmx.ajax via setupAccountFilter(); URL gains ?sourceId=N; all page data filters to the selected account; existing query params (groupBy, q, etc.) are preserved"
    why_human: "JavaScript URL manipulation and multi-account filter behavior require a live browser with real account data"
---

# Phase 6: Foundation Verification Report

**Phase Goal:** Users can access a fully functional web UI built from a single `go build` binary with no npm, no Node.js, and no CDN dependencies, with feature parity across all pages the React SPA provided

**Verified:** 2026-03-10
**Status:** HUMAN_NEEDED — all automated checks pass; 7 items require browser verification
**Re-verification:** No — initial verification


## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | `go build` produces a working binary with no npm/Node.js | VERIFIED | `go build -tags fts5 -o /dev/null ./cmd/msgvault` exits 0; Makefile `build` target has no `web-build` dependency |
| 2 | All static assets (htmx, CSS, JS) are embedded in the binary | VERIFIED | `//go:embed static/*` in embed.go; staticSubFS() serves all three files; TestStaticFiles passes for all three |
| 3 | HTTP server serves a base HTML page with Solarized Dark layout at / | VERIFIED | TestHandlersReturnHTML passes; layout.templ links `/static/style.css`; style.css is 737 lines with complete Solarized Dark theme |
| 4 | All 5 page routes return 200 with text/html | VERIFIED | TestHandlersReturnHTML passes all 5 subtests (dashboard, messages, aggregate, search, deletions) |
| 5 | Dashboard renders stat cards with real engine data | VERIFIED | handlers_dashboard.go calls GetTotalStats + Aggregate (top senders/domains); TestDashboard passes |
| 6 | Messages list is paginated (50/page) with sortable columns | VERIFIED | handlers_messages.go sets Limit=50; MessagesPage uses SortHeader; TestMessages passes |
| 7 | Message detail renders headers, body, attachments | VERIFIED | MessageDetailPage in message.templ renders From/To/Cc/Bcc, BodyText pre-wrap, attachment download links; TestMessageDetail passes |
| 8 | Aggregate shows 7 view types with drill-down and breadcrumbs | VERIFIED | AggregatePage has 7 viewTabs; aggregateDrilldown calls SubAggregate or ListMessages; breadcrumbs built in handler; TestAggregate passes |
| 9 | Search uses DuckDB fast path first, FTS5 fallback | VERIFIED | handlers_search.go: SearchFast first, Search() fallback when len(results)==0 and len(TextTerms)>0; TestSearch passes |
| 10 | Deletions page lists manifests; staging creates manifest with OOB badge update | VERIFIED | handlers_deletions.go collectAllManifests; stageDeletion renders StageResult + DeletionBadgeOOB (hx-swap-oob="true"); TestStageDeletion passes |
| 11 | Keyboard shortcuts: j/k nav, Enter open, Esc back, Tab cycle, q no-op | VERIFIED | keys.js is 268-line full implementation; all cases in switch including `case 'q': e.preventDefault()` |
| 12 | Account filter propagates across all views | VERIFIED | setupAccountFilter() in keys.js uses JS URL manipulation; account-filter select has no HTMX attrs (JS handles it); TestAccountFilter passes |
| 13 | web/ and internal/api/ are deleted; build requires no npm | VERIFIED | `test ! -d web` and `test ! -d internal/api` both pass; git commit 3f0659a removed all 36 files |

**Score: 13/13 automated truths verified**


### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/web/embed.go` | go:embed static/* directive | VERIFIED | `//go:embed static/*` present; staticSubFS() exports fs.FS |
| `internal/web/server.go` | chi router with all routes, buildRouter() | VERIFIED | All 11 routes registered; buildRouter() extracted for testability |
| `internal/web/handlers.go` | handlers struct, renderPage, renderError, pendingDeletionCount | VERIFIED | All three helpers implemented with real logic (not stubs) |
| `internal/web/handlers_dashboard.go` | Dashboard handler with GetTotalStats + Aggregate | VERIFIED | Calls GetTotalStats, Aggregate (senders), Aggregate (domains) |
| `internal/web/handlers_messages.go` | Messages list + message detail | VERIFIED | messagesList(Limit=50) + messageDetail with chi.URLParam |
| `internal/web/handlers_aggregate.go` | Aggregate + aggregateDrilldown | VERIFIED | 214 lines; applyKeyToFilter for all 7 view types; SubAggregate for filterView |
| `internal/web/handlers_search.go` | SearchFast + Search fallback | VERIFIED | Two-tier search implemented; mode tracking; SearchFastCount for pagination |
| `internal/web/handlers_deletions.go` | Deletions page, stageDeletion, cancelDeletion | VERIFIED | collectAllManifests; OOB badge via DeletionBadgeOOB; form POST parsing |
| `internal/web/params.go` | Parameter parsing helpers | VERIFIED | parseViewType, parseSortField, parseSortDirection, parseTimeGranularity, parseMessageSortField, parseAggregateOptions, parseMessageFilter, intParam, clampInt |
| `internal/web/static/htmx.min.js` | HTMX 2.0.8 vendored | VERIFIED | 51,250 bytes, single-line minified JS |
| `internal/web/static/style.css` | Solarized Dark CSS | VERIFIED | 737 lines; all component classes present |
| `internal/web/static/keys.js` | Full keyboard shortcut handler | VERIFIED | 268 lines; all shortcuts including q no-op, setupAccountFilter, toggleHelp |
| `internal/web/templates/layout.templ` | Base layout with navbar, account dropdown, deletion badge | VERIFIED | 101 lines; all 5 nav links with HTMX attrs; account-filter select; deletion-badge span |
| `internal/web/templates/dashboard.templ` | Dashboard with stat cards, top senders/domains | VERIFIED | 104 lines; stat-grid with 4 cards; top-lists with clickable rows |
| `internal/web/templates/messages.templ` | Message list with pagination, sort | VERIFIED | 93 lines; SortHeader used; data-href on rows; Pagination component |
| `internal/web/templates/message.templ` | Message detail: headers, body, attachments | VERIFIED | 98 lines; From/To/Cc/Bcc/Date/Size/Labels; BodyText pre-wrap; attachment download links |
| `internal/web/templates/components.templ` | Pagination and SortHeader components | VERIFIED | 122 lines; both components with HTMX attrs and hx-replace-url |
| `internal/web/templates/aggregate.templ` | All 7 view tabs, filter bar, drill-down, staging form | VERIFIED | 402 lines; viewTabs slice; stagingForm for all 7 groupBy types; sub-view tabs with filterView |
| `internal/web/templates/search.templ` | Search input with debounce, results, mode badge | VERIFIED | 96 lines; hx-trigger delay:500ms; htmx-indicator; mode badge; syntax help |
| `internal/web/templates/deletions.templ` | Deletions page, DeletionBadgeOOB, StageResult | VERIFIED | 98 lines; hx-delete cancel; hx-swap-oob="true" on badge; hx-confirm |
| `internal/web/templates/helpers.go` | FormatBytes, FormatNum, FormatTime, FormatDate, Pluralize | VERIFIED | All 5 helpers implemented with real logic |
| `internal/web/templates/*_templ.go` | Generated files committed | VERIFIED | 9 _templ.go files present: aggregate, components, dashboard, deletions, layout, message, messages, search, stub |
| `internal/web/handlers_test.go` | Integration tests with mock Engine | VERIFIED | 9 test functions all pass; mockEngine + real router via buildRouter() |
| `Makefile` | Pure go build, templ-generate target | VERIFIED | build target: `CGO_ENABLED=1 go build -tags fts5`; templ-generate target present; no npm/web-build |


### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| server.go | embed.go | staticSubFS() for /static/* | VERIFIED | `http.FileServer(http.FS(staticSubFS()))` at line 53 |
| server.go | handlers.go | chi route registration | VERIFIED | 11 `r.Get`/`r.Post`/`r.Delete` calls |
| layout.templ | /static/style.css | link href=/static/style.css | VERIFIED | `<link rel="stylesheet" href="/static/style.css"/>` |
| layout.templ | /static/htmx.min.js | script src | VERIFIED | `<script src="/static/htmx.min.js">` |
| layout.templ | /static/keys.js | script src | VERIFIED | `<script src="/static/keys.js">` |
| handlers_dashboard.go | query.Engine | GetTotalStats + Aggregate | VERIFIED | `h.engine.GetTotalStats` + `h.engine.Aggregate` (both calls present) |
| handlers_messages.go | query.Engine | ListMessages + GetMessage | VERIFIED | `h.engine.ListMessages` + `h.engine.GetMessage` |
| handlers_aggregate.go | query.Engine | Aggregate + SubAggregate | VERIFIED | `h.engine.Aggregate` + `h.engine.SubAggregate` (conditional on filterViewStr) |
| handlers_search.go | query.Engine | SearchFast + Search fallback | VERIFIED | `h.engine.SearchFast` + `h.engine.Search` + `h.engine.SearchFastCount` |
| handlers_deletions.go | deletion.Manager | CreateManifest, SaveManifest, ListPending, CancelManifest | VERIFIED | `h.deletions.CreateManifest`, `h.deletions.SaveManifest`, `h.deletions.ListPending`, `h.deletions.CancelManifest` |
| handlers_deletions.go | templates.DeletionBadgeOOB | OOB swap for navbar badge | VERIFIED | `templates.DeletionBadgeOOB(pendingCount).Render(r.Context(), w)` in both stageDeletion and cancelDeletion |
| keys.js | account-filter select | setupAccountFilter() JS URL manipulation | VERIFIED | `document.getElementById('account-filter')` + htmx.ajax with URL param replacement |
| aggregate.templ (drilldown) | /deletions/stage | stagingForm component with groupBy-specific hidden fields | VERIFIED | `@stagingForm(groupBy, filterKey, sourceID)` rendered when `subViewType == ""` |
| messages.templ | components.templ | Pagination component | VERIFIED | `@Pagination(baseURL, filter.Pagination.Offset, filter.Pagination.Limit, total)` |
| aggregate.templ | components.templ | SortHeader component | VERIFIED | `@SortHeader("Name", "name", sortField, sortDir, baseURL)` etc. |
| Makefile | go build | build target calls go build only | VERIFIED | No web-build dependency; `CGO_ENABLED=1 go build -tags fts5` directly |


### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| FOUND-01 | 06-01, 06-05 | Single go build binary, no npm/Node.js | SATISFIED | Build succeeds; Makefile has no npm dependency |
| FOUND-02 | 06-01 | Server-rendered Templ + HTMX | SATISFIED | All pages use templ components served via chi |
| FOUND-03 | 06-05 | React SPA, JSON API, npm tooling removed | SATISFIED | web/ and internal/api/ do not exist in repo |
| FOUND-04 | 06-01 | Static assets embedded via go:embed | SATISFIED | embed.go has `//go:embed static/*` |
| FOUND-05 | 06-01, 06-05 | _templ.go files committed, no templ CLI required | SATISFIED | 9 _templ.go files tracked by git; templ-generate is dev-only |
| PARITY-01 | 06-02 | Dashboard with archive stats overview | SATISFIED | Stat cards (messages, accounts, size, attachments) + top senders/domains. Note: time series chart deferred to POLISH-02 (Phase 9) per locked decision in 06-02-PLAN.md |
| PARITY-02 | 06-03 | Aggregates with 7-view drill-down | SATISFIED | 7 view-type tabs; drill-down to messages; sub-view tabs trigger SubAggregate |
| PARITY-03 | 06-02 | Paginated message list with sort | SATISFIED | 50 rows/page; SortHeader for date/size/subject; Pagination component |
| PARITY-04 | 06-03 | Full-text search with debounced input | SATISFIED | 500ms debounce; SearchFast + FTS5 fallback; mode badge |
| PARITY-05 | 06-02 | Message detail with headers, body, attachments | SATISFIED | MessageDetailPage renders From/To/Cc/Bcc, plain text body, attachment download links |
| PARITY-06 | 06-04 | Stage messages for deletion, manage staged | SATISFIED | stageDeletion creates manifest; DeletionsPage lists all statuses; Cancel button removes pending |
| PARITY-07 | 06-04 | Vim-style keyboard shortcuts | SATISFIED | keys.js: j/k/Enter/Esc/Tab/s/r/t/a//?/q all implemented |
| PARITY-08 | 06-04 | Filter all views by account | SATISFIED | setupAccountFilter() JS URL manipulation; sourceId propagated to all handlers |


### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/web/templates/stub.templ` | 5-17 | StubPage + ErrorContent still exist | INFO | Stub components remain but are only used in tests; no production handler calls StubPage — all handlers have real implementations. Not a blocker. |
| `internal/web/templates/message.templ` | 61 | "HTML body available — full rendering coming in a future update" | INFO | Intentional design decision (HTML sanitization deferred to Phase 7 / RENDER-01,02). Not a stub — this is the correct behavior for Phase 6. |

No blockers or warnings found. The two INFO items are intentional design decisions documented in the plans.


### Human Verification Required

#### 1. Dashboard with Real Data

**Test:** Run `make build && ./msgvault web --port 8484`, visit http://localhost:8484/
**Expected:** Stat grid shows real message count, account count, total size, attachment count; top senders table and top domains table appear with real email addresses and message counts
**Why human:** Requires a populated ~/.msgvault/msgvault.db — httptest mock cannot validate real data rendering

#### 2. Messages Sort and Pagination

**Test:** Visit /messages, click Date/Subject/Size column headers, navigate pages
**Expected:** Sort arrow appears on active column; URL gains sortField/sortDir params; 50 rows per page; Prev/Next pagination works via HTMX partial update (no full page reload)
**Why human:** Live engine data required; HTMX partial update behavior only observable in browser

#### 3. Aggregate 7-Tab Drill-Down with Sub-View Tabs

**Test:** Visit /aggregate, click each of the 7 view-type tabs, click a row to drill down, verify breadcrumbs link back, click a sub-view tab in drill-down
**Expected:** Each tab shows correct data for its view type; drill-down page shows messages for clicked row; breadcrumbs show "Aggregate > Senders > alice@example.com"; sub-view tab (e.g., Domains) shows sub-aggregate table
**Why human:** Seven distinct data shapes, navigation state, and SubAggregate call require real engine + browser

#### 4. Search Debounce and Mode Indicator

**Test:** Visit /search, type "from:alice", wait ~600ms, observe
**Expected:** Searching... indicator appears during request; results table renders below input; URL updates to ?q=from%3Aalice; "(metadata search)" or "(full-text search)" badge appears
**Why human:** Debounce timing and htmx-indicator CSS class behavior only observable in real browser

#### 5. Deletion Staging with OOB Badge Update

**Test:** Drill into an aggregate row (senders view), click "Stage for Deletion", accept confirmation
**Expected:** Stage success banner appears in-place; navbar Deletions badge count increments without full page reload; visit /deletions to see new Pending manifest with Cancel button
**Why human:** HTMX OOB swap behavior (hx-swap-oob="true") must be observed in browser; requires live deletion.Manager

#### 6. Keyboard Shortcuts

**Test:** On /messages or /aggregate, press j, k, Enter, Esc, Tab, /, ?, q in sequence
**Expected:** j/k highlights rows with row-focused class; Enter navigates to focused row; Esc goes back; Tab cycles view-type tabs on aggregate page; / focuses search input; ? shows help overlay; q does nothing
**Why human:** All keyboard behavior is DOM-event-driven, not testable via httptest

#### 7. Account Filter Propagation

**Test:** With multiple synced accounts, change the account dropdown on dashboard, then navigate to messages and aggregate
**Expected:** All pages filter to selected account; URL gains ?sourceId=N; existing params (groupBy, q, etc.) are preserved when switching accounts; changing back to "All accounts" removes sourceId
**Why human:** Requires multiple configured accounts; JavaScript URL manipulation behavior requires browser observation


### Gaps Summary

No automated gaps found. All 13 observable truths are verified. All 24 artifacts are substantive and wired. All 13 requirements have implementation evidence.

The phase is in human_needed state because 7 interactive behaviors (visual rendering with real data, HTMX partial updates, keyboard events, OOB swaps) cannot be verified programmatically without a live browser session.

The time series chart listed in PARITY-01's description is explicitly deferred to POLISH-02 (Phase 9) per a locked decision in 06-02-PLAN.md. The dashboard satisfies PARITY-01 with its stats overview; the chart is not a Phase 6 gap.

---

_Verified: 2026-03-10_
_Verifier: Claude (gsd-verifier)_
