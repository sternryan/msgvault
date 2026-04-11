# Worklog

**Session 2026-04-11 — msgvault: v1.2 AI Archive Intelligence (Phases 12-13)**

- Carryover from Outlook import: committed merge script, gitignored local data dirs, committed Azure client_id in config.toml
- Fixed IMAP source lookup bug: `GetSourcesByIdentifier` now fuzzy-matches IMAP URIs by email (verified on Mac Mini — 2 new msgs synced)
- Researched Azure $200 credits: GPT-4o-mini at $0.15/M tokens makes full-archive enrichment viable (~$150 for embeddings + categorization + timeline + entities)
- Initialized v1.2 milestone: 3 phases, 16 requirements across Embeddings, Enrichment, Pipeline
- Phase 12 (Pipeline Infrastructure): Azure OpenAI config, HTTP client with retry/backoff, dual TPM/RPM rate limiter, batch runner with checkpoints, progress display, `pipeline test` CLI — 9 files, 1,420 lines, 28 tests
- Phase 13 (Embeddings & Vector Search): sqlite-vec CGO wrapper (solved 3.33.0 compat), vec_messages table, embedding pipeline, `embed` + `search --semantic/--hybrid` CLI, hybrid RRF re-ranking (k=60), web UI semantic/hybrid tabs with similarity badges — 18 files, 12,800 lines, 30+ tests
- 27 commits, 51 files changed, +18,137/-121 lines, 4 new packages (ai, embedding, sqlvec, store/pipeline)
- Carryover: Phase 14 (AI Enrichment — categorization, life timeline, entity extraction, 7 reqs), web UI checkpoint visual verification

**Session 2026-03-29 — msgvault PR review fixes (wesm/msgvault#224)**

- Addressed 2 rounds of roborev CI review on iMessage sync PR
- Round 1 (3 fixes): timezone date filters (18 `time.Parse` → `time.ParseInLocation` across 9 files), batch nil entries (append-based `GetMessagesRawBatch` in imessage + gvoice), attachment warning (Warn-level log on `cache_has_attachments`)
- Round 2 (3 fixes): attributedBody fallback (NSKeyedArchiver plist decoder + fallback when text is NULL), Message-ID sanitization (SHA-256 hash of GUID for RFC 5322 compliance), nullable Service field (`*string` to handle NULL on system messages)
- 11 files changed across both branches, 35 test packages pass, 6 new tests added
- 7 commits on main, 3 commits on imessage-upstream branch
- Repo owner (wesm) responded: reviewing iMessage, WhatsApp, and Google Voice PRs together for storage layer coherence before merging
- No blockers — waiting on owner review
