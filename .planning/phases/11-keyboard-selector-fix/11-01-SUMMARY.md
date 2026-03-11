---
phase: 11-keyboard-selector-fix
plan: 01
subsystem: web-ui
tags: [keyboard, dom-selectors, htmx, templates, tdd, css, go.mod]
dependency_graph:
  requires: []
  provides: [PARITY-07, POLISH-03, INT-03, INT-04, INT-05]
  affects: [internal/web/static/keys.js, internal/web/templates/components.templ, internal/web/static/style.css, go.mod]
tech_stack:
  added: []
  patterns:
    - "TDD RED/GREEN cycle: failing test first, then minimal fix to pass"
    - "templ.KV() for conditional class attributes on th elements"
    - "data-sort-field attribute on SortHeader th for JS selector targeting"
requirements_completed: [PARITY-07, POLISH-03]
key_files:
  created: []
  modified:
    - internal/web/handlers_test.go
    - internal/web/templates/components.templ
    - internal/web/templates/components_templ.go
    - internal/web/static/style.css
    - internal/web/static/keys.js
    - go.mod
    - go.sum
    - .planning/phases/09-polish/09-02-SUMMARY.md
decisions:
  - "templ.KV('active', field == currentField) on the th class list — idiomatic templ conditional class pattern"
  - "data-sort-field on the th element itself (not the inner anchor) — matches JS querySelector target level"
metrics:
  duration: "4min"
  completed_date: "2026-03-11"
  tasks_completed: 2
  files_modified: 8
---

# Phase 11 Plan 01: Keyboard Selector Fix Summary

**One-liner:** Fixed DOM selector mismatches in keys.js and SortHeader template so j/k row nav and s/r sort cycling work correctly in the web UI.

## What Was Built

Four broken DOM selectors in keys.js were corrected to match what the templates actually emit. The SortHeader template was enhanced to emit the attributes the JS depends on. CSS class name was harmonized. bluemonday promoted to direct dependency.

### Changes

**keys.js (3 selector fixes):**
- `moveRow()`: `[data-row]` → `[data-href]` — message/aggregate/search/dashboard rows emit `data-href`, not `data-row`
- `cycleSortField()`: `.sort-header[data-sort-field]` → `.sortable-header[data-sort-field]`
- `cycleSortField()` + `reverseSortDir()`: `.sort-header.active` → `.sortable-header.active`

**components.templ (SortHeader):**
- Added `data-sort-field={ field }` attribute to the `<th>` element
- Added `templ.KV("active", field == currentField)` to the class list

**style.css:**
- Renamed `.sort-header` → `.sortable-header` to match the template class

**go.mod:**
- Promoted `github.com/microcosm-cc/bluemonday` from `// indirect` to direct dependency

**09-02-SUMMARY.md:**
- Added `requirements_completed: [POLISH-03]` to frontmatter (doc gap closure)

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 (RED) | Add failing TestSortHeaderEmitsSortField | 400cf87c | handlers_test.go |
| 1 (GREEN) | Fix SortHeader attributes, CSS class, promote bluemonday | 02e11e1b | components.templ, components_templ.go, style.css, go.mod, go.sum |
| 2 | Fix keys.js selectors + update 09-02-SUMMARY frontmatter | 24f457f3 | keys.js, 09-02-SUMMARY.md |

## Deviations from Plan

None — plan executed exactly as written.

## Self-Check: PASSED
