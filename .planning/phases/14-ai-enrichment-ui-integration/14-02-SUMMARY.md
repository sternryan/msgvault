---
phase: 14-ai-enrichment-ui-integration
plan: "02"
subsystem: web-ui, cli
tags: [entities, web-ui, htmx, templ, cli, export, lifevault]
dependency_graph:
  requires:
    - internal/store/enrichment.go (GetEntities, GetLifeEventsForExport — Plan 14-01)
    - internal/web/server.go (route registration)
    - internal/web/handlers.go (handlers struct, renderPage)
  provides:
    - internal/web/handlers_entities.go (GET /entities, GET /entities/partial)
    - internal/web/templates/entities.templ (EntitiesPage, EntitiesTableContent, EntitiesPagination)
    - cmd/msgvault/cmd/export_timeline.go (msgvault export-timeline CLI)
  affects:
    - internal/web/server.go (added /entities and /entities/partial routes)
    - internal/web/templates/layout.templ (added Entities nav link)
tech_stack:
  added: []
  patterns:
    - HTMX partial swap target #entities-table for filter/search (existing HTMX pattern)
    - Entity type allowlist validation in handler (T-14-05 mitigation)
    - Templ auto-escaping for all entity values (T-14-07 — no raw HTML rendering)
    - Cobra command with RunE + openStore pattern (matches embed.go and enrich.go)
key_files:
  created:
    - internal/web/handlers_entities.go
    - internal/web/templates/entities.templ
    - internal/web/templates/entities_templ.go
    - cmd/msgvault/cmd/export_timeline.go
  modified:
    - internal/web/server.go (added 2 routes)
    - internal/web/templates/layout.templ (added Entities nav link)
    - internal/web/templates/layout_templ.go (regenerated)
decisions:
  - "Entity rows link to /search?q=<value> rather than a custom message list — reuses existing search infrastructure and avoids a new endpoint"
  - "EntitiesTableContent rendered as separate templ component — enables both full-page and HTMX partial swap without duplication"
  - "entitiesPage detects HX-Request header to skip Layout wrapper for HTMX nav clicks — consistent with how other pages behave"
  - "export-timeline writes entries as empty array [] when no life events exist — valid JSON, avoids null output"
metrics:
  duration_minutes: 25
  completed_date: "2026-04-12"
  tasks_completed: 2
  files_created: 4
  files_modified: 3
  lines_added: ~1000
  commits: 2
---

# Phase 14 Plan 02: Export-Timeline CLI and Entities Web Page Summary

**One-liner:** Entities web page at /entities with type filter dropdown, HTMX search, paginated table, and entity→search back-links; plus `msgvault export-timeline` CLI command producing LifeVault-compatible JSON arrays.

## What Was Built

### Task 1: Entities Web Page (commit `b7ec37fb`)

**`internal/web/handlers_entities.go`:**
- `entitiesPage` handler: parses type/q/offset/limit params, validates `type` against `validEntityTypes` allowlist (T-14-05), calls `h.store.GetEntities()`, renders `EntitiesPage` template. Detects `HX-Request` header to render without Layout wrapper for HTMX nav clicks.
- `entitiesPartial` handler: same query logic but always renders `EntitiesTableContent` (no Layout) — used as HTMX swap target for filter/search changes.
- `parseEntitiesParams`: extracts and validates params; entity type stripped to `""` if not in allowlist.

**`internal/web/templates/entities.templ`:**
- `EntitiesPage`: filter bar with type `<select>` (hx-get /entities/partial, hx-include `[name='q']`) and search input (hx-trigger keyup delay:300ms, hx-include `[name='type']`), wraps `#entities-table` div for HTMX swaps.
- `EntitiesTableContent`: table with Type (entity-badge with type-specific CSS class), Value, Normalized, Context, Messages (link to `/search?q=<value>`); plus `EntitiesPagination`.
- `EntitiesPagination`: prev/next with hx-target `#entities-table` (no full-page reload for pagination).

**`internal/web/server.go`:** Added `r.Get("/entities", h.entitiesPage)` and `r.Get("/entities/partial", h.entitiesPartial)`.

**`internal/web/templates/layout.templ`:** Added Entities nav link after Deletions, before `nav-spacer`, with `strings.HasPrefix(currentPath, "/entities")` active state.

### Task 2: Export-Timeline CLI (commit `c95ab3a6`)

**`cmd/msgvault/cmd/export_timeline.go`:**
- `TimelineEntry` struct: `date`, `type`, `description`, `source_message_id` JSON tags — matches LifeVault schema.
- `exportTimelineCmd`: Cobra command `export-timeline` with `--output/-o`, `--type`, `--pretty` (default true) flags.
- `runExportTimeline`: opens store, calls `GetLifeEventsForExport(eventType)`, maps to `[]TimelineEntry`, marshals with `json.MarshalIndent` or `json.Marshal`, writes to file or stdout, prints summary to stderr.
- Registered with `rootCmd.AddCommand` in `init()`.

## Verification Results

```
go build ./cmd/msgvault/        → SUCCESS
go vet ./internal/web/...       → CLEAN
go vet ./cmd/msgvault/...       → CLEAN
./msgvault export-timeline --help → shows --output, --type, --pretty flags
```

## Deviations from Plan

None — plan executed exactly as written.

Note: Plan 14-01 commits (`58d12f8d`, `030fb727`) were cherry-picked into this worktree at execution start since this worktree was branched before Phase 14 work began. The cherry-pick was clean with no conflicts.

## Known Stubs

None — all handlers are fully wired to real store methods. No hardcoded empty values flow to the UI.

## Threat Flags

No new threat surface beyond what was in the plan's threat model:
- T-14-05: Entity type allowlist (`validEntityTypes` map) enforced in `parseEntitiesParams` — strips to `""` for unknown values
- T-14-06: `export-timeline` is local CLI writing to local filesystem — accepted
- T-14-07: Templ auto-escapes all interpolated values; no `templ.Raw()` used for entity data

## Self-Check: PASSED

All created files exist on disk. Both commits (`b7ec37fb`, `c95ab3a6`) confirmed in git log.
