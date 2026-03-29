---
phase: quick
plan: 260329-iwx
subsystem: cli, web, imessage, gvoice
tags: [bug-fix, timezone, date-parsing, batch-results, imessage-attachments]
dependency_graph:
  requires: []
  provides: [correct-date-filter-semantics, compact-batch-results, attachment-warning-log]
  affects: [cmd/syncfull, cmd/sync_gvoice, cmd/sync_imessage, cmd/stage_deletion, cmd/output, cmd/export_vault, internal/web/params, internal/mcp/handlers, internal/gvoice/client, internal/imessage/client]
tech_stack:
  added: []
  patterns: [time.ParseInLocation for user-facing dates, append-based batch accumulation, Warn-level log for unarchived data]
key_files:
  created: []
  modified:
    - cmd/msgvault/cmd/syncfull.go
    - cmd/msgvault/cmd/sync_gvoice.go
    - cmd/msgvault/cmd/sync_imessage.go
    - cmd/msgvault/cmd/stage_deletion.go
    - cmd/msgvault/cmd/output.go
    - cmd/msgvault/cmd/export_vault.go
    - cmd/msgvault/cmd/validation_test.go
    - internal/web/params.go
    - internal/mcp/handlers.go
    - internal/gvoice/client.go
    - internal/imessage/client.go
decisions:
  - Use time.ParseInLocation with time.Local (not UTC) for all user-supplied date strings
  - Switch gvoice/imessage GetMessagesRawBatch to append (compact slice, no nil holes)
  - Log attachment presence at Warn (not Debug) because it is silent data loss
metrics:
  duration: ~15 minutes
  completed: 2026-03-29
  tasks_completed: 2
  files_modified: 11
---

# Quick Fix 260329-iwx: Fix Roborev Review Bugs Summary

**One-liner:** Timezone-correct date parsing (ParseInLocation+Local) across 9 user-facing files, compact batch results via append in gvoice/imessage, and Warn-level attachment log for unarchived iMessage attachments.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Fix timezone-aware date parsing | `ebe4b596` | syncfull.go, sync_gvoice.go, sync_imessage.go, stage_deletion.go, output.go, export_vault.go, web/params.go, mcp/handlers.go, validation_test.go |
| 2 | Compact batch results + attachment warning | `f1c9ba04` | internal/gvoice/client.go, internal/imessage/client.go |

## What Was Fixed

### Bug 1: Timezone on date filters (Task 1)

`time.Parse("2006-01-02", ...)` parses as UTC midnight. A user running
`--after 2024-01-01` in e.g. America/Los_Angeles would get UTC midnight,
which is 8 hours off from local midnight — silently excluding messages from
the first 8 hours of Jan 1 local time.

**Fix:** Replaced all 11 occurrences (9 CLI/web files) with
`time.ParseInLocation("2006-01-02", ..., time.Local)`.

**Exclusions honored:** DB timestamp parsing, RFC3339 remote parsing, email
Date header parsing, vault period/year string parsing — all left unchanged as
they parse non-user-supplied values.

### Bug 2: Nil entries in batch results (Task 2 — override applied)

`GetMessagesRawBatch` in `gvoice/client.go` and `imessage/client.go` used
pre-allocated `make([]*RawMessage, len(ids))` with index assignment and
`continue` on error, leaving nil holes in the slice. The sync consumers
(`sync.go:193`, `incremental.go:161`) already guard `if raw == nil`, so
this was not breaking — but was a code smell flagged by the PR reviewer.

**Fix:** Switched to `make([]*RawMessage, 0, len(ids))` + `append(results, msg)`,
so the returned slice is always compact. The existing nil guards in sync.go
remain as defensive code.

Note: `internal/imap/client.go` and `internal/gmail/client.go` were
intentionally NOT changed — imap uses UID-to-index mapping that requires
positional correlation; gmail uses errgroup with concurrent goroutines.

### Bug 3: Missing attachment warning (Task 2 — override applied)

`imessage/client.go GetMessageRaw` fetched `cache_has_attachments` from
the DB into `msg.HasAttachments` but never used the value. Attachments exist
in the iMessage DB but are not yet extracted or archived by msgvault —
silent data loss flagged HIGH severity by the reviewer.

**Fix:** Added Warn-level log immediately after error checks:
```go
if msg.HasAttachments != 0 {
    c.logger.Warn("message has attachments that will not be archived (attachment extraction not yet implemented)", "id", messageID, "guid", msg.GUID)
}
```
Used Warn (not Debug) because this represents unarchived data.

## Verification

- `grep -rn 'time\.Parse("2006-01-02"' cmd/ internal/web/ internal/mcp/` returns zero matches
- `go vet ./...` passes cleanly
- `go build ./...` passes cleanly
- All tests pass (full suite via pre-commit hook on both commits)

## Deviations from Plan

### Task 2 Override Applied

The original plan's Task 2 called for adding doc comments only (not code changes) for the batch nil issue, and using Debug level for the attachment log. The user overrode both:

1. **Batch fix (gvoice + imessage):** Changed from doc-comment-only to actual append-based fix as originally described in the reviewer's report.
2. **Attachment log level:** Changed from Debug to Warn because missing attachment archival is silent data loss, not just informational.

Both overrides tracked as intentional — no deviations from the override instructions.

## Self-Check: PASSED

- `ebe4b596` exists in git log: FOUND
- `f1c9ba04` exists in git log: FOUND
- `/Users/ryanstern/msgvault/.claude/worktrees/agent-aa3ebb3c/internal/imessage/client.go` contains `append(results`: FOUND
- `/Users/ryanstern/msgvault/.claude/worktrees/agent-aa3ebb3c/internal/gvoice/client.go` contains `append(results`: FOUND
- Zero bare `time.Parse("2006-01-02"` in cmd/ internal/web/ internal/mcp/: CONFIRMED
