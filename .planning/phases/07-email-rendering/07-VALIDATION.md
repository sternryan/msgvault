---
phase: 7
slug: email-rendering
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-10
---

# Phase 7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` |
| **Config file** | none (go test ./...) |
| **Quick run command** | `go test ./internal/web/... -run TestSanitize -v` |
| **Full suite command** | `go test ./... 2>&1 \| tail -20` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/... -run TestSanitize -v`
- **After every plan wave:** Run `go test ./... 2>&1 | tail -20`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 07-01-01 | 01 | 1 | RENDER-01 | unit | `go test ./internal/web/... -run TestSanitizeEmailHTML_StripScript -v` | ❌ W0 | ⬜ pending |
| 07-01-02 | 01 | 1 | RENDER-01 | unit | `go test ./internal/web/... -run TestSanitizeEmailHTML_StripOnclick -v` | ❌ W0 | ⬜ pending |
| 07-01-03 | 01 | 1 | RENDER-01 | unit | `go test ./internal/web/... -run TestSanitizeEmailHTML_PreserveTable -v` | ❌ W0 | ⬜ pending |
| 07-02-01 | 02 | 1 | RENDER-02 | integration | `go test ./internal/web/... -run TestMessageBodyEndpoint -v` | ❌ W0 | ⬜ pending |
| 07-02-02 | 02 | 1 | RENDER-02 | integration | `go test ./internal/web/... -run TestMessageBodyEndpointStandalone -v` | ❌ W0 | ⬜ pending |
| 07-03-01 | 03 | 1 | RENDER-03 | unit | `go test ./internal/web/... -run TestSubstituteCIDImages -v` | ❌ W0 | ⬜ pending |
| 07-04-01 | 04 | 1 | RENDER-04 | unit | `go test ./internal/web/... -run TestBlockExternalImages -v` | ❌ W0 | ⬜ pending |
| 07-04-02 | 04 | 1 | RENDER-04 | unit | `go test ./internal/web/... -run TestBlockExternalImages_ShowImages -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/web/sanitize_email_test.go` — stubs for RENDER-01 (sanitizer), RENDER-03 (CID sub), RENDER-04 (image blocking)
- [ ] `internal/web/handlers_test.go` additions — TestMessageBodyEndpoint, TestMessageBodyEndpointStandalone covering RENDER-02

*Existing `go test` + httptest pattern already established in handlers_test.go*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Iframe renders without breaking Solarized layout | RENDER-02 | Visual rendering check | Open message detail with HTML body, verify iframe displays with 1px border, email CSS doesn't leak to parent |
| Iframe auto-resizes to content height | RENDER-02 | Browser-specific postMessage behavior | Open long HTML email, verify no internal scrollbar, page scrolls naturally |
| "Load images" banner toggles external images | RENDER-04 | HTMX interaction requires browser | Click "Load images", verify external images appear without page reload |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
