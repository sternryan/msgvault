# Worklog

**Session 2026-03-29 — msgvault quick-task**

- Fixed 3 bugs from roborev CI review on wesm/msgvault#224 (iMessage sync PR)
- Timezone date filters: replaced 18 `time.Parse("2006-01-02")` → `time.ParseInLocation(..., time.Local)` across 9 files (6 CLI commands, web params, MCP handler, validation test)
- Batch nil entries: switched `GetMessagesRawBatch` in imessage + gvoice from pre-allocated indexed slice to append-based (compact, nil-free results)
- Attachment warning: added Warn-level log when `cache_has_attachments != 0` in iMessage client (silent data loss → explicit)
- Confirmed attributedBody fallback was already implemented (client.go:305-313)
- Pushed fixes to `imessage-upstream` branch, updating PR #224 on wesm/msgvault
- Posted PR comment addressing all 4 review items
- 5 commits on main, 1 commit on imessage-upstream, all tests pass (35 packages)
- No blockers or carryover
