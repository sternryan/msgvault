# msgvault: offline Gmail/IMAP archive with local search and analytics (Go)

See `README.md` for overview, stack, commands, and deployment.

## Commands

```bash
make build          # debug build
make build-release  # optimized build
make test           # run tests
make lint           # run linter (auto-fix)
```

## Conventions

- Commit after every turn without asking, if changes were made. Don't ask "shall I commit?", just commit.
- Complete multi-step chains (implement + commit + PR) in one pass; don't stop mid-chain.
- PR descriptions are changelog-oriented (what/why/how to use): no test plans or implementation detail.
- After any Go change, run `go fmt ./...` and `go vet ./...` before committing, and stage the resulting formatting diffs too.

## Gotchas

- SQLite driver is `mutecomm/go-sqlcipher/v4` (no key set), not `mattn/go-sqlite3`: the two link the SQLite C library and collide with ~260 duplicate symbols at link time. `go-sqlite3` is not even a dependency; don't add it back.
- **Never call LLM/encoder endpoints in the triage hot path** (D-04, Phase 14). Semantic similarity, if ever needed, goes through the `topic_pairs` cache populated by forge's `audit-bridges` / `synthesize`, not an inline call.
- `internal/triage/jsonl.go` output is byte-deterministic for byte-identical inputs (no `time.Now()`, `SetEscapeHTML(false)`). The 11-field `Candidate` schema is a locked contract with forge's `_ingest_from_triage`: changing it requires updating both repos.
- Never `SELECT DISTINCT` with a JOIN: use an `EXISTS` subquery instead (semi-join: stops at first match, no dedup needed).
- Never JOIN or scan `message_bodies` in list/aggregate/search queries: it's split from `messages` to keep that table's B-tree small. Only touch it via direct PK lookup for single-message detail views; use FTS5 (`messages_fts`) for search.
