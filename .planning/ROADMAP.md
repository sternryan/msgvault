# Roadmap: msgvault

## Milestones

- ✅ **v1.0 Core Archive & Search** - Phases 1-5 (shipped 2026-03-10)
- 🚧 **v1.1 Web UI Rebuild (Templ + HTMX)** - Phases 6-9 (in progress)

## Phases

<details>
<summary>✅ v1.0 Core Archive & Search (Phases 1-5) - SHIPPED 2026-03-10</summary>

Phases 1-5 delivered the complete offline Gmail archiver: full/incremental sync, MIME parsing, Parquet ETL, DuckDB query engine, full-featured TUI, UTF-8 repair, deletion execution, React SPA Web UI, and JSON API server.

</details>

---

### 🚧 v1.1 Web UI Rebuild (Templ + HTMX) (In Progress)

**Milestone Goal:** Replace the React SPA with server-rendered Templ + HTMX to restore single-binary purity, then add thread view and inline attachment rendering.

## Phases (v1.1)

- [x] **Phase 6: Foundation** - Adopt PR #176's Templ + HTMX UI, achieve full parity with React SPA, produce single `go build` binary with no npm (completed 2026-03-11)
- [x] **Phase 7: Email Rendering** - Sanitize and sandbox HTML email bodies; serve inline attachments with CID substitution and external image blocking (completed 2026-03-11)
- [x] **Phase 8: Thread View** - Full conversation view with collapsible messages, inline images, and keyboard navigation (completed 2026-03-11)
- [x] **Phase 9: Polish** - Text/HTML toggle, loading indicators, CSS bar chart for dashboard, and final validation pass (completed 2026-03-11)
- [x] **Phase 10: Integration Test & DOM Cleanup** - Fix stale test assertions and duplicate DOM IDs from cross-phase integration (completed 2026-03-11)
- [x] **Phase 11: Keyboard Selector Fix & Cleanup** - Fix DOM selector mismatches in keys.js so j/k/Enter row nav and s/r sort cycling work on all pages (completed 2026-03-11)

## Phase Details

### Phase 6: Foundation
**Goal**: Users can access a fully functional web UI built from a single `go build` binary with no npm, no Node.js, and no CDN dependencies, with feature parity across all pages the React SPA provided
**Depends on**: Nothing (first v1.1 phase; v1.0 code is existing baseline)
**Requirements**: FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05, PARITY-01, PARITY-02, PARITY-03, PARITY-04, PARITY-05, PARITY-06, PARITY-07, PARITY-08
**Success Criteria** (what must be TRUE):
  1. Running `go build` with no `templ` CLI installed produces a working binary that serves the web UI at localhost
  2. The `web/` directory, `internal/api/` package, `package.json`, and all npm/Vite artifacts are absent from the repository
  3. User can access Dashboard, Messages, Aggregate, Search, Message Detail, and Deletions pages — all functional with real data
  4. User can filter any view by account and navigate with Vim-style keyboard shortcuts
  5. Staging a message for deletion updates the deletion badge count without a full page reload
**Plans:** 5/5 plans complete

Plans:
- [ ] 06-01-PLAN.md — Templ + HTMX + chi foundation (static assets, embed, router, layout template, stub handlers)
- [ ] 06-02-PLAN.md — Dashboard, Messages list, and Message Detail pages with real data
- [ ] 06-03-PLAN.md — Aggregate page with 7 view types and drill-down + Search page with debounced live search
- [ ] 06-04-PLAN.md — Deletions staging with OOB badge, keyboard shortcuts, account filter propagation
- [ ] 06-05-PLAN.md — Cleanup (delete React SPA and JSON API) + end-to-end browser verification

### Phase 7: Email Rendering
**Goal**: HTML email bodies render correctly and securely — sanitized before reaching the browser, isolated in sandboxed iframes so email CSS cannot break application layout, with inline images resolved from local attachments and external images blocked by default
**Depends on**: Phase 6
**Requirements**: RENDER-01, RENDER-02, RENDER-03, RENDER-04
**Success Criteria** (what must be TRUE):
  1. Viewing a message with an HTML body shows the rendered email inside a sandboxed iframe without breaking the application's Solarized layout
  2. A malicious `<script>` tag in an email body does not execute in the browser (bluemonday strips it server-side before render)
  3. CID image references in an email body display as inline images served from the local attachment store
  4. External images in email bodies are hidden by default; clicking an opt-in toggle reveals them without a page reload
**Plans:** 2/2 plans complete

Plans:
- [ ] 07-01-PLAN.md — Schema migration (content_id), data layer updates, bluemonday sanitization pipeline with CID substitution and external image blocking
- [ ] 07-02-PLAN.md — Body endpoint, iframe rendering, CSS/JS integration, CID backfill, and visual verification

### Phase 8: Thread View
**Goal**: Users can read an entire email conversation on a single page, with messages displayed chronologically, earlier messages collapsible, and inline images rendering without any new attachment infrastructure
**Depends on**: Phase 7
**Requirements**: THREAD-01, THREAD-02, THREAD-03, THREAD-04, THREAD-05
**Success Criteria** (what must be TRUE):
  1. User can navigate to a thread view from the message detail page via a link and the `t` keyboard shortcut
  2. All messages in a conversation appear chronologically on one page, with the most recent message pre-expanded and earlier messages collapsed
  3. Expanding a collapsed message loads its body without a full page reload; inline images display directly, other attachments appear as download links
  4. User can move between messages in the thread using `n` (next) and `p` (previous) keyboard shortcuts
**Plans:** 2/2 plans complete

Plans:
- [ ] 08-01-PLAN.md — Thread handler, route, templ template, and collapsible message rendering with lazy-load iframes
- [ ] 08-02-PLAN.md — "View thread" link on message detail, t/n/p keyboard shortcuts, highlight/scroll, thread CSS

### Phase 9: Polish
**Goal**: The web UI is complete — text/HTML body toggle works, the dashboard chart displays time-series data, loading indicators appear during partial updates, and all React SPA artifacts are confirmed absent
**Depends on**: Phase 8
**Requirements**: POLISH-01, POLISH-02, POLISH-03
**Success Criteria** (what must be TRUE):
  1. On a message with both text and HTML parts, user can toggle between plain text and HTML rendering and the preference persists in the URL
  2. The dashboard displays a time-series bar chart of archive volume using only CSS (no JS charting library)
  3. Triggering any HTMX partial update (search, sort, pagination, staging) shows a visible loading indicator until the response arrives
**Plans:** 2/2 plans complete

Plans:
- [ ] 09-01-PLAN.md — Text/HTML body toggle (message detail + thread) and CSS bar chart for dashboard
- [ ] 09-02-PLAN.md — Loading indicators on all HTMX partial update trigger points

### Phase 10: Integration Test & DOM Cleanup
**Goal**: Cross-phase integration debt is resolved — stale test assertions match current implementation and thread view DOM is spec-compliant
**Depends on**: Phase 9
**Requirements**: RENDER-04, POLISH-01 (integration fix, not re-implementation)
**Gap Closure:** Closes INT-01, INT-02 from v1.1 milestone audit
**Success Criteria** (what must be TRUE):
  1. `TestMessageBodyWrapperEndpoint` passes with assertions matching Phase 9's unified toolbar (`email-toolbar`, `closest .email-render-wrapper`)
  2. Thread view lazy-loaded messages use unique IDs (e.g., `email-body-wrapper-{messageID}`) instead of duplicate `id=email-body-wrapper`
  3. `go test ./internal/web/...` passes with no test failures
**Plans:** 1/1 plans complete

Plans:
- [ ] 10-01-PLAN.md — Fix stale test assertions (INT-01) and duplicate DOM IDs in thread lazy-load (INT-02)

### Phase 11: Keyboard Selector Fix & Cleanup
**Goal**: Keyboard shortcuts j/k/Enter (row navigation) and s/r (sort cycling) work correctly on all pages — DOM selectors in keys.js match the attributes emitted by Templ templates
**Depends on**: Phase 10
**Requirements**: PARITY-07, POLISH-03 (doc fix)
**Gap Closure:** Closes INT-03, INT-04, INT-05, PARITY-07, POLISH-03 from v1.1 milestone audit
**Success Criteria** (what must be TRUE):
  1. Pressing j/k on messages, aggregate, search, and dashboard pages moves the active row highlight and Enter opens the selected row
  2. Pressing s on a page with sort headers cycles the sort field; pressing r reverses sort direction
  3. `go mod tidy` produces no changes (bluemonday correctly classified)
  4. 09-02-SUMMARY.md frontmatter includes POLISH-03 in `requirements_completed`
  5. `go test ./internal/web/...` passes with no failures
**Plans:** 1/1 plans complete

Plans:
- [ ] 11-01-PLAN.md — Fix keys.js selectors (data-row to data-href, sort-header to sortable-header), add data-sort-field + active class to SortHeader, go mod tidy, SUMMARY frontmatter fix

## Progress

**Execution Order:**
Phases execute in numeric order: 6 → 7 → 8 → 9 → 10 → 11

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-5. Core Archive | v1.0 | - | Complete | 2026-03-10 |
| 6. Foundation | 5/5 | Complete   | 2026-03-11 | - |
| 7. Email Rendering | 2/2 | Complete   | 2026-03-11 | - |
| 8. Thread View | 2/2 | Complete   | 2026-03-11 | - |
| 9. Polish | 2/2 | Complete   | 2026-03-11 | - |
| 10. Integration Test & DOM Cleanup | 1/1 | Complete    | 2026-03-11 | - |
| 11. Keyboard Selector Fix & Cleanup | 1/1 | Complete    | 2026-03-11 | - |
