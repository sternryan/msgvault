---
phase: 09
slug: polish
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-11
---

# Phase 09 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` package + `net/http/httptest` |
| **Config file** | none — standard `go test ./...` |
| **Quick run command** | `go test ./internal/web/... -timeout 30s` |
| **Full suite command** | `go test ./... -timeout 120s` |
| **Estimated runtime** | ~30 seconds (web), ~120 seconds (full) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/... -timeout 30s`
- **After every plan wave:** Run `go test ./... -timeout 120s`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 09-01-01 | 01 | 1 | POLISH-01 | unit | `go test ./internal/web/... -run TestMessageBodyWrapper -timeout 30s` | ❌ W0 | ⬜ pending |
| 09-01-02 | 01 | 1 | POLISH-01 | unit | `go test ./internal/web/... -run TestMessageDetail -timeout 30s` | ❌ W0 | ⬜ pending |
| 09-02-01 | 02 | 1 | POLISH-02 | unit | `go test ./internal/web/... -run TestDashboard -timeout 30s` | ❌ W0 | ⬜ pending |
| 09-02-02 | 02 | 1 | POLISH-02 | unit | `go test ./internal/web/... -run TestBarChart -timeout 30s` | ❌ W0 | ⬜ pending |
| 09-03-01 | 03 | 1 | POLISH-03 | unit | `go test ./internal/web/... -run TestIndicators -timeout 30s` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Extend `internal/web/handlers_test.go` — add test cases for body-wrapper format param (`?format=text` returns `<pre>`, `?format=html` returns iframe)
- [ ] Extend `internal/web/handlers_test.go` — add test cases for dashboard handler returning chart data
- [ ] No new test files needed — existing test infrastructure covers all requirements

*Existing infrastructure covers framework setup; only new test cases needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Text/HTML toggle visual appearance | POLISH-01 | CSS styling, button active state | Open message with both formats, verify toggle renders, click Text, verify `<pre>` block shows plain text |
| Bar chart visual proportions | POLISH-02 | CSS width percentages, Solarized colors | Open dashboard, verify bars are proportional to counts, click a bar to drill down |
| Loading indicator visibility timing | POLISH-03 | Requires slow network to observe | Open Messages, click pagination, verify "Loading..." appears during request |
| Toggle hidden for single-format messages | POLISH-01 | Template conditional rendering | Open text-only message, verify no toggle appears |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
