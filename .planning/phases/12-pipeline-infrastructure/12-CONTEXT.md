# Phase 12: Pipeline Infrastructure - Context

**Gathered:** 2026-04-11
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — discuss skipped)

<domain>
## Phase Boundary

Users can run resumable Azure OpenAI batch jobs against their archive with live progress and cost tracking. This phase builds the foundation: config, Azure OpenAI client, batch processing framework with checkpoints, rate limiting, and progress display.

Requirements: PIPE-01, PIPE-02, PIPE-03, PIPE-04

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

Key constraints from conversation:
- Azure OpenAI text-embedding-3-small for embeddings ($0.02/M tokens)
- Azure OpenAI GPT-4o-mini for enrichment ($0.15/M input, $0.60/M output)
- Config should extend existing config.toml TOML structure
- API key should NOT be in config.toml — use env var or file reference
- Checkpoint pattern should follow existing sync_checkpoints table approach
- Rate limiting must be configurable (Azure TPM/RPM quotas vary by tier)
- Pipeline must handle 472K messages at Mac Mini scale
- Budget: ~$200 total across all phases

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/config/config.go` — existing TOML config loader with Microsoft section
- `internal/sync/sync.go` — sync orchestration with progress display pattern
- `internal/store/schema.sql` — sync_checkpoints table for resumability pattern
- `internal/gmail/client.go` — rate limiting implementation reference

### Established Patterns
- Cobra CLI commands in `cmd/msgvault/cmd/`
- Store struct for all DB operations
- Context-based cancellation for long operations
- Progress display with rate/count/ETA in sync commands

### Integration Points
- New `[azure_openai]` config section in config.toml
- New `internal/ai/` package for Azure OpenAI client and batch framework
- New checkpoint table for AI pipeline state
- New CLI commands: `msgvault embed`, `msgvault enrich`

</code_context>

<specifics>
## Specific Ideas

No specific requirements — infrastructure phase. Refer to ROADMAP phase description and success criteria.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
