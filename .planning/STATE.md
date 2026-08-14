---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: AI Archive Intelligence
status: shipped
stopped_at: "v1.2 shipped 2026-04-12 (tag v1.2, milestone audit PASSED 16/16 requirements); untracked work continued after — see below"
last_updated: "2026-08-14T00:00:00.000Z"
last_activity: 2026-08-07
progress:
  total_phases: 3
  completed_phases: 3
  total_plans: 7
  completed_plans: 7
  percent: 100
---

# Project State — reconciled 2026-08-14

**This file previously claimed `status: planning` / 0% for v1.2 — that was self-contradictory**
(v1.2's own phases were separately marked complete, and `.planning/MILESTONES.md` + the git tag
`v1.2` + `.planning/milestones/v1.2-MILESTONE-AUDIT.md` all confirm v1.2 shipped 2026-04-12,
PASSED, 16/16 requirements). Corrected below against verified git/tag/audit reality.

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-11)

**Core value:** Users can safely archive their entire Gmail history offline and search it instantly, with confidence that nothing is lost before deletion.
**v1.2 (AI Archive Intelligence) shipped 2026-04-12** — Azure OpenAI pipeline infra, embeddings/semantic+hybrid search (sqlite-vec), and AI enrichment (categorization, life timeline, entity extraction). Tag `v1.2`.

## Current Position (post-v1.2, not tracked as a formal GSD milestone in ROADMAP.md)

Git history shows substantial work after v1.2 with no corresponding milestone entry in
`.planning/ROADMAP.md`:

- Phase "16" (authority graph): `authority.Store`/SQLite impl, daily recompute cobra command +
  launchd plist, retired the old `ExpertAllowlist`/trustedcontacts package, perf-hardened the
  recompute (N+1 collapse, window-match reply detection, 100k-message regression guard) —
  commits `fe3ff290`..`060212b4`, 2026-04-29 to 2026-05-03.
- Triage pipeline (reused "14-0x" plan numbering, distinct from the v1.2 Phase 14): gmail send +
  digest formatter, sources.db integration with the `forge` repo's graph.db — commits
  `24836dfe`..`f21272f9`.
- `docs: vector subsystem evaluation and recommendation` (2026-07-01, `9c85f8e8`).
- `refactor(query): split 2,510-line duckdb.go into themed files` (2026-07-05, `a94ccb55`).
- `feat(imap): persist a per-account sync floor via --since` (2026-08-01, `20216065`) — most
  recent feature commit.
- Latest commit overall: `ed29002d` — 2026-08-07 — "chore: ignore bin/ (66MB compiled binary)".

None of this is organized under a v1.3 (or later) entry in ROADMAP.md — treat it as verified-via-git
untracked follow-on work, not as a planned/gated milestone.

```
Progress: v1.2 [██████████████████████] 100% (3/3 phases, shipped 2026-04-12)
```

## Accumulated Context

- Archive: 472K messages, 6 accounts, 21GB on Mac Mini SSOT
- Azure credits: $200 expiring ~2026-05-11 (hard deadline)
- Budget math: text-embedding-3-small ~$0.02/M tokens, GPT-4o-mini ~$0.15/M input tokens
- sqlite-vec Go bindings exist; vector table lives alongside message IDs in SQLite
- Existing label system (labels + message_labels tables) is the storage target for AI categories
- OCR (Tesseract) skipped — only 6 audio attachments, not worth integration effort
- Relationship graph deferred to v1.3
- vaulttrain-stern DPO pipeline needs dpo_formatter.py fix (separate project, not blocking)
- Phase 13 and 14 both depend on Phase 12 (pipeline infra) but are otherwise independent — could parallelize

## Architecture Notes

- Pipeline checkpoints: should use same pattern as existing sync_checkpoints table
- Rate limiting: Azure TPM/RPM quotas vary by tier — must be configurable, not hardcoded
- LifeVault export format: JSON with date, type, description, source_message_id
- Entity table needs back-references to messages for drill-down in web UI

## Session Continuity

Last session: 2026-04-11
Stopped at: Roadmap created — run /gsd-plan-phase 12 to begin
Resume file: None

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260329-iwx | Fix roborev review bugs: timezone date filters, nil batch entries, missing attachment warning | 2026-03-29 | 16a22895 | [260329-iwx-fix-roborev-review-bugs-timezone-date-fi](./quick/260329-iwx-fix-roborev-review-bugs-timezone-date-fi/) |
