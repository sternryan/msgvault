# Milestones: msgvault

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
