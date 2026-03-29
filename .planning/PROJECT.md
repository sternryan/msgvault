# msgvault

## What This Is

An offline Gmail archive tool that exports and stores email data locally with full-text search capabilities. Single-binary Go application with server-rendered web UI (Templ + HTMX). Built for archiving 20+ years of Gmail data from multiple accounts, making it searchable, and eventually deleting emails from Gmail once safely archived. Public repo under sternryan.

## Core Value

Users can safely archive their entire Gmail history offline and search it instantly, with confidence that nothing is lost before deletion.

## Requirements

### Validated

- ✓ Gmail Sync: Full/incremental sync, OAuth (browser + headless), rate limiting, resumable checkpoints — v1.0
- ✓ MIME Parsing: Subject, body (text/HTML), attachments, charset detection — v1.0
- ✓ Parquet ETL: DuckDB-based SQLite → Parquet export with incremental updates — v1.0
- ✓ Query Engine: DuckDB over Parquet for fast aggregate analytics — v1.0
- ✓ TUI: Full-featured with drill-down navigation, search, selection, deletion staging — v1.0
- ✓ UTF-8 Repair: Comprehensive encoding repair for all string fields — v1.0
- ✓ Deletion Execution: Execute staged deletions via Gmail API (trash or permanent delete) — v1.0
- ✓ Web UI (Templ + HTMX): Server-rendered single-binary web UI with Dashboard, Messages, Aggregate, Search, Deletions, Message Detail pages — v1.1
- ✓ HTML Email Rendering: bluemonday sanitization, sandboxed iframes, CID image substitution, external image blocking — v1.1
- ✓ Thread View: Chronological conversation display with collapsible messages, lazy-load bodies, inline images — v1.1
- ✓ Text/HTML Toggle: Per-message format switching with URL persistence — v1.1
- ✓ Keyboard Navigation: j/k row nav, s/r sort cycling, t/n/p thread nav, Tab/Enter/Esc across all pages — v1.1
- ✓ Loading Indicators: HTMX indicator on all 42 partial update trigger points — v1.1
- ✓ CSS Bar Chart: Pure CSS time-series chart on dashboard — v1.1

### Active

(None — next milestone not yet defined)

### Out of Scope

- Mobile app — Desktop/CLI tool, not a mobile use case
- Gmail modification during sync — Sync is read-only by design
- Infinite scroll — Breaks back button with HTMX; offset pagination is correct for archive browsing
- Session-based auth — Personal local tool; no multi-user auth needed
- Real-time updates / WebSocket — Archive is static data; request-response is sufficient

## Context

- Go single-binary architecture (8,654 LOC in web package alone), CLI-first with Cobra commands
- SQLite as system of record, Parquet for analytics layer
- Templ + HTMX server-rendered web UI (replaced React SPA in v1.1)
- Bubble Tea TUI with lipgloss styling
- DuckDB for Parquet queries, content-addressed attachment storage
- Public repo under sternryan GitHub account
- Data stored in ~/.msgvault/ by default (configurable via MSGVAULT_HOME)
- Pre-commit hooks enforce gofmt + golangci-lint

## Constraints

- **Language**: Go — established codebase, single-binary deployment
- **Storage**: SQLite + Parquet dual-layer — proven architecture, not changing
- **API**: Gmail API with OAuth2 — Google's requirements
- **Privacy**: All data stays local — core design principle
- **Web UI**: Templ + HTMX — single-binary purity, no npm/Node.js dependency

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go single binary | Simple deployment, no runtime deps | ✓ Good |
| SQLite + Parquet dual layer | SQLite for ACID, Parquet for analytics speed | ✓ Good |
| DuckDB for analytics | ~3000x faster than SQLite JOINs for aggregates | ✓ Good |
| Bubble Tea TUI | Rich terminal UI, Go-native | ✓ Good |
| Content-addressed attachments | Deduplication across messages | ✓ Good |
| React SPA for Web UI (v1.0) | Quick to build, feature-rich | ⚠️ Replaced — broke single-binary, upstream won't accept |
| Templ + HTMX rebuild (v1.1) | Restores single-binary, upstream-compatible, no npm | ✓ Good — 8,654 LOC, all features ported |
| bluemonday + sandboxed iframe | Defense in depth for email HTML rendering | ✓ Good — no allow-same-origin |
| CID substitution before sanitization | bluemonday strips cid: URL scheme | ✓ Good — order matters |
| HTMX outerHTML swap for image toggle | No JS iframe src mutation needed | ✓ Good — clean HTMX pattern |
| CSS-only bar chart | No JS charting library dependency | ✓ Good — simple, effective |
| Universal #page-indicator | Persistent across #main-content swaps | ✓ Good — simpler than per-trigger |

---
*Last updated: 2026-03-11 after v1.1 milestone completion*
