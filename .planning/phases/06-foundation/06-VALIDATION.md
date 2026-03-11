---
phase: 6
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-10
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib (`testing` package) + `go test` |
| **Config file** | None — existing `go test -tags fts5 ./...` runs all tests |
| **Quick run command** | `go test -tags fts5 ./internal/web/... -run TestHandlers` |
| **Full suite command** | `go test -tags fts5 ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go build -tags fts5 ./cmd/msgvault`
- **After every plan wave:** Run `go test -tags fts5 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | FOUND-01 | smoke | `go build -tags fts5 ./cmd/msgvault` | ✅ | ⬜ pending |
| 06-01-02 | 01 | 1 | FOUND-02 | integration | `go test -tags fts5 ./internal/web/... -run TestHandlersReturnHTML` | ❌ W0 | ⬜ pending |
| 06-01-03 | 01 | 1 | FOUND-04 | integration | `go test -tags fts5 ./internal/web/... -run TestStaticFiles` | ❌ W0 | ⬜ pending |
| 06-01-04 | 01 | 1 | FOUND-05 | smoke | `go build -tags fts5 ./... && ls internal/web/templates/*_templ.go` | ❌ W0 | ⬜ pending |
| 06-02-01 | 02 | 2 | PARITY-01 | integration | `go test -tags fts5 ./internal/web/... -run TestDashboard` | ❌ W0 | ⬜ pending |
| 06-02-02 | 02 | 2 | PARITY-02 | integration | `go test -tags fts5 ./internal/web/... -run TestAggregate` | ❌ W0 | ⬜ pending |
| 06-02-03 | 02 | 2 | PARITY-03 | integration | `go test -tags fts5 ./internal/web/... -run TestMessages` | ❌ W0 | ⬜ pending |
| 06-02-04 | 02 | 2 | PARITY-04 | integration | `go test -tags fts5 ./internal/web/... -run TestSearch` | ❌ W0 | ⬜ pending |
| 06-02-05 | 02 | 2 | PARITY-05 | integration | `go test -tags fts5 ./internal/web/... -run TestMessageDetail` | ❌ W0 | ⬜ pending |
| 06-02-06 | 02 | 2 | PARITY-06 | integration | `go test -tags fts5 ./internal/web/... -run TestStageDeletion` | ❌ W0 | ⬜ pending |
| 06-02-07 | 02 | 2 | PARITY-07 | manual | Browser keyboard test | — | ⬜ pending |
| 06-02-08 | 02 | 2 | PARITY-08 | integration | `go test -tags fts5 ./internal/web/... -run TestAccountFilter` | ❌ W0 | ⬜ pending |
| 06-03-01 | 03 | 3 | FOUND-03 | smoke | `test ! -d web && test ! -d internal/api` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/web/handlers_test.go` — test stubs for FOUND-02, FOUND-04, PARITY-01 through PARITY-06, PARITY-08
- [ ] Test helper: `httptest.NewServer` with mock `query.Engine` (interface already defined)
- [ ] No new framework install needed — Go stdlib `testing` + `net/http/httptest` is sufficient

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Keyboard shortcuts (j/k/Enter/Esc/Tab/s/r/t/a) fire correctly | PARITY-07 | Browser keyboard interaction requires JS event listeners; not automatable without Playwright | 1. Open any page 2. Press `j`/`k` to navigate rows 3. Press `Enter` to drill-down 4. Press `Esc` to go back 5. Press `?` for help overlay |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
