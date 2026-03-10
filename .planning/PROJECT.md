# msgvault

## What This Is

An offline Gmail archive tool that exports and stores email data locally with full-text search capabilities. Built for archiving 20+ years of Gmail data from multiple accounts, making it searchable, and eventually deleting emails from Gmail once safely archived. Single-binary Go application, public repo under vanboompow.

## Core Value

Users can safely archive their entire Gmail history offline and search it instantly, with confidence that nothing is lost before deletion.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

- Gmail Sync: Full/incremental sync, OAuth (browser + headless), rate limiting, resumable checkpoints
- MIME Parsing: Subject, body (text/HTML), attachments, charset detection
- Parquet ETL: DuckDB-based SQLite -> Parquet export with incremental updates
- Query Engine: DuckDB over Parquet for fast aggregate analytics
- TUI: Full-featured with drill-down navigation, search, selection, deletion staging
- UTF-8 Repair: Comprehensive encoding repair for all string fields
- Deletion Execution: Execute staged deletions via Gmail API (trash or permanent delete)
- Web UI: React 19 + TypeScript SPA with Dashboard, Messages, Aggregate, Search, Deletions, Thread, Message detail pages
- JSON API: Go API server with auth and rate limiting serving the React SPA

### Active

<!-- Current scope. Building toward these. -->

## Current Milestone: v1.1 Web UI Rebuild (Templ + HTMX)

**Goal:** Replace the React SPA with server-rendered Templ + HTMX to restore single-binary purity, then add thread view and inline attachment rendering.

**Target features:**
- Adopt upstream PR #176's Templ + HTMX web UI
- Remove React SPA, API server, and all npm/Node.js dependencies
- Thread/conversation view with chronological message display
- Inline attachment rendering (images, download links)
- HTML sanitization for email bodies
- Keyboard shortcuts for thread navigation

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- Mobile app — Desktop/CLI tool, not a mobile use case
- Gmail modification during sync — Sync is read-only by design

## Context

- Go single-binary architecture, CLI-first with Cobra commands
- SQLite as system of record, Parquet for analytics layer
- Bubble Tea TUI with lipgloss styling
- DuckDB for Parquet queries, content-addressed attachment storage
- Public repo under vanboompow GitHub org
- Data stored in ~/.msgvault/ by default (configurable via MSGVAULT_HOME)
- Pre-commit hooks enforce gofmt + golangci-lint

## Constraints

- **Language**: Go — established codebase, single-binary deployment
- **Storage**: SQLite + Parquet dual-layer — proven architecture, not changing
- **API**: Gmail API with OAuth2 — Google's requirements
- **Privacy**: All data stays local — core design principle

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go single binary | Simple deployment, no runtime deps | Good |
| SQLite + Parquet dual layer | SQLite for ACID, Parquet for analytics speed | Good |
| DuckDB for analytics | ~3000x faster than SQLite JOINs for aggregates | Good |
| Bubble Tea TUI | Rich terminal UI, Go-native | Good |
| Content-addressed attachments | Deduplication across messages | Good |
| React SPA for Web UI | Quick to build, feature-rich | ⚠️ Revisit — breaks single-binary, upstream won't accept |
| Templ + HTMX rebuild | Restores single-binary, upstream-compatible, no npm | — Pending |

---
*Last updated: 2026-03-10 after v1.1 milestone definition*
