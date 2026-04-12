# Phase 14: AI Enrichment & UI Integration - Context

**Gathered:** 2026-04-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can browse their archive by AI-generated categories and extract a structured life timeline. This phase uses the Phase 12 pipeline infrastructure and Azure OpenAI GPT-4o-mini to auto-categorize all messages, extract life events and entities, and expose these through the existing TUI and web UI.

Requirements: ENRICH-01, ENRICH-02, ENRICH-03, ENRICH-04, ENRICH-05, ENRICH-06, ENRICH-07

</domain>

<decisions>
## Implementation Decisions

### AI Categorization Strategy
- Single LLM call per message with subject+snippet — cheapest approach, subject carries most search intent (consistent with Phase 13 embedding strategy)
- Exactly one primary category per message — simpler UX, cleaner label filter, matches Gmail label mental model
- AI categories stored as labels with label_type='auto' in existing labels table — reuses all existing label UI/filtering/aggregation code in both TUI and web
- Fixed 8 categories: finance, travel, legal, health, shopping, newsletters, personal, work (per ROADMAP success criteria)

### Life Events & Entity Extraction
- Combined single LLM call returns category + events + entities — halves API cost vs separate pipeline runs ($0.15/M input tokens shared across all three extraction tasks)
- Life events extracted as structured JSON with date/type/description/source_message_id per LifeVault format spec
- New `entities` table with columns: id, message_id (FK), entity_type (person/company/date/amount), value, normalized_value, context — separate from labels
- CLI command `msgvault export-timeline` producing JSON file with `[{date, type, description, source_message_id}]` per ENRICH-05

### UI Integration
- Web UI: Dropdown filter on messages page — same pattern as existing account filter, populated from labels where label_type='auto'
- TUI: Extend existing Tab-cycle views — add "AI Categories" as a new view alongside Senders/Labels/Time, reuses drill-down pattern
- Web UI: New "Entities" page linked from nav — searchable table with type filter, click entity to see source messages
- Life timeline: Export-only (CLI) for v1.2 — LifeVault consumes the JSON. Web timeline visualization deferred to v1.3

### Claude's Discretion
- LLM prompt engineering for categorization accuracy and structured output parsing
- Entity normalization strategy (deduplication of "Google" vs "Google LLC" vs "Google Inc")
- Error handling for malformed LLM responses (retry, skip, fallback)
- Batch size tuning for combined extraction call (may differ from embedding batch size)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/ai/client.go` — Client.ChatCompletion() for GPT-4o-mini calls
- `internal/ai/pipeline.go` — BatchRunner with checkpoints, ProcessFunc interface
- `internal/ai/progress.go` — ProgressReporter with rate/cost/ETA display
- `internal/ai/ratelimit.go` — RateLimiter respecting Azure TPM/RPM quotas
- `internal/store/schema.sql` — labels table with label_type='auto' support, message_labels junction
- `internal/store/api.go` — batchGetLabels() for efficient label loading
- `internal/web/params.go` — existing label filter parameter parsing
- `internal/web/templates/aggregate.templ` — labels view in aggregate page
- `internal/tui/` — Tab-cycle views with drill-down pattern

### Established Patterns
- Templ + HTMX for all web UI — server-rendered partials with hx-swap
- Cobra CLI commands in cmd/msgvault/cmd/
- Store struct for all DB operations
- Pipeline type string in BatchRunner (e.g. "embedding", "categorize")
- Label display: .label-tag spans in templates, label filter via query params

### Integration Points
- New `msgvault enrich` CLI command (uses BatchRunner from Phase 12)
- New `msgvault export-timeline` CLI command
- New `entities` table + `life_events` table in schema.sql
- Web UI: category dropdown on messages page, new Entities nav item + page
- TUI: new AI Categories view type in Tab cycle

</code_context>

<specifics>
## Specific Ideas

- Use Phase 12 BatchRunner directly — ProcessFunc calls Client.ChatCompletion() with structured JSON output
- Combined prompt: "Given this email subject and snippet, return JSON: {category: string, life_events: [{date, type, description}], entities: [{type, value}]}"
- Reuse label_type='auto' with source_id=NULL to distinguish from Gmail-synced labels
- Entities page: `/entities?type=person&q=search` with HTMX partial loading
- Export timeline: simple JSON array write, no streaming needed for reasonable dataset sizes

</specifics>

<deferred>
## Deferred Ideas

- Web timeline visualization (v1.3 — after LifeVault integration proves the export format)
- Entity deduplication across accounts (v1.3 REL-02)
- Relationship graph from entity co-occurrence (v1.3 REL-01)

</deferred>
