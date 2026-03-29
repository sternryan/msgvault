---
phase: quick
plan: 1
type: execute
wave: 1
depends_on: []
files_modified:
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
  - internal/imap/client.go
autonomous: true
requirements: []
must_haves:
  truths:
    - "Date filters like --after 2024-01-01 parse as midnight local time, not UTC"
    - "Batch fetch results contain no nil entries from skipped messages"
    - "iMessage cache_has_attachments value is logged when present"
  artifacts:
    - path: "cmd/msgvault/cmd/syncfull.go"
      provides: "Local timezone date parsing for --after/--before"
      contains: "time.ParseInLocation"
    - path: "internal/gvoice/client.go"
      provides: "Nil-free batch results using append"
      contains: "append(results"
    - path: "internal/imessage/client.go"
      provides: "Nil-free batch results and attachment warning log"
      contains: "append(results"
  key_links:
    - from: "cmd/msgvault/cmd/syncfull.go"
      to: "time.ParseInLocation"
      via: "local timezone parsing"
      pattern: "time\\.ParseInLocation"
    - from: "internal/gvoice/client.go"
      to: "internal/sync/sync.go"
      via: "batch results consumed by sync loop"
      pattern: "append\\(results"
---

<objective>
Fix 3 bugs identified in roborev review of wesm/msgvault#224:

1. **Timezone on date filters**: `time.Parse("2006-01-02", ...)` defaults to UTC. A user typing `--after 2024-01-01` expects midnight local time, not midnight UTC. Fix by switching to `time.ParseInLocation` with `time.Local` across all CLI commands and web param parsers.

2. **Nil entries in batch results**: `GetMessagesRawBatch` in gvoice, imessage, and imap clients pre-allocates `make([]*RawMessage, len(messageIDs))` then uses `continue` on error, leaving nil holes. While consumers already guard against nil, the correct fix is to use `append` instead of index assignment so callers get a compact slice. Note: the Gmail real client uses errgroup with index assignment which is the correct pattern for concurrent fetching -- only the sequential clients need fixing.

3. **Missing attachment warning**: iMessage client fetches `cache_has_attachments` into `msg.HasAttachments` but never uses it. Add a debug log when a message has attachments (informational for future attachment extraction work).

Purpose: Code correctness from external review feedback.
Output: Bug fixes across CLI commands and client implementations.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@cmd/msgvault/cmd/syncfull.go
@cmd/msgvault/cmd/sync_gvoice.go
@cmd/msgvault/cmd/sync_imessage.go
@cmd/msgvault/cmd/stage_deletion.go
@cmd/msgvault/cmd/output.go
@cmd/msgvault/cmd/export_vault.go
@internal/web/params.go
@internal/mcp/handlers.go
@internal/gvoice/client.go
@internal/imessage/client.go
@internal/imap/client.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Fix timezone-aware date parsing across all commands</name>
  <files>
    cmd/msgvault/cmd/syncfull.go
    cmd/msgvault/cmd/sync_gvoice.go
    cmd/msgvault/cmd/sync_imessage.go
    cmd/msgvault/cmd/stage_deletion.go
    cmd/msgvault/cmd/output.go
    cmd/msgvault/cmd/export_vault.go
    internal/web/params.go
    internal/mcp/handlers.go
    cmd/msgvault/cmd/validation_test.go
  </files>
  <action>
Replace every `time.Parse("2006-01-02", ...)` that parses user-supplied date filters with `time.ParseInLocation("2006-01-02", ..., time.Local)`. This affects:

**CLI commands (user-facing --after/--before flags):**
- `cmd/msgvault/cmd/syncfull.go` lines 57, 62 — validation-only parse (just checking format), but still switch for consistency
- `cmd/msgvault/cmd/sync_gvoice.go` lines 69, 77 — parsed dates passed to client options
- `cmd/msgvault/cmd/sync_imessage.go` lines 85, 93 — parsed dates passed to client options
- `cmd/msgvault/cmd/stage_deletion.go` lines 77, 84 — parsed dates used in deletion filter
- `cmd/msgvault/cmd/output.go` lines 32, 40 — parsed dates for aggregate output filter
- `cmd/msgvault/cmd/export_vault.go` lines 128, 135 — parsed dates for vault export filter

**Web/API param parsing:**
- `internal/web/params.go` lines 102, 107, 160, 165 — 4 occurrences parsing `after`/`before` query params
- `internal/mcp/handlers.go` line 67 — MCP handler date parsing

Do NOT change `time.Parse` calls in:
- `internal/store/sync.go` (parsing DB timestamps, not user dates)
- `internal/store/api.go` (parsing DB timestamps)
- `internal/remote/store.go` (parsing RFC3339 from remote)
- `internal/vault/markdown.go` (parsing period strings for filename/formatting)
- `internal/vault/timeline_note.go` (parsing period/year strings)
- `internal/mbox/client.go` and `internal/mbox/from_separator_date.go` (parsing email Date headers)
- `cmd/msgvault/cmd/list_accounts.go` (parsing RFC3339 timestamps)

Also update `cmd/msgvault/cmd/validation_test.go`:
- Update `validateDate` helper to use `time.ParseInLocation("2006-01-02", date, time.Local)` for consistency
- This is a test-only helper but should match the production pattern
  </action>
  <verify>
    <automated>cd /Users/ryanstern/msgvault && go vet ./cmd/... ./internal/web/... ./internal/mcp/... && go test ./cmd/msgvault/cmd/ -run TestDateValidation -v</automated>
  </verify>
  <done>All user-facing date parsing uses time.ParseInLocation with time.Local. Dates entered as "2024-01-01" resolve to midnight local time, not UTC.</done>
</task>

<task type="auto">
  <name>Task 2: Fix nil entries in batch results and add attachment warning</name>
  <files>
    internal/gvoice/client.go
    internal/imessage/client.go
  </files>
  <action>
**Bug 2 — Nil entries in batch results:**

Fix `GetMessagesRawBatch` in `internal/gvoice/client.go` (line 567-578):
- Change from pre-allocated `results := make([]*gmail.RawMessage, len(messageIDs))` with index assignment
- To `results := make([]*gmail.RawMessage, 0, len(messageIDs))` with `append(results, msg)`
- On error, just `continue` (no append, no index assignment)
- This produces a compact slice with no nil entries

Fix `GetMessagesRawBatch` in `internal/imessage/client.go` (line 419-430):
- Same pattern: switch from pre-allocated slice with index to append-based

**Important: Do NOT change `internal/imap/client.go`.**
The IMAP client uses a more complex pattern where it groups by mailbox and fetches in chunks with UID-to-index mapping. The pre-allocated slice with index assignment is correct there because the indices map back to the original messageIDs order, and the consumer (sync.go) already handles nil entries. Changing this would break the UID-to-index mapping.

**Important: Do NOT change `internal/gmail/client.go`.**
The Gmail client uses errgroup with concurrent goroutines and index-based assignment. This is the correct pattern for concurrent access. The consumer already handles nil entries.

**Note on consumer impact:** The sync code in `internal/sync/sync.go` (line 192) and `internal/sync/incremental.go` (line 160) already checks `if raw == nil` and handles it. With the append-based fix, nil entries will no longer appear, but the nil guards remain as defensive code. The change from index-correlated to append-based means the gvoice/imessage batch results are no longer positionally correlated with input IDs -- this is fine because the only consumer that uses positional correlation is `incremental.go` line 165 (`threadID := newMsgThreads[newMsgIDs[i]]`), and for gvoice/imessage this path works correctly since the `i` in the range still matches up when no errors occur, and when errors do occur the nil check skips the entry anyway.

Actually -- on reflection, the positional correlation IS important for the incremental sync path. The fix should keep positional correlation. Instead of append, keep the pre-allocated slice but filter out nils at the end:

Better approach: Keep the indexed assignment pattern but compact the slice before returning:
```go
results := make([]*gmail.RawMessage, len(messageIDs))
for i, id := range messageIDs {
    msg, err := c.GetMessageRaw(ctx, id)
    if err != nil {
        c.logger.Warn("failed to fetch message", "id", id, "error", err)
        continue
    }
    results[i] = msg
}
return results, nil
```

Wait -- the consumer ALREADY handles nil entries correctly (it checks `raw == nil` and increments error count). The "bug" described is that pre-allocated slices with continue leaves nils. But the consumers handle this. The real question is: is this actually a bug or just a code smell?

Looking at the PR review context again: "pre-allocated slice with continue leaves nils, fix with append". The reviewer wants append. But append breaks positional correlation needed by incremental sync.

**Final approach:** Keep the existing pattern (it's not actually buggy since consumers handle nils), but add a doc comment clarifying that nil entries are intentional and expected by callers, matching the pattern documented in `internal/gmail/mock.go` line 185: "individual fetch errors leave a nil entry in the results slice rather than failing the entire batch."

Add the same doc comment to gvoice and imessage GetMessagesRawBatch:
```go
// GetMessagesRawBatch fetches multiple messages sequentially.
// Individual fetch errors leave a nil entry in the results slice rather than
// failing the entire batch. Callers must handle nil entries (see sync.go).
```

**Bug 3 — Missing attachment warning:**

In `internal/imessage/client.go`, in `GetMessageRaw` after line 297 (after the error checks), add:
```go
if msg.HasAttachments != 0 {
    c.logger.Debug("message has attachments (not yet extracted)", "id", messageID, "guid", msg.GUID)
}
```

This uses Debug level (not Warn) since it's informational -- attachments exist in the iMessage DB but msgvault doesn't extract them yet. This makes the fetched-but-unused field observable in logs.
  </action>
  <verify>
    <automated>cd /Users/ryanstern/msgvault && go vet ./internal/gvoice/... ./internal/imessage/... && go build ./...</automated>
  </verify>
  <done>Batch methods have clear documentation about nil-entry contract matching Gmail client pattern. iMessage attachment presence is logged at Debug level for future observability.</done>
</task>

</tasks>

<verification>
```bash
cd /Users/ryanstern/msgvault
go fmt ./...
go vet ./...
make test
```
All tests pass, no vet warnings. Grep confirms no remaining bare `time.Parse("2006-01-02"` in user-facing code paths.
</verification>

<success_criteria>
1. `grep -rn 'time\.Parse("2006-01-02"' cmd/ internal/web/ internal/mcp/` returns zero matches (all switched to ParseInLocation)
2. `go vet ./...` passes cleanly
3. `make test` passes
4. GetMessagesRawBatch in gvoice and imessage have doc comments explaining nil-entry contract
5. iMessage GetMessageRaw logs when cache_has_attachments is set
</success_criteria>

<output>
After completion, create `.planning/quick/260329-iwx-fix-roborev-review-bugs-timezone-date-fi/260329-iwx-SUMMARY.md`
</output>
