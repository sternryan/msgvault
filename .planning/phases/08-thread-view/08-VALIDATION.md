---
phase: 8
slug: thread-view
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-11
---

# Phase 8 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) + httptest |
| **Config file** | none — `go test ./...` |
| **Quick run command** | `go test ./internal/web/... -run TestThread -v` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/... -run TestThread -v`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 08-01-01 | 01 | 1 | THREAD-01 | integration | `go test ./internal/web/... -run TestThreadView` | ❌ W0 | ⬜ pending |
| 08-01-02 | 01 | 1 | THREAD-02 | integration | `go test ./internal/web/... -run TestThreadMessageCollapsible` | ❌ W0 | ⬜ pending |
| 08-01-03 | 01 | 1 | THREAD-03 | integration | `go test ./internal/web/... -run TestThreadLazyLoad` | ❌ W0 | ⬜ pending |
| 08-02-01 | 02 | 1 | THREAD-04 | integration | `go test ./internal/web/... -run TestMessageDetailViewThreadLink` | ❌ W0 | ⬜ pending |
| 08-02-02 | 02 | 1 | THREAD-05 | integration | `go test ./internal/web/... -run TestThreadNavAttributes` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/web/handlers_thread_test.go` — thread handler tests (THREAD-01 through THREAD-05)
- [ ] Mock engine in `handlers_test.go` needs `ListMessages` to support `ConversationID` filter response

*Existing test infrastructure in `handlers_test.go` + `setupTestServer` covers the framework; only thread-specific tests are new.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `n`/`p` keyboard shortcuts scroll/expand messages | THREAD-05 | JS behavior requires browser | Press n/p in browser thread view, verify focus moves and message expands |
| iframe auto-resize in thread context | THREAD-03 | PostMessage cross-frame behavior | Open thread with HTML messages, verify iframes resize without scrollbars |
| `t` shortcut on message detail navigates to thread | THREAD-04 | JS keyboard handler | Press `t` on message detail page, verify navigation to /threads/{id} |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
