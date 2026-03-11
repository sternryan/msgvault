# Roadmap: msgvault

## Milestones

- ✅ **v1.0 Core Archive & Search** - Phases 1-5 (shipped 2026-03-10)
- ✅ **v1.1 Web UI Rebuild (Templ + HTMX)** - Phases 6-11 (shipped 2026-03-11)

## Phases

<details>
<summary>✅ v1.0 Core Archive & Search (Phases 1-5) - SHIPPED 2026-03-10</summary>

Phases 1-5 delivered the complete offline Gmail archiver: full/incremental sync, MIME parsing, Parquet ETL, DuckDB query engine, full-featured TUI, UTF-8 repair, deletion execution, React SPA Web UI, and JSON API server.

</details>

<details>
<summary>✅ v1.1 Web UI Rebuild (Phases 6-11) - SHIPPED 2026-03-11</summary>

Replaced React SPA with server-rendered Templ + HTMX single-binary web UI. Added HTML email rendering (sanitization, sandboxed iframes, CID substitution), thread view with collapsible messages, text/HTML toggle, CSS bar chart, keyboard navigation, and loading indicators. 6 phases, 13 plans, 25 requirements satisfied.

See: `milestones/v1.1-ROADMAP.md` for full phase details.

</details>

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 1-5 | v1.0 | - | Complete | 2026-03-10 |
| 6-11 | v1.1 | 13/13 | Complete | 2026-03-11 |
