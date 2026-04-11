# Roadmap: msgvault

## Milestones

- ✅ **v1.0 Core Archive & Search** - Phases 1-5 (shipped 2026-03-10)
- ✅ **v1.1 Web UI Rebuild (Templ + HTMX)** - Phases 6-11 (shipped 2026-03-11)
- 🚧 **v1.2 AI Archive Intelligence** - Phases 12-14 (in progress)

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

### 🚧 v1.2 AI Archive Intelligence (In Progress)

**Milestone Goal:** Add semantic search and AI-powered enrichment to the 472K-message archive using $200 Azure credits before they expire 2026-05-11.

- [ ] **Phase 12: Pipeline Infrastructure** - Azure OpenAI client, config, rate limiting, checkpointed batch runner
- [ ] **Phase 13: Embeddings & Vector Search** - Embed all messages, sqlite-vec storage, CLI + web semantic search, hybrid re-ranking
- [ ] **Phase 14: AI Enrichment & UI Integration** - Categorization, life events, entity extraction, label storage, TUI/web filters

## Phase Details

### Phase 12: Pipeline Infrastructure
**Goal**: Users can run resumable Azure OpenAI batch jobs against their archive with live progress and cost tracking
**Depends on**: Phase 11 (existing codebase)
**Requirements**: PIPE-01, PIPE-02, PIPE-03, PIPE-04
**Success Criteria** (what must be TRUE):
  1. `config.toml` accepts Azure OpenAI endpoint, API key reference, and deployment names without breaking existing config
  2. A batch job interrupted mid-run resumes from its last checkpoint rather than restarting from zero
  3. Running any AI pipeline command prints live message count, cost estimate, tokens/sec rate, and ETA
  4. The pipeline pauses automatically when TPM/RPM quota limits are approached and resumes without error
**Plans**: TBD

### Phase 13: Embeddings & Vector Search
**Goal**: Users can find semantically related emails using natural language queries, beyond exact keyword matching
**Depends on**: Phase 12
**Requirements**: EMBED-01, EMBED-02, EMBED-03, EMBED-04, EMBED-05
**Success Criteria** (what must be TRUE):
  1. All 472K messages are embedded via Azure OpenAI text-embedding-3-small and stored in a sqlite-vec table keyed by message ID
  2. `msgvault search --semantic "query"` returns results ranked by vector similarity from the CLI
  3. The web UI search page accepts a semantic query and displays results ranked by similarity score
  4. Hybrid search combines FTS5 keyword matches with vector similarity and re-ranks results — a query like "flight to Japan" finds both exact matches and semantically related messages
**UI hint**: yes
**Plans**: TBD

### Phase 14: AI Enrichment & UI Integration
**Goal**: Users can browse their archive by AI-generated categories and extract a structured life timeline
**Depends on**: Phase 12
**Requirements**: ENRICH-01, ENRICH-02, ENRICH-03, ENRICH-04, ENRICH-05, ENRICH-06, ENRICH-07
**Success Criteria** (what must be TRUE):
  1. All messages are categorized into finance, travel, legal, health, shopping, newsletters, personal, or work — categories appear as AI-generated labels in the existing label system
  2. TUI and web UI both expose an AI category filter so users can view only messages in a given category
  3. Life events (jobs, moves, purchases, travel, milestones) are extracted and exportable as a LifeVault-compatible JSON file with date, type, description, and source_message_id
  4. Entities (people, companies, dates, amounts) are stored in a searchable table with back-references to source messages
**UI hint**: yes
**Plans**: TBD

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 1-5 | v1.0 | - | Complete | 2026-03-10 |
| 6-11 | v1.1 | 13/13 | Complete | 2026-03-11 |
| 12. Pipeline Infrastructure | v1.2 | 0/TBD | Not started | - |
| 13. Embeddings & Vector Search | v1.2 | 0/TBD | Not started | - |
| 14. AI Enrichment & UI Integration | v1.2 | 0/TBD | Not started | - |
