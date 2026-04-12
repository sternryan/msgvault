---
phase: 14
slug: ai-enrichment-ui-integration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-11
---

# Phase 14 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test infrastructure |
| **Quick run command** | `go test ./internal/ai/... ./internal/store/... -count=1 -short` |
| **Full suite command** | `make test` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/ai/... ./internal/store/... -count=1 -short`
- **After every plan wave:** Run `make test`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 14-01-01 | 01 | 1 | ENRICH-01 | — | Category whitelist enforced | unit | `go test ./internal/ai/... -run TestEnrich` | ❌ W0 | ⬜ pending |
| 14-01-02 | 01 | 1 | ENRICH-02 | — | Auto labels stored with type='auto' | unit | `go test ./internal/store/... -run TestAutoLabel` | ❌ W0 | ⬜ pending |
| 14-01-03 | 01 | 1 | ENRICH-04 | — | Life events extracted with required fields | unit | `go test ./internal/ai/... -run TestLifeEvent` | ❌ W0 | ⬜ pending |
| 14-01-04 | 01 | 1 | ENRICH-06 | — | Entities stored with back-references | unit | `go test ./internal/store/... -run TestEntity` | ❌ W0 | ⬜ pending |
| 14-02-01 | 02 | 2 | ENRICH-03 | — | Category filter in web UI | integration | `go test ./internal/web/... -run TestCategoryFilter` | ❌ W0 | ⬜ pending |
| 14-02-02 | 02 | 2 | ENRICH-05 | — | Timeline export produces valid JSON | unit | `go test ./cmd/... -run TestExportTimeline` | ❌ W0 | ⬜ pending |
| 14-02-03 | 02 | 2 | ENRICH-07 | — | Entities page renders with type filter | integration | `go test ./internal/web/... -run TestEntitiesPage` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Test stubs for enrichment pipeline processing
- [ ] Test stubs for auto-label storage operations
- [ ] Test stubs for entity and life event storage

*Existing go test infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| TUI AI Categories view navigation | ENRICH-03 | Bubble Tea TUI requires interactive terminal | Launch TUI, press Tab until AI Categories view appears, verify drill-down works |
| Category dropdown populates in web UI | ENRICH-03 | Visual verification of rendered HTML | Load messages page, verify dropdown shows 8 AI categories |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
