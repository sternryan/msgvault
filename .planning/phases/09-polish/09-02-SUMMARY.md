---
phase: 09-polish
plan: 02
subsystem: web-ui
tags: [htmx, loading-indicators, ux, templates]
dependency_graph:
  requires: [09-01]
  provides: [POLISH-03]
  affects: [all web templates]
tech_stack:
  added: []
  patterns:
    - "Page-level indicator span outside #main-content for universal outerHTML swap coverage"
    - "Specialized inline indicator spans (#filter-indicator, #stage-indicator) for local-target swaps"
    - "Universal #page-indicator approach (no per-element logic complexity)"
key_files:
  created: []
  modified:
    - internal/web/templates/layout.templ
    - internal/web/templates/components.templ
    - internal/web/static/style.css
    - internal/web/templates/messages.templ
    - internal/web/templates/message.templ
    - internal/web/templates/aggregate.templ
    - internal/web/templates/dashboard.templ
    - internal/web/templates/deletions.templ
    - internal/web/templates/thread.templ
    - internal/web/templates/search.templ
decisions:
  - "Universal #page-indicator for all #main-content swaps rather than per-trigger indicators — simpler, minimal, consistent"
  - "Specialized #filter-indicator (Filtering...) and #stage-indicator (Staging...) for aggregate filter input and staging form — better contextual UX for those two operations"
  - "Kept existing #search-indicator (Searching...) in search.templ — already had best-in-class UX"
  - ".page-indicator-bar uses height:0 + overflow:visible so indicator consumes no layout space"
metrics:
  duration: "5min"
  completed_date: "2026-03-11"
  tasks_completed: 2
  files_modified: 10
requirements_completed: [POLISH-03]
---

# Phase 09 Plan 02: Loading Indicators Summary

**One-liner:** Added hx-indicator attributes to all 37 HTMX trigger points across 10 files using a persistent #page-indicator span outside #main-content.

## What Was Built

Loading indicators for every HTMX asynchronous operation in the web UI. When a user clicks any link, row, tab, or button that triggers an HTMX request, "Loading..." text appears immediately (in yellow, matching the Solarized Dark theme) and disappears when the response arrives.

### Architecture

A single persistent `<span id="page-indicator" class="htmx-indicator">Loading...</span>` was placed between the `<nav>` and `<main id="main-content">` in layout.templ. Because it lives outside `#main-content`, it survives `hx-swap="outerHTML"` swaps and can be referenced by any trigger that replaces the main content region.

Two specialized inline indicators were also added:
- `#filter-indicator` ("Filtering...") — next to the aggregate filter input for contextual feedback
- `#stage-indicator` ("Staging...") — in the staging bar for the "Stage for Deletion" form

The existing `#search-indicator` ("Searching...") in search.templ was preserved as-is.

### CSS

A `.page-indicator-bar` div with `height: 0` and `overflow: visible` was added to position the indicator without consuming layout space.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Page-level indicator in layout + Pagination/SortHeader | 5ace4266 | layout.templ, components.templ, style.css |
| 2 | Indicators on all remaining HTMX trigger points | 54e9ec84 | messages.templ, message.templ, aggregate.templ, dashboard.templ, deletions.templ, thread.templ, search.templ |

## Indicator Coverage

| Template | Trigger Points | Indicator Used |
|----------|---------------|----------------|
| layout.templ | 5 nav links | #page-indicator |
| components.templ | Pagination Prev/Next, SortHeader | #page-indicator |
| messages.templ | Message rows | #page-indicator |
| message.templ | Back link, View thread, Text/HTML toggle, Load images (2x) | #page-indicator |
| aggregate.templ | View tabs (2), filter input, drill-down rows, breadcrumbs, sub-view tabs (2+), drilldown msg rows, staging form | #filter-indicator, #stage-indicator, #page-indicator |
| dashboard.templ | Chart rows, Top Senders rows, Top Domains rows | #page-indicator |
| deletions.templ | Cancel button | #page-indicator |
| thread.templ | Back link, Text toggle, Load images, lazy-load div | #page-indicator |
| search.templ | Search result rows (input unchanged) | #page-indicator |

**Total hx-indicator attributes:** 37

## Deviations from Plan

None — plan executed exactly as written. The "universal #page-indicator" simplification described in the plan's action section was followed. The filter input got a specialized #filter-indicator (as specified), and the staging form got a specialized #stage-indicator (as specified).

## Deferred Items

Pre-existing vet errors in unrelated packages (not introduced by this plan):
- `internal/mbox/client_test.go:93` — undefined `writeTempMbox`
- `internal/export/attachments_test.go:88` — wrong argument count in `Attachments` call
- `cmd/msgvault/cmd/validation_test.go:33` — `TestEmailValidation` redeclared

These were pre-existing before Phase 09 work began and are out of scope for this plan.

## Self-Check: PASSED
