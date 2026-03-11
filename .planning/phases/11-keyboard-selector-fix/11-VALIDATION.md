---
phase: 11
slug: keyboard-selector-fix
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-11
---

# Phase 11 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) + httptest |
| **Config file** | none — standard `go test` |
| **Quick run command** | `go test ./internal/web/... -run TestHandlersReturnHTML` |
| **Full suite command** | `go test ./internal/web/...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/... -run TestHandlersReturnHTML -v`
- **After every plan wave:** Run `go test ./internal/web/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 11-01-01 | 01 | 1 | PARITY-07 | unit | `go test ./internal/web/... -run TestSortHeaderEmitsSortField` | ❌ W0 | ⬜ pending |
| 11-01-02 | 01 | 1 | PARITY-07 | unit | `go test ./internal/web/... -run TestMessages` | ✅ (extend) | ⬜ pending |
| 11-01-03 | 01 | 1 | PARITY-07 | manual | n/a — j/k/Enter/s/r requires browser | N/A | ⬜ pending |
| 11-01-04 | 01 | 1 | POLISH-03 | manual | inspect 09-02-SUMMARY.md | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/web/handlers_test.go` — add `TestSortHeaderEmitsSortField`: GET `/messages`, parse body, assert `data-sort-field` attribute present on `<th class="sortable-header">` elements

*Existing tests already verify handler 200 responses; Wave 0 adds targeted attribute assertions.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| j/k moves row highlight, Enter navigates | PARITY-07 | DOM interaction requires browser | Open messages page, press j/k, verify row highlight moves, press Enter to open |
| s cycles sort field, r reverses direction | PARITY-07 | DOM interaction requires browser | Open messages page, press s, verify sort field changes, press r to reverse |
| Loading indicators visible | POLISH-03 | Visual UX verification | Trigger HTMX update, verify indicator appears |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
