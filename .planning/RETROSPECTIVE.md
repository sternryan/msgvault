# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.1 — Web UI Rebuild (Templ + HTMX)

**Shipped:** 2026-03-11
**Phases:** 6 | **Plans:** 13 | **Timeline:** 42 days

### What Was Built
- Complete Templ + HTMX web UI replacing React SPA (8,654 LOC web package)
- HTML email rendering pipeline (bluemonday sanitization, sandboxed iframes, CID substitution)
- Thread/conversation view with collapsible messages and lazy-load
- Text/HTML toggle, CSS bar chart, loading indicators, full keyboard navigation

### What Worked
- Phase 6 (foundation) was well-scoped — 5 plans covered full SPA parity in one phase
- TDD approach caught selector mismatches early in Phase 11
- Integration checker after Phase 9 caught DOM selector and test assertion drift before they became user-facing bugs
- Gap closure phases (10, 11) were fast — narrow scope, clear targets from audit
- Parallel research + plan verification loop produced executable plans with minimal rework

### What Was Inefficient
- Phase 6 VERIFICATION had human_needed status that was never formally cleared — browser testing items accumulated across 4 phases (22 items total)
- Selector mismatches (INT-03, INT-04) between keys.js and templates were introduced in Phase 6 but not caught until milestone audit — earlier integration testing would have caught them
- Some SUMMARY.md files had inconsistent frontmatter format (no one_liner field) — extraction tools returned null

### Patterns Established
- `hx-target="closest .email-render-wrapper"` pattern for context-safe HTMX swaps (avoids ID collisions in thread view)
- `wrapperID` query parameter pattern for unique DOM IDs in repeated HTMX fragments
- Universal `#page-indicator` outside `#main-content` for persistent loading state across HTMX swaps
- `templ.KV("active", condition)` for conditional CSS classes in Templ components
- `sanitize → substitute CIDs → block external images` pipeline order (bluemonday strips cid: scheme)

### Key Lessons
1. **DOM selector alignment needs cross-cutting verification** — selectors in JS must be checked against template output, not just JS unit tests. Phase 11 was entirely avoidable with a selector-matching test in Phase 6.
2. **Gap closure phases should be budgeted** — Phases 10 and 11 were reactive fixes. A "integration sweep" task at end of Phase 9 would have caught all 5 INT issues in one pass.
3. **HTMX outerHTML swap is the right pattern for toggles** — avoids JS iframe manipulation, keeps server in control of rendering decisions.

### Cost Observations
- Model mix: Opus orchestration, Sonnet execution and verification
- 13 plans executed across ~6 executor agent spawns
- Gap closure (Phases 10-11) added 2 extra phases but were fast (1 plan each, <5 min execution)

---

## Cross-Milestone Trends

| Metric | v1.0 | v1.1 |
|--------|------|------|
| Phases | 5 (inferred) | 6 |
| Plans | — | 13 |
| Requirements | — | 25/25 |
| Gap closure phases | — | 2 |
| Timeline | — | 42 days |

**Recurring patterns:**
- Integration issues surface at milestone audit, not during phase execution — consider adding integration checks between phases
- Human verification items accumulate — consider a "browser testing session" checkpoint mid-milestone
