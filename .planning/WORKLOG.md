# Worklog

**Session 2026-03-29 — msgvault PR review fixes (wesm/msgvault#224)**

- Addressed 2 rounds of roborev CI review on iMessage sync PR
- Round 1 (3 fixes): timezone date filters (18 `time.Parse` → `time.ParseInLocation` across 9 files), batch nil entries (append-based `GetMessagesRawBatch` in imessage + gvoice), attachment warning (Warn-level log on `cache_has_attachments`)
- Round 2 (3 fixes): attributedBody fallback (NSKeyedArchiver plist decoder + fallback when text is NULL), Message-ID sanitization (SHA-256 hash of GUID for RFC 5322 compliance), nullable Service field (`*string` to handle NULL on system messages)
- 11 files changed across both branches, 35 test packages pass, 6 new tests added
- 7 commits on main, 3 commits on imessage-upstream branch
- Repo owner (wesm) responded: reviewing iMessage, WhatsApp, and Google Voice PRs together for storage layer coherence before merging
- No blockers — waiting on owner review
