# Milestones: msgvault

## v1.2 AI Archive Intelligence (Shipped: 2026-04-12)

**Phases completed:** 3 phases, 7 plans, 15 tasks

**Key accomplishments:**

- Azure OpenAI client with dual TPM/RPM rate limiter, AzureOpenAIConfig in Config struct, and pipeline_runs/pipeline_checkpoints schema with Store CRUD methods
- Generic batch processing framework with checkpoint resumability, live progress display (count/cost/tok-per-sec/ETA), graceful SIGINT shutdown, and a hidden 'pipeline test' CLI command for infrastructure validation
- One-liner:
- One-liner:
- One-liner:
- One-liner:

---

## v1.1 Web UI Rebuild (Templ + HTMX) (Shipped: 2026-03-11)

**Phases:** 6 (Phases 6-11) | **Plans:** 13 | **Tasks:** 27
**Timeline:** 42 days (2026-01-28 → 2026-03-11)
**Files changed:** 135 | **Lines:** +18,958 / -11,339
**Web package:** 8,654 LOC (Go + Templ)
**Requirements:** 25/25 satisfied | **Audit:** Passed

**Key accomplishments:**

- Replaced React SPA with server-rendered Templ + HTMX — single `go build` binary, no npm/Node.js
- HTML email rendering with bluemonday sanitization, sandboxed iframes, CID image substitution, external image blocking with opt-in toggle
- Thread/conversation view with chronological messages, collapsible via `<details>`, lazy-load bodies, n/p/t keyboard navigation
- Text/HTML body toggle per message with URL persistence, CSS-only time-series bar chart on dashboard
- Full keyboard navigation: j/k row nav, s/r sort cycling, Enter/Esc/Tab/a shortcuts across all pages
- Loading indicators on all 42 HTMX trigger points across 10 templates
- Deleted React SPA (`web/`), JSON API (`internal/api/`), and all npm/Vite artifacts

**Archive:** `milestones/v1.1-ROADMAP.md`, `milestones/v1.1-REQUIREMENTS.md`, `milestones/v1.1-MILESTONE-AUDIT.md`

---

## v1.0 — Core Archive & Search

**Status:** Complete
**Phases:** 1-5 (inferred from shipped features)

**What shipped:**

- Full/incremental Gmail sync with OAuth (browser + headless)
- MIME parsing (subject, body, attachments, charset)
- Parquet ETL pipeline (DuckDB-based, incremental)
- DuckDB query engine over Parquet for analytics
- Full-featured TUI (drill-down, search, selection, 7 views)
- UTF-8 encoding repair
- Deletion staging and execution via Gmail API
- `--list` flag for delete-staged command
- Web UI: React 19 + TypeScript SPA (Dashboard, Messages, Aggregate, Search, Deletions, Thread, Message detail)
- JSON API server (`internal/api/`) with auth and rate limiting

**Validated:** Users can sync Gmail, search/browse archives in TUI or Web UI, and stage/execute deletions.

---
*Last updated: 2026-03-10 — bootstrapped from shipped codebase*
