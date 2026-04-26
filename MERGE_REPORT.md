# Merge Report: upstream/main → fork main

**Worktree:** `/Users/ryanstern/msgvault/.claude/worktrees/agent-a448c0e80c510157a`
**Merge base:** `14b06871bc74a45ec38b0c670f5c2559c2eb8884`
**Local tip (HEAD):** `503711c2`
**Upstream tip:** `327fb13a`
**Counts at start:** 171 ahead / 37 behind

---

## 1. Status (UPDATED 2026-04-25)

**Ready for review.** All textual conflicts resolved. `go build` passes.
`go vet` passes. **All 45 test packages pass** (0 failures).

**Post-agent fixes applied:**

1. **SQLite driver clash resolved.** Replaced every `github.com/mattn/go-sqlite3`
   import with `github.com/mutecomm/go-sqlcipher/v4` (drop-in API-compatible
   fork — same `sqlite3` package name, same `SQLiteDriver`/`SQLiteConn`/`Error`
   types and constants). Affected files: `cmd/msgvault/cmd/search_vector.go`,
   `internal/scheduler/scheduler_test.go`, `internal/textimport/integration_test.go`,
   `internal/imessage/client.go`, `internal/store/dialect_sqlite.go`,
   `internal/vector/sqlitevec/{backend,ext,backend_testhelpers_test,fused_test}.go`,
   `internal/vector/hybrid/{filter_test,engine_test}.go`,
   `internal/whatsapp/queries_test.go`, `internal/store/db_logger_test.go`.
   Three comment-only references retained (no code impact).

2. **`sqlite_vec` build tag dropped from default.** `BUILD_TAGS` in `Makefile`
   reduced from `fts5 sqlite_vec` to `fts5`. Reason: `sqlite-vec` calls
   `sqlite3_vtab_in*` (added in SQLite 3.38, 2022) which `go-sqlcipher v4.4.2`
   does not expose. Re-enable once sqlcipher upgrades its bundled SQLite.
   **Side effect:** upstream's `internal/vector/sqlitevec/` C-extension paths
   are dormant in default builds. The fork's existing
   `internal/embedding/` AI Archive Intelligence subsystem is unaffected.

3. **`loggedDB.ExecContext` panic fix.** Multi-statement DDL execs in
   sqlcipher return a `Result` whose underlying `driver.Result` is nil —
   calling `RowsAffected()` panicked. Extracted to `safeRowsAffected(err, res)`
   helper with `defer recover` so logging never crashes callers.
   `internal/store/db_logger.go`.

4. **Search address-token normalization restored.** HEAD's hot-path
   tokenizer in `dispatchToken` was wrapping address values with
   `toLowerFast` only. Now wraps with `normalizeAddr(toLowerFast(value))`
   for `from:`, `to:`, `cc:`, `bcc:` so bare-domain inputs (e.g.
   `from:example.com`) get the upstream-introduced `@` prefix. Restores
   `TestParse/DomainNormalization/*` to green.

The merge has not been committed. `git status` shows
"All conflicts fixed but you are still merging" — ready for human review.

---

## 2. Build result

```
CGO_ENABLED=1 go build -tags "fts5 sqlite_vec" ./...
```

**Exit:** non-zero. **All Go compile errors are gone.** Failure is at the
`cc` link stage with hundreds of duplicate symbols of the form
`_sqlite3_vsnprintf`, `_sqlite3_load_extension`, `_sqlite3_column_count`,
`_sqlite3_value_text16le`, etc., between two object files (one is the
sqlite3 amalgamation embedded by `mattn/go-sqlite3`, the other is the
amalgamation embedded by `mutecomm/go-sqlcipher`). Additional
`_sqlite3_vec_numpy_init`, `_vec0_*`, `_vector_*` duplicates show that
`asg017/sqlite-vec-go-bindings/cgo` is also colliding.

**This is not a logic error.** It is a packaging clash from picking up
both a fork-side encryption driver and an upstream-side vanilla sqlite
driver in the same binary.

---

## 3. Vet result

```
CGO_ENABLED=1 go vet -tags "fts5 sqlite_vec" ./...
```

**Exit:** 0. Clean. (Initial vet exposed three signature-drift issues in
test files; all were repaired — see §6.)

---

## 4. Test summary

`go test` was not run because the link failure prevents any binary from
being produced. Once §8 is resolved the test suite can run.

---

## 5. Files hand-resolved (one-line summary each)

| File | Resolution |
|------|------------|
| `internal/store/store.go` | Union per spec — kept SQLCipher passphrase + `tryLock` + `lockFile` + advisory lock + `migrateAddContentID` + `InitVectorTable` + `Passphrase()` + `DBPath()`; added upstream's `Dialect`, `loggedDB`, `OpenReadOnly`, `IsBusyError`, `SchemaStale`, dialect-driven FTS loading. `DB()` now returns `s.db.DB`. Close() checkpoints WAL (when not read-only) AND releases advisory lock. |
| `internal/store/messages.go` | UpsertAttachment INSERT — kept HEAD's `content_id` column AND adopted upstream's `s.dialect.Now()` formatter. New 7-arg signature. |
| `internal/sync/sync.go` | DefaultOptions — kept HEAD's CheckpointInterval + SourceType. Added missing `CheckpointInterval int` field on `Options`. Reverted upstream's debug log wording change (`gmail_id` over `id`). |
| `internal/config/config.go` | Combined struct — added upstream `Log`, `Vector`, kept HEAD `Encryption`, `AzureOpenAI`. Removed HEAD's duplicate `MicrosoftConfig` definition (upstream's lives later in same file with identical semantics). Added upstream's `LogConfig`. Kept HEAD's `BackupsDir`, `DeletionsDir` AND upstream's `LogsDir`. |
| `internal/imap/config.go` | Took upstream's typed `AuthMethod` (`type AuthMethod string`) so the upstream `xoauth2.go` we adopted compiles. |
| `internal/imap/client.go` | Kept upstream's `WithDateFilter`, `WaitGreeting`, switch-on-auth-method; deleted HEAD's stringly-typed branch and the duplicate `tokenSource` struct field. |
| `internal/search/parser.go` | Kept HEAD's hot-path optimized parser (`dispatchToken`, `toLowerFast`, `parseSizeFast`, `queryStore`) AND added upstream's new helpers (`normalizeAddr`, `looksLikeDomain`, `knownGTLDs`, `isKnownTLD`, `operators` map, `HasOperators` method). HEAD's tokenizer wins. |
| `internal/export/attachments.go` | Took HEAD's `zipPath` variable name (upstream typo'd `zipFilename`). Added `_ =` for staticcheck. |
| `internal/tui/actions_test.go` | Kept HEAD — zip files now go to `dataDir/exports/`, no need for upstream's working-dir cleanup. |
| `cmd/msgvault/cmd/root.go` | Var block, PersistentPreRunE, init: union — kept `--quiet`, `passphrase` plumbing, encryption prompt. Adopted upstream's full structured-logging pipeline (`logging.BuildHandler`, `logResult`, `--log-file`, `--log-level`, `--no-log-file`, `--log-sql`, `--log-sql-slow-ms`, SQL log adapter config, startup/exit headers, `sanitizeArgs`, `recoverAndLogPanic`). `--quiet` now maps to a level override. |
| `cmd/msgvault/cmd/sync.go` | Took upstream's `imapSkipReason()` helper path; dropped HEAD's inline imaplib + microsoft import block. |
| `cmd/msgvault/cmd/syncfull.go` | Took upstream's `imapSkipReason` + `WithDateFilter` + `--after`/`--before` parsing in `buildAPIClient`; dropped HEAD's inline duplicate of the same logic. |
| `cmd/msgvault/cmd/search.go` | Combined flag set — kept HEAD's `--semantic`, `--hybrid` AND upstream's `--mode`, `--explain`. Deleted the duplicate `runHybridSearch`/`outputHybridResultsTable`/`outputHybridResultsJSON` from search.go in favor of the newer build-tagged versions in `search_vector.go` (which use `hybrid.ResultMeta` and the new `(mode, explain)` signature). Updated the `searchHybrid` call site to pass `"hybrid"`, `searchExplain`. |
| `cmd/msgvault/cmd/serve.go` | Took upstream's `Server.ValidateSecure` + APIKey-length warning. |
| `cmd/msgvault/cmd/deletions.go` | Removed HEAD's duplicate `store.Open`/`InitSchema` block (already done at line 394 by upstream). Threaded `passphrase` into the surviving open call. Combined `init()` — kept HEAD's `--skip-verify`, `--backup`, `--account`. |
| `Makefile` | Combined targets — kept HEAD's `templ-generate`, `setup-hooks` AND upstream's `bench`, `lint-ci`, `install-hooks`, `BUILD_TAGS := fts5 sqlite_vec`. `setup-hooks` is aliased to `install-hooks`. |
| `go.mod` | Union — kept HEAD's `templ`, `bluemonday`, `crypto`, `term`, `gopkg.in/yaml.v3`. Took upstream's pinned versions for `mark3labs/mcp-go`, `mattn/go-isatty`, `mattn/go-runewidth`, `mattn/go-sqlite3`, `golang.org/x/{mod,net,sys,text,tools,telemetry}`. Added upstream's `asg017/sqlite-vec-go-bindings`. Promoted indirect deps as needed. |
| `go.sum` | Took upstream wholesale, then ran `go mod tidy`. |
| `README.md` | Took upstream wholesale per spec. |

---

## 6. Files where upstream was taken wholesale (per spec)

```
cmd/msgvault/cmd/addo365.go
cmd/msgvault/cmd/embed.go
internal/microsoft/oauth.go
internal/microsoft/oauth_test.go
internal/imessage/{client,models,parser,parser_test}.go
internal/gvoice/{client,models,parser,parser_test}.go
internal/imap/xoauth2.go
internal/imap/xoauth2_test.go
go.sum
README.md
```

Plus three follow-on test fixes triggered by signature changes:

| File | Edit |
|------|------|
| `internal/whatsapp/importer.go` | Added empty `""` content_id arg to `UpsertAttachment` call. |
| `internal/store/sources_test.go` | Added empty `""` content_id arg to 6 `UpsertAttachment` test call sites. |
| `cmd/msgvault/cmd/remove_account_test.go` | Added empty `""` content_id arg to `UpsertAttachment` test call. |
| `internal/web/handlers_test.go` | Added `GetMessageSummariesByIDs` method to `mockEngine` so it implements `query.Engine` after upstream extended the interface. |

---

## 7. MERGE TODO list

Two files were stubbed out behind a never-true build tag to unblock the
rest of the binary. Both contain a top-of-file MERGE TODO comment
explaining the trade-off:

| File | Issue |
|------|-------|
| `cmd/msgvault/cmd/sync_gvoice.go` | HEAD-only `sync-gvoice` command. The HEAD-side gvoice client API (with `BatchDeleteMessages`, etc.) does not survive — we adopted upstream's gvoice package. Upstream wires Voice through `cmd/msgvault/cmd/import_gvoice.go` instead. Owner must decide: delete this file, or port to the new API as a parallel sync command. |
| `cmd/msgvault/cmd/sync_imessage.go` | Same situation as sync_gvoice. HEAD's `imessage.WithMyAddress`, `imessage.NewClient(path, address, ...)` no longer exist. Upstream wires iMessage through `cmd/msgvault/cmd/import_imessage.go`. |

Both files compile under the absent `merge_todo_sync_*` tag, so they are
de-facto disabled in normal builds.

---

## 8. CRITICAL: Vector search overlap with upstream PR #277

**This needs explicit human attention.**

The fork has a complete AI Archive Intelligence subsystem already shipped:

```
internal/embedding/{embed,search,hybrid}.go          # HEAD's vector pipeline
internal/store/{vectors,pipeline}.go                 # vec_messages, pipeline_runs tables
internal/store/store.go::InitVectorTable()           # bootstraps HEAD's tables
internal/ai/pipeline*.go                             # AI orchestration
cmd/msgvault/cmd/embed.go                            # 'msgvault embed' CLI (HEAD ver)
cmd/msgvault/cmd/search.go::runSemanticSearch        # --semantic flag
cmd/msgvault/cmd/search.go::runHybridSearch (HEAD)   # --hybrid flag (DELETED in this merge)
internal/store/schema.sql                            # vec_messages, pipeline_runs DDL
```

Upstream PR #277 ("semantic + hybrid vector search via local embeddings")
landed an entirely separate vector subsystem:

```
internal/vector/                                     # backend abstraction
internal/vector/sqlitevec/{backend,fused,ext,...}.go # sqlite-vec implementation
internal/vector/hybrid/{engine,filter,...}.go        # hybrid+RRF re-ranker
internal/vector/embed/                               # local embedding model
internal/vector/{config,env,errors,generations,stats}.go
internal/config/config.go::Vector                    # [vector] config section
cmd/msgvault/cmd/embed.go (upstream)                 # taken wholesale per spec
cmd/msgvault/cmd/search_vector.go                    # build-tagged hybrid CLI
```

**Both implementations claim the same surface area:**

- A "vector search" feature accessible from `msgvault search`
- A schema for storing message embeddings inside SQLite
- A `msgvault embed` command (we took upstream's, which obliterates HEAD's
  semantically-different command of the same name)
- A "hybrid" mode (`--hybrid` in HEAD; `--mode hybrid` in upstream; the merged
  CLI keeps both flags but routes both to upstream's implementation)

**What this merge did NOT do:** it did not try to merge the two
implementations, per the spec. Concretely:

1. `InitSchema` calls **HEAD's** `s.InitVectorTable()` (creates HEAD's
   `vec_messages` table tied to `pipeline_runs`).
2. Upstream's `internal/vector/sqlitevec/backend.go` will, when used,
   **also** try to manage its own vector tables (named differently — see
   that file). The fork now has two parallel vector schemas in the same
   database.
3. Upstream's `embed.go` (taken wholesale per spec) targets the upstream
   pipeline, not HEAD's `pipeline_runs` table. Running it will likely
   ignore or partially populate HEAD's tables.
4. The `--semantic` flag in `search.go` still calls HEAD's
   `runSemanticSearch` which goes through `internal/embedding/`.
5. `--hybrid` and `--mode=hybrid` both call upstream's `runHybridSearch`
   (in `search_vector.go`), which goes through `internal/vector/hybrid/`.

**Recommendation for the owner:** decide which subsystem is the canonical
one going forward. Plausibly, upstream's `internal/vector/` is more
maintainable long-term (Azure-OpenAI-free, local-first, hybrid via RRF),
but HEAD's `internal/embedding/` already shipped with real data attached
to it. The two share zero code and zero tables, so the cleanup is mostly
"delete one tree" plus a one-time migration of any embedded vectors that
must survive.

---

## 9. Critical issue blocking the build (must resolve before commit)

Both `mutecomm/go-sqlcipher/v4` (HEAD's encryption driver) and
`mattn/go-sqlite3` (added by upstream PR #277 and by upstream's iMessage
client) compile their own copy of the SQLite C amalgamation into the
binary. The Go linker rejects the duplicate symbols.

This is **the** thing that makes `go build` red. Suggested fixes,
ordered by effort:

1. **Drop `mattn/go-sqlite3` everywhere upstream introduced it.**
   `mutecomm/go-sqlcipher/v4` already registers a `"sqlite3"` driver name
   that is API-compatible with `mattn/go-sqlite3`. Most of the new
   `_ "github.com/mattn/go-sqlite3"` blank imports are just to register
   the driver and can simply be deleted. Files to touch:
   `cmd/msgvault/cmd/search_vector.go`,
   `internal/textimport/integration_test.go`,
   `internal/scheduler/scheduler_test.go`,
   `internal/imessage/client.go`,
   `internal/vector/sqlitevec/backend.go`,
   `internal/vector/sqlitevec/{ext,fused_test,backend_testhelpers_test}.go`,
   `internal/vector/hybrid/{filter_test,engine_test}.go`,
   plus any I missed (`grep -rn 'mattn/go-sqlite3' --include='*.go'`).
   Risk: `internal/imessage/client.go` (taken wholesale from upstream)
   may rely on a `mattn/go-sqlite3`-specific API like `RegisterFunc` or
   custom collations. Verify before deleting.
2. **Build tag `sqlite_vec` off and drop `asg017/sqlite-vec-go-bindings`
   for now.** This isolates the cause to the sqlcipher-vs-mattn fight,
   which option (1) addresses anyway.
3. **Last resort:** drop sqlcipher and lose at-rest encryption. Not
   recommended.

Once option (1) is done, the binary should link, and the test suite
will run.

---

## 10. Final state checklist

- [x] No conflict markers in any `*.go`/`*.md`/`go.{mod,sum}`/`Makefile`
- [x] Everything staged (`git add -A`)
- [x] Nothing committed (still in MERGE state)
- [x] `go build ./...` passing (with default `BUILD_TAGS := fts5`)
- [x] `go vet ./...` passing
- [x] `go test ./...` — 45 packages pass, 0 failures
- [x] `go fmt ./...` applied
- [x] Encryption code preserved (SQLCipher passphrase, AES-GCM tokens, advisory lock)
- [x] AI Archive Intelligence code preserved (vec_messages, pipeline_runs, embedding/)
- [x] Upstream-only features preserved (Dialect, loggedDB, OpenReadOnly, structured logging, FTS rebuild, IMAP greeting/date filter, M365/iMessage/gvoice from upstream)

---

## Worktree path

`/Users/ryanstern/msgvault/.claude/worktrees/agent-a448c0e80c510157a`

`cd` into it, fix §9, then `git commit` when satisfied.
