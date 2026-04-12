---
phase: 14-ai-enrichment-ui-integration
reviewed: 2026-04-11T00:00:00Z
depth: standard
files_reviewed: 18
files_reviewed_list:
  - cmd/msgvault/cmd/enrich.go
  - cmd/msgvault/cmd/export_timeline.go
  - internal/enrichment/pipeline.go
  - internal/enrichment/prompt.go
  - internal/store/enrichment.go
  - internal/store/schema.sql
  - internal/query/models.go
  - internal/query/sqlite.go
  - internal/query/duckdb.go
  - internal/tui/model.go
  - internal/tui/view.go
  - internal/tui/keys.go
  - internal/web/handlers_entities.go
  - internal/web/handlers_messages.go
  - internal/web/server.go
  - internal/web/templates/entities.templ
  - internal/web/templates/messages.templ
  - internal/web/templates/layout.templ
findings:
  critical: 0
  warning: 2
  info: 3
  total: 5
status: issues_found
---

# Phase 14: Code Review Report

**Reviewed:** 2026-04-11
**Depth:** standard
**Files Reviewed:** 18
**Status:** issues_found

## Summary

This phase adds the AI enrichment pipeline (categorization, life event extraction, entity extraction via Azure OpenAI GPT-4o-mini), the `export-timeline` CLI command, the entities web page, and integration of AI category labels into the TUI and message list. The architecture is clean: LLM output is validated against an allowlist before storage, queries use parameterized arguments throughout, and the enrichment pipeline is idempotent and resumable.

Two warnings were found: a bug where the `--deployment` CLI flag is silently ignored (the hardcoded value `"chat"` is always used), and an unescaped LIKE pattern that allows users to use `%` and `_` as wildcards in entity search. Three info-level items cover a deprecated stdlib function, a minor off-by-one display edge, and commented-out dead code in the enrichment dry-run cost model.

## Warnings

### WR-01: `--deployment` flag silently ignored — hardcoded "chat" always used

**File:** `cmd/msgvault/cmd/enrich.go:91` and `internal/enrichment/pipeline.go:221`

**Issue:** The `--deployment` flag is accepted, logged, and documented, but `RunEnrichPipeline` does not accept a deployment parameter. The call at `pipeline.go:221` hardcodes `"chat"`:

```go
Process: buildEnrichProcessFunc(client, s, "chat", logger),
```

A user who configures a second deployment (e.g., `gpt-4o`) and passes `--deployment gpt-4o` will silently have the flag ignored. No error is returned.

**Fix:** Either pass `enrichDeployment` through to `RunEnrichPipeline`, or remove the flag if only one deployment name is intended to be supported:

```go
// In internal/enrichment/pipeline.go
func RunEnrichPipeline(ctx context.Context, client *ai.Client, s *store.Store, logger *slog.Logger, batchSize int, deployment string) error {
    ...
    Process: buildEnrichProcessFunc(client, s, deployment, logger),
```

```go
// In cmd/msgvault/cmd/enrich.go
if err := enrichment.RunEnrichPipeline(cmd.Context(), aiClient, s, slogLogger, enrichBatchSize, enrichDeployment); err != nil {
```

---

### WR-02: Entity search query allows unescaped LIKE wildcards (`%`, `_`)

**File:** `internal/store/enrichment.go:229`

**Issue:** The `q` parameter from the web request is inserted directly into a LIKE pattern without escaping:

```go
like := "%" + searchQuery + "%"
args = append(args, like, like)
```

This is not a SQL injection risk (parameterized queries are used), but a user who types `%` or `_` in the search box will get unintended wildcard semantics. For example, searching for `%` matches every row; `_` matches any single character. For a personal email archive this is low severity, but it can cause confusing results and violates the principle of literal string matching for search inputs.

**Fix:** Escape LIKE special characters before constructing the pattern. A helper already exists in the query package:

```go
// internal/store/enrichment.go
func escapeLike(s string) string {
    r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
    return r.Replace(s)
}

// Then:
like := "%" + escapeLike(searchQuery) + "%"
// And add ESCAPE '\' to the SQL condition:
conditions = append(conditions, "(value LIKE ? ESCAPE '\\' OR normalized_value LIKE ? ESCAPE '\\')")
```

---

## Info

### IN-01: `strings.Title` is deprecated — use `golang.org/x/text/cases`

**File:** `internal/web/templates/messages.templ:29`

**Issue:** `strings.Title` is deprecated since Go 1.18 and does not handle Unicode correctly. The compiler will emit a deprecation warning. The generated `messages_templ.go` file also contains the call.

**Fix:**
```go
import "golang.org/x/text/cases"
import "golang.org/x/text/language"

caser := cases.Title(language.English)
// Then in the template helper:
caser.String(label)
```

Alternatively, since category labels are a fixed set of ASCII lowercase strings, a simple `cases.Title(language.Und).String(label)` or a hand-written `strings.ToUpper(label[:1]) + label[1:]` guard is sufficient and avoids the dependency.

---

### IN-02: Off-by-one display when `entities` result set is empty but `offset > 0`

**File:** `internal/web/templates/entities.templ:110`

**Issue:** The "Showing N–M of T entities" line renders `offset+1` as the start even when the result slice is empty (which can happen if a user manually crafts a URL with a high `offset`). The `EntitiesTableContent` template guards `len(entities) == 0` at the top and renders an empty-state message, so the table meta line is never reached in practice — but the `EntitiesPagination` component at line 158 renders `offset+1` unconditionally when `total > 0`. If `offset >= total`, the "Prev" link appears but "Next" does not, and the range displayed would be wrong (e.g., "51–50 of 50 entities").

This is a display-only issue: the data returned is correct (the DB returns an empty slice), but the label is misleading.

**Fix:** Clamp the display start to `min(offset+1, total)`, or add a guard in `EntitiesPagination` that only renders when `offset < total`.

---

### IN-03: `--deployment` flag printed in progress message but value never takes effect (related to WR-01)

**File:** `cmd/msgvault/cmd/enrich.go:88-89`

**Issue:** The status line `fmt.Fprintf(os.Stderr, "Starting enrichment pipeline (batch-size=%d, deployment=%s)...\n", enrichBatchSize, enrichDeployment)` prints the user-supplied deployment name, giving false confidence that the value is in use. This is a side effect of WR-01 and will be resolved by the same fix.

---

_Reviewed: 2026-04-11_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
