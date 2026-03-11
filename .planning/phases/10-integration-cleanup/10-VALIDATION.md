---
phase: 10
slug: integration-cleanup
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-11
---

# Phase 10 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go testing |
| **Quick run command** | `go test ./internal/web/...` |
| **Full suite command** | `go test ./internal/web/... -v` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/...`
- **After every plan wave:** Run `go test ./internal/web/... -v`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 10-01-01 | 01 | 1 | RENDER-04, POLISH-01 | integration | `go test ./internal/web/... -run TestMessageBodyWrapperEndpoint -v` | ✅ | ⬜ pending |
| 10-01-02 | 01 | 1 | THREAD-02, THREAD-03 | integration | `go test ./internal/web/... -run TestBodyWrapperWithWrapperIDParam -v` | ❌ W0 | ⬜ pending |
| 10-01-03 | 01 | 1 | — | regression | `go test ./internal/web/... -v` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `TestBodyWrapperWithWrapperIDParam` test stub — validates unique wrapper IDs from `?wrapperID=` param

*Existing `TestMessageBodyWrapperEndpoint` covers INT-01 once assertions are updated.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Thread view messages show unique IDs in DOM | THREAD-02 | Browser DOM inspection | Open thread with 3+ messages, expand all, verify no duplicate `id` attributes in Elements panel |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
