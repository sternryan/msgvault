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

### Validated (v1.2)

- ✓ Azure OpenAI batch pipeline with checkpoint resumability, rate limiting, progress display — v1.2
- ✓ Semantic embeddings via text-embedding-3-small stored in sqlite-vec — v1.2
- ✓ Semantic and hybrid (RRF) search in CLI and web UI — v1.2
- ✓ AI categorization (8 categories) via GPT-4o-mini as auto-labels — v1.2
- ✓ Life event extraction with LifeVault-compatible JSON export — v1.2
- ✓ Entity extraction (person/company/date/amount) with searchable web page — v1.2
- ✓ TUI AI Categories view and web category dropdown filter — v1.2

## Current State

**v1.2 AI Archive Intelligence shipped 2026-04-12.** All 16 requirements delivered across 3 phases (7 plans).

Next milestone not yet planned. Candidates: relationship intelligence (v1.3), encryption at rest, Microsoft 365 connector.

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
| Azure OpenAI batch pipeline | Checkpoint resumability for 472K messages | ✓ Good — survived interruptions cleanly |
| sqlite-vec for embeddings | Single-file vector search, same SQLite connection | ✓ Good — no external vector DB needed |
| Combined LLM call (category+events+entities) | Halves API cost vs separate pipelines | ✓ Good — $0.15/M shared across tasks |
| Auto-labels (label_type='auto') | Reuses all existing label UI/filtering code | ✓ Good — zero new query logic for TUI/web |
| RRF hybrid search (k=60) | Simple, proven re-ranking without tuning | ✓ Good — standard approach |

---
## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-12 after v1.2 milestone completion*
