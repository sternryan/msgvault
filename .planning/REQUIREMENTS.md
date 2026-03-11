# Requirements: msgvault v1.1

**Defined:** 2026-03-10
**Core Value:** Users can safely archive their entire Gmail history offline and search it instantly, with confidence that nothing is lost before deletion.

## v1.1 Requirements

Requirements for the Web UI rebuild. Each maps to roadmap phases.

### Foundation

- [x] **FOUND-01**: User can access the web UI from a single `go build` binary with no npm/Node.js dependency
- [x] **FOUND-02**: Web UI serves all pages via server-rendered Templ + HTMX (adopted from upstream PR #176)
- [x] **FOUND-03**: React SPA (`web/`), JSON API server (`internal/api/`), and all npm/Vite tooling are removed
- [x] **FOUND-04**: All static assets (HTMX, CSS, JS) are embedded via `go:embed` in the binary
- [x] **FOUND-05**: Generated `_templ.go` files are committed so `go build` works without the `templ` CLI installed

### Web UI Parity

- [x] **PARITY-01**: User can view dashboard with archive stats overview and time series chart
- [x] **PARITY-02**: User can browse aggregates with drill-down across all 7 view types (Senders, Sender Names, Recipients, Recipient Names, Domains, Labels, Time)
- [x] **PARITY-03**: User can view paginated message list with sort and filter
- [x] **PARITY-04**: User can search messages with full-text search (debounced input)
- [x] **PARITY-05**: User can view message detail with headers, body, and attachments
- [x] **PARITY-06**: User can stage messages for deletion and manage staged deletions
- [x] **PARITY-07**: User can navigate the web UI with Vim-style keyboard shortcuts
- [x] **PARITY-08**: User can filter all views by account (multi-account support)

### Email Rendering

- [x] **RENDER-01**: Email HTML bodies are sanitized server-side with bluemonday before rendering (XSS prevention)
- [ ] **RENDER-02**: Email HTML bodies render in sandboxed iframes so email CSS cannot break application layout
- [x] **RENDER-03**: CID image references in emails are substituted with local attachment URLs server-side
- [x] **RENDER-04**: External images in emails are blocked by default with an opt-in toggle to load them

### Thread View

- [ ] **THREAD-01**: User can view all messages in a conversation chronologically on a single page
- [ ] **THREAD-02**: Thread messages are collapsible via native HTML `<details>`, with the latest message pre-expanded
- [ ] **THREAD-03**: Inline images render directly in thread messages, other attachments as download links
- [ ] **THREAD-04**: User can navigate to thread view from message detail via link and `t` keyboard shortcut
- [ ] **THREAD-05**: User can scroll between thread messages with `n`/`p` keyboard shortcuts

### Polish

- [ ] **POLISH-01**: User can toggle between plain text and HTML rendering per message
- [ ] **POLISH-02**: Dashboard displays time-series data as a CSS bar chart (no JS charting library)
- [ ] **POLISH-03**: Loading indicators display during HTMX partial page updates

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### API

- **API-01**: JSON API for programmatic access (needed for MCP integration or mobile client)

### Security

- **SEC-01**: App-level encryption for database and attachments at rest

### Visualization

- **VIS-01**: Server-side SVG time-series charts for dashboard (upgrade from CSS bar chart)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Mobile app | Desktop/CLI tool, not a mobile use case |
| Gmail modification during sync | Sync is read-only by design |
| Infinite scroll | Breaks back button with HTMX; offset pagination is correct for archive browsing |
| Session-based auth | Personal local tool; no multi-user auth needed |
| Real-time updates / WebSocket | Archive is static data; request-response is sufficient |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| FOUND-01 | Phase 6 | Complete |
| FOUND-02 | Phase 6 | Complete |
| FOUND-03 | Phase 6 | Complete |
| FOUND-04 | Phase 6 | Complete |
| FOUND-05 | Phase 6 | Complete |
| PARITY-01 | Phase 6 | Complete |
| PARITY-02 | Phase 6 | Complete |
| PARITY-03 | Phase 6 | Complete |
| PARITY-04 | Phase 6 | Complete |
| PARITY-05 | Phase 6 | Complete |
| PARITY-06 | Phase 6 | Complete |
| PARITY-07 | Phase 6 | Complete |
| PARITY-08 | Phase 6 | Complete |
| RENDER-01 | Phase 7 | Complete |
| RENDER-02 | Phase 7 | Pending |
| RENDER-03 | Phase 7 | Complete |
| RENDER-04 | Phase 7 | Complete |
| THREAD-01 | Phase 8 | Pending |
| THREAD-02 | Phase 8 | Pending |
| THREAD-03 | Phase 8 | Pending |
| THREAD-04 | Phase 8 | Pending |
| THREAD-05 | Phase 8 | Pending |
| POLISH-01 | Phase 9 | Pending |
| POLISH-02 | Phase 9 | Pending |
| POLISH-03 | Phase 9 | Pending |

**Coverage:**
- v1.1 requirements: 25 total
- Mapped to phases: 25
- Unmapped: 0

---
*Requirements defined: 2026-03-10*
*Last updated: 2026-03-10 — traceability mapped to phases 6-9*
