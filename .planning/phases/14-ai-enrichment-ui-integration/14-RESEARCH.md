# Phase 14: AI Enrichment & UI Integration - Research

**Researched:** 2026-04-11
**Domain:** Azure OpenAI GPT-4o-mini chat completions, structured JSON output, SQLite schema extension, TUI ViewType extension, Templ/HTMX web UI patterns
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**AI Categorization Strategy**
- Single LLM call per message with subject+snippet — cheapest approach
- Exactly one primary category per message
- AI categories stored as labels with label_type='auto' in existing labels table — reuses all existing label UI/filtering/aggregation code in both TUI and web
- Fixed 8 categories: finance, travel, legal, health, shopping, newsletters, personal, work

**Life Events & Entity Extraction**
- Combined single LLM call returns category + events + entities — halves API cost vs separate pipeline runs
- Life events extracted as structured JSON with date/type/description/source_message_id per LifeVault format spec
- New `entities` table with columns: id, message_id (FK), entity_type (person/company/date/amount), value, normalized_value, context
- CLI command `msgvault export-timeline` producing JSON file with `[{date, type, description, source_message_id}]`

**UI Integration**
- Web UI: Dropdown filter on messages page — same pattern as existing account filter, populated from labels where label_type='auto'
- TUI: Extend existing Tab-cycle views — add "AI Categories" as new view alongside Senders/Labels/Time, reuses drill-down pattern
- Web UI: New "Entities" page linked from nav — searchable table with type filter, click entity to see source messages
- Life timeline: Export-only (CLI) for v1.2 — LifeVault consumes the JSON. Web timeline visualization deferred to v1.3

### Claude's Discretion
- LLM prompt engineering for categorization accuracy and structured output parsing
- Entity normalization strategy (deduplication of "Google" vs "Google LLC" vs "Google Inc")
- Error handling for malformed LLM responses (retry, skip, fallback)
- Batch size tuning for combined extraction call (may differ from embedding batch size)

### Deferred Ideas (OUT OF SCOPE)
- Web timeline visualization (v1.3)
- Entity deduplication across accounts (v1.3 REL-02)
- Relationship graph from entity co-occurrence (v1.3 REL-01)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ENRICH-01 | User can auto-categorize all messages (finance, travel, legal, health, shopping, newsletters, personal, work) | BatchRunner + ChatCompletion ProcessFunc; query for uncategorized messages via NOT EXISTS on message_labels where label_type='auto' |
| ENRICH-02 | Categories stored as AI-generated labels in existing label system | EnsureLabel() with source_id=NULL + label_type='auto'; AddMessageLabel() insert into message_labels |
| ENRICH-03 | User can filter by AI categories in TUI and web UI | TUI: add ViewAICategories to ViewType enum + query.Aggregate case; Web: dropdown on /messages using filter.Label param (already wired) |
| ENRICH-04 | User can extract life events (jobs, moves, purchases, travel, milestones) from messages | Combined LLM call; new life_events table; Store methods to insert/query events |
| ENRICH-05 | Life events exported in LifeVault-compatible format (JSON with date, type, description, source_message_id) | New `msgvault export-timeline` Cobra command writing JSON array; source_message_id = messages.source_message_id |
| ENRICH-06 | User can extract entities (people, companies, dates, amounts) from message content | Combined LLM call returns entities array; new entities table with message_id FK |
| ENRICH-07 | Entities stored in searchable table with back-references to source messages | entities table + Store methods; web /entities page with ?type= and ?q= filters; HTMX partial loading |
</phase_requirements>

## Summary

Phase 14 adds AI-powered categorization, life event extraction, and entity extraction on top of the Phase 12 pipeline infrastructure already built. The architecture is well-established: a single `msgvault enrich` CLI command wires a new `categorization.ProcessFunc` into the existing `ai.BatchRunner`. Each batch calls `ai.Client.ChatCompletion()` once per message with a structured JSON prompt returning `{category, life_events, entities}`. Results fan out into three write paths: (1) insert into `labels`/`message_labels` tables for categories, (2) insert into a new `life_events` table, and (3) insert into a new `entities` table.

UI integration reuses existing patterns almost entirely. The TUI adds `ViewAICategories` as a new constant to the `query.ViewType` iota — the `cycleViewType()` function iterates `ViewTypeCount` automatically, so adding one constant before `ViewTypeCount` is sufficient to add a new tab. The web messages page adds a category dropdown that uses the already-wired `filter.Label` query param (no new query logic needed). The Entities page is a new `/entities` route with Templ template + HTMX partial, following the same pattern as the existing `/aggregate` page.

**Primary recommendation:** Implement in four focused tasks — (1) schema migration + store methods, (2) enrichment pipeline (ProcessFunc + enrich CLI command), (3) export-timeline CLI command, (4) web + TUI UI integration.

## Standard Stack

### Core (all already in go.mod — no new dependencies required)
| Library | Purpose | Already Used |
|---------|---------|-------------|
| `internal/ai.Client.ChatCompletion()` | GPT-4o-mini structured JSON output | Yes — Phase 12 |
| `internal/ai.BatchRunner` | Checkpoint-based batch processing | Yes — Phase 12/13 |
| `encoding/json` | Unmarshal LLM structured output | Yes — stdlib |
| `github.com/a-h/templ` | Server-rendered HTML templates | Yes — existing web UI |
| HTMX (cdn-pinned via static/) | Partial page swaps | Yes — existing web UI |
| `github.com/go-chi/chi/v5` | HTTP routing | Yes — existing web UI |

### No New Dependencies
All required libraries are already in the project. This phase is purely additive — new tables, new Go packages, new CLI commands, new templates.

## Architecture Patterns

### Recommended Project Structure
```
internal/enrichment/          # New package (mirrors internal/embedding/)
├── pipeline.go               # ProcessFunc + RunEnrichPipeline()
├── pipeline_test.go
└── prompt.go                 # Prompt construction + JSON parsing

internal/store/
├── schema.sql                # Add life_events + entities tables + indexes
├── enrichment.go             # New store methods: InsertLifeEvent, InsertEntity, GetAutoLabel, etc.

cmd/msgvault/cmd/
├── enrich.go                 # `msgvault enrich` Cobra command
└── export_timeline.go        # `msgvault export-timeline` Cobra command

internal/web/
├── handlers_entities.go      # New: GET /entities handler
└── templates/
    ├── entities.templ         # New: Entities page template
    └── entities_templ.go     # Generated

internal/query/
└── models.go                 # Add ViewAICategories to ViewType iota
```

### Pattern 1: Combined LLM ProcessFunc

The embedding pipeline (internal/embedding/embed.go) is the direct template. The categorization ProcessFunc receives a batch of MessageRows, calls ChatCompletion once per message (not batch — GPT-4o-mini structured output works best 1:1), and writes results.

```go
// Source: internal/embedding/embed.go (existing pattern)
// internal/enrichment/pipeline.go

type EnrichResult struct {
    Category   string       `json:"category"`
    LifeEvents []LifeEvent  `json:"life_events"`
    Entities   []Entity     `json:"entities"`
}

type LifeEvent struct {
    Date        string `json:"date"`        // YYYY-MM-DD or YYYY-MM or YYYY
    Type        string `json:"type"`        // job/move/purchase/travel/milestone
    Description string `json:"description"`
}

type Entity struct {
    Type  string `json:"type"`  // person/company/date/amount
    Value string `json:"value"`
}

func buildEnrichProcessFunc(client *ai.Client, s *store.Store, deployment string, logger *slog.Logger) ai.ProcessFunc {
    return func(ctx context.Context, messages []ai.MessageRow) (*ai.BatchResult, error) {
        result := &ai.BatchResult{}
        for _, msg := range messages {
            resp, err := client.ChatCompletion(ctx, deployment, buildEnrichRequest(msg))
            if err != nil {
                result.Failed++
                continue
            }
            enrichResult, err := parseEnrichResponse(resp)
            if err != nil {
                result.Failed++
                logger.Warn("malformed LLM response, skipping", "msg_id", msg.ID, "err", err)
                continue
            }
            if err := writeEnrichResults(s, msg.ID, enrichResult); err != nil {
                result.Failed++
                continue
            }
            result.Processed++
            result.TokensInput += int64(resp.Usage.PromptTokens)
            result.TokensOutput += int64(resp.Usage.CompletionTokens)
            result.CostUSD += costForTokens(resp.Usage)
            result.LastMessageID = msg.ID
        }
        return result, nil
    }
}
```

### Pattern 2: Auto-Label Storage

The `EnsureLabel()` pattern already handles idempotent label creation. For AI categories, source_id must be NULL (cross-account labels) and source_label_id is NULL (no Gmail ID). The existing UNIQUE constraint is `(source_id, name)` — so NULL source_id + name "finance" will need careful handling since SQLite treats NULL != NULL in UNIQUE constraints. Use `WHERE source_id IS NULL AND name = ?` for the lookup, not the UNIQUE index.

```go
// internal/store/enrichment.go
func (s *Store) GetOrCreateAutoLabel(name string) (int64, error) {
    var id int64
    err := s.db.QueryRow(
        `SELECT id FROM labels WHERE source_id IS NULL AND name = ? AND label_type = 'auto'`,
        name,
    ).Scan(&id)
    if err == nil {
        return id, nil
    }
    if err != sql.ErrNoRows {
        return 0, err
    }
    result, err := s.db.Exec(
        `INSERT INTO labels (source_id, source_label_id, name, label_type) VALUES (NULL, NULL, ?, 'auto')`,
        name,
    )
    if err != nil {
        return 0, err
    }
    return result.LastInsertId()
}

func (s *Store) AddMessageLabel(messageID, labelID int64) error {
    _, err := s.db.Exec(
        `INSERT OR IGNORE INTO message_labels (message_id, label_id) VALUES (?, ?)`,
        messageID, labelID,
    )
    return err
}
```

### Pattern 3: ViewAICategories in TUI

The TUI cycles through views using `cycleViewType()` which iterates `0..ViewTypeCount`. Adding a new ViewType before `ViewTypeCount` automatically includes it. The `Aggregate()` query method and `switch` statements in sqlite.go, duckdb.go, view.go, and keys.go all need the new case added.

```go
// internal/query/models.go — add before ViewTypeCount
const (
    ViewSenders ViewType = iota
    ViewSenderNames
    ViewRecipients
    ViewRecipientNames
    ViewDomains
    ViewLabels
    ViewTime
    ViewAICategories  // NEW: AI-generated category labels

    ViewTypeCount
)

// String() method — add case:
case ViewAICategories:
    return "AI Categories"
```

The aggregate query for ViewAICategories is identical to ViewLabels but filtered to label_type='auto':
```sql
-- sqlite.go Aggregate case ViewAICategories:
SELECT l.name AS key, COUNT(*) AS count, ...
FROM messages m
JOIN message_labels ml ON ml.message_id = m.id
JOIN labels l ON l.id = ml.label_id
WHERE l.label_type = 'auto'
GROUP BY l.name
ORDER BY count DESC
```

### Pattern 4: Web Category Dropdown

The messages page already receives `filter.Label` via `parseMessageFilter()` and passes it to the query engine. Adding a category dropdown is purely a template change — no new handler logic needed.

```go
// internal/web/templates/messages.templ addition
// New store method needed: GetAutoLabels() []string
// Passed from handler into MessagesPage() template
```

The handler `messagesList` needs to fetch auto-labels from the store and pass them to the template. The store query:
```sql
SELECT DISTINCT name FROM labels WHERE label_type = 'auto' ORDER BY name
```

### Pattern 5: Entities Page (new route)

Following the existing aggregate/search pattern: handler fetches from store, passes to Templ component, HTMX handles partial updates.

```go
// internal/web/server.go — add routes
r.Get("/entities", h.entitiesPage)
r.Get("/entities/partial", h.entitiesPartial)  // HTMX target

// internal/web/handlers_entities.go
func (h *handlers) entitiesPage(w http.ResponseWriter, r *http.Request) {
    entityType := r.URL.Query().Get("type")  // person/company/date/amount
    q := r.URL.Query().Get("q")
    // Query entities table, paginate
    // Render templates.EntitiesPage(entities, filter, total)
}
```

### Pattern 6: Export Timeline CLI

Following export_vault.go pattern — open store, query life_events, write JSON:

```go
// cmd/msgvault/cmd/export_timeline.go
type TimelineEntry struct {
    Date            string `json:"date"`
    Type            string `json:"type"`
    Description     string `json:"description"`
    SourceMessageID string `json:"source_message_id"`
}
```

### Pattern 7: LLM Prompt for Combined Extraction

The prompt must produce deterministic JSON. GPT-4o-mini reliably follows JSON-schema instructions. Key prompt design decisions (Claude's Discretion):

```
System: You are a personal email analyst. Extract structured information from emails.
        Always return valid JSON matching the schema exactly.

User: Email subject: {subject}
      Email preview: {snippet}

      Return JSON:
      {
        "category": "<one of: finance|travel|legal|health|shopping|newsletters|personal|work>",
        "life_events": [{"date":"YYYY-MM-DD","type":"<job|move|purchase|travel|milestone>","description":"..."}],
        "entities": [{"type":"<person|company|date|amount>","value":"..."}]
      }

      Rules:
      - category: exactly one, lowercase, from the allowed list only
      - life_events: empty array [] if none found; only extract significant life events
      - entities: empty array [] if none; extract named people, companies, specific dates, monetary amounts
      - Return ONLY the JSON object, no explanation
```

**Fallback handling for malformed responses:** Attempt JSON unmarshal; if it fails, try extracting JSON with a regex `\{.*\}` (dotall); if still fails, log warning and use `category="personal"` with empty arrays rather than failing the message.

### Anti-Patterns to Avoid

- **Batch ChatCompletion calls:** Unlike embeddings (which batch 100 at a time), chat completions work best 1 message per call. Batching prompts into one call produces inconsistent JSON and harder parsing.
- **Using EnsureLabel() for auto labels:** The existing `EnsureLabel()` requires both `source_id` and `source_label_id` — it doesn't handle NULL source_id correctly for the auto label case. Use the new `GetOrCreateAutoLabel()` instead.
- **Scanning message_bodies in the enrichment query:** CLAUDE.md explicitly forbids scanning `message_bodies` in list/aggregate queries. Use `messages.subject` + `messages.snippet` only (consistent with embedding pipeline).
- **Adding label_type filter to existing batchGetLabels:** Don't change existing label loading — it already loads all labels including auto ones. The AI category dropdown is a separate filter mechanism.
- **SELECT DISTINCT with JOINs:** CLAUDE.md mandates EXISTS subqueries instead of DISTINCT + JOINs for the "uncategorized messages" query.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead |
|---------|-------------|-------------|
| Checkpoint/resume | Custom state table | `ai.BatchRunner` + `store.pipeline_runs` (Phase 12) |
| Rate limiting | Custom token counter | `ai.RateLimiter` (Phase 12) |
| Progress display | Custom terminal output | `ai.ProgressReporter` (Phase 12) |
| Label storage | New labels system | Existing `labels`/`message_labels` + new `GetOrCreateAutoLabel()` |
| Category filtering | New query param | Existing `filter.Label` param already wired in all query paths |
| HTMX page rendering | Custom JS | Existing Templ + HTMX pattern in all web handlers |

## Schema Changes Required

The schema needs two new tables and migration-safe addition. The `InitSchema()` function uses `CREATE TABLE IF NOT EXISTS` so new tables can be added to schema.sql safely.

```sql
-- Add to internal/store/schema.sql

-- Life events extracted by AI pipeline
CREATE TABLE IF NOT EXISTS life_events (
    id INTEGER PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    event_date TEXT,          -- YYYY-MM-DD, YYYY-MM, or YYYY (partial dates OK)
    event_type TEXT NOT NULL, -- 'job', 'move', 'purchase', 'travel', 'milestone'
    description TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_life_events_message ON life_events(message_id);
CREATE INDEX IF NOT EXISTS idx_life_events_type ON life_events(event_type);
CREATE INDEX IF NOT EXISTS idx_life_events_date ON life_events(event_date);

-- Entities extracted by AI pipeline
CREATE TABLE IF NOT EXISTS entities (
    id INTEGER PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,     -- 'person', 'company', 'date', 'amount'
    value TEXT NOT NULL,           -- raw extracted value
    normalized_value TEXT,         -- normalized form (e.g. "Google" for "Google LLC")
    context TEXT,                  -- snippet of surrounding text (optional)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_entities_message ON entities(message_id);
CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(entity_type);
CREATE INDEX IF NOT EXISTS idx_entities_value ON entities(value);
CREATE INDEX IF NOT EXISTS idx_entities_normalized ON entities(normalized_value)
    WHERE normalized_value IS NOT NULL;
```

**Idempotency for enrichment pipeline:** The "uncategorized messages" query uses NOT EXISTS on message_labels filtered to auto labels:
```sql
SELECT m.id, m.subject, m.snippet
FROM messages m
WHERE m.id > ?
AND NOT EXISTS (
    SELECT 1 FROM message_labels ml
    JOIN labels l ON l.id = ml.label_id
    WHERE ml.message_id = m.id AND l.label_type = 'auto'
)
ORDER BY m.id ASC
LIMIT ?
```

## Common Pitfalls

### Pitfall 1: NULL UNIQUE Constraint in Labels Table
**What goes wrong:** The labels table has `UNIQUE(source_id, name)`. In SQLite, NULL != NULL, so two rows with `source_id IS NULL, name = 'finance'` can both be inserted without violating the constraint.
**Why it happens:** SQL NULL semantics differ from application-level uniqueness.
**How to avoid:** Use `GetOrCreateAutoLabel()` with explicit `WHERE source_id IS NULL AND name = ?` lookup before insert. Add a partial unique index for robustness: `CREATE UNIQUE INDEX IF NOT EXISTS idx_labels_auto_name ON labels(name) WHERE source_id IS NULL AND label_type = 'auto'`
**Warning signs:** Duplicate rows in labels table for the same category name.

### Pitfall 2: JSON Parsing Failures from LLM
**What goes wrong:** GPT-4o-mini occasionally returns JSON wrapped in markdown fences (```json ... ```) or with trailing commentary.
**Why it happens:** Despite system prompt instructions, the model sometimes adds explanation.
**How to avoid:** Strip markdown fences before JSON unmarshal. Regex extract `\{[^}]+\}` as fallback. Never fail the entire batch — log and skip individual messages with `result.Failed++`.
**Warning signs:** High failed count in pipeline progress output.

### Pitfall 3: Batch Size Too Large for Chat Completions
**What goes wrong:** Processing 100 messages per batch (same as embedding) means 100 sequential API calls per batch. At 10 QPS this takes 10 seconds per batch with no parallelism.
**Why it happens:** Embedding batches 100 texts in one API call; chat completions are one call per message.
**How to avoid:** Set `--batch-size` default to 20 for the enrich command (20 messages = 20 sequential calls per batch, then checkpoint). Consider adding concurrency within the ProcessFunc (goroutine pool of 3-5 workers within a batch). The BatchRunner's checkpoint happens between batches — within-batch concurrency is safe.
**Warning signs:** Very slow pipeline progress despite high TPM quota.

### Pitfall 4: ViewTypeCount Placement
**What goes wrong:** Adding `ViewAICategories` after `ViewTypeCount` instead of before it breaks the cycleViewType() loop.
**Why it happens:** The iota sentinel must always be last.
**How to avoid:** Always insert new ViewType constants immediately before `ViewTypeCount`.
**Warning signs:** TUI Tab cycling skips AI Categories view or panics on modulo.

### Pitfall 5: Messages Page Template Needs Auto-Labels List
**What goes wrong:** The category dropdown needs the list of existing auto labels, but the current `messagesList` handler doesn't fetch labels.
**Why it happens:** Current handler only fetches messages and total stats.
**How to avoid:** Add `GetAutoLabels(ctx) ([]string, error)` store method. Call it in `messagesList` handler. Pass the list to `MessagesPage()` template. The template renders an empty dropdown gracefully if no labels exist yet (no messages categorized).

### Pitfall 6: export-timeline source_message_id vs internal ID
**What goes wrong:** Using internal database `messages.id` (integer PK) as `source_message_id` in the LifeVault export.
**Why it happens:** Internal IDs are meaningless outside msgvault and change between databases.
**How to avoid:** Always use `messages.source_message_id` (the Gmail message ID) as `source_message_id` in the exported JSON. The life_events table stores `message_id` (FK to internal ID) — the export query JOINs messages to get `source_message_id`.

## Code Examples

### Enrichment ProcessFunc (primary pattern)
```go
// Source: internal/embedding/embed.go lines 43-80 (verified in codebase)
// Pattern: createQueryFunc + processFunc + RunBatchPipeline

func RunEnrichPipeline(ctx context.Context, client *ai.Client, s *store.Store, logger *slog.Logger, batchSize int) error {
    runner := ai.NewBatchRunner(ai.RunConfig{
        PipelineType:  "categorize",
        BatchSize:     batchSize,
        Store:         s,
        Logger:        logger,
        QueryMessages: createEnrichQueryFunc(s),
        Process:       buildEnrichProcessFunc(client, s, "chat", logger),
    })
    return runner.Run(ctx)
}
```

### Auto-Label Idempotency Query
```sql
-- Source: verified against schema.sql (labels + message_labels tables)
-- Find messages not yet categorized
SELECT m.id, m.subject, m.snippet
FROM messages m
WHERE m.id > ?
AND NOT EXISTS (
    SELECT 1 FROM message_labels ml
    JOIN labels l ON l.id = ml.label_id
    WHERE ml.message_id = m.id AND l.label_type = 'auto'
)
ORDER BY m.id ASC
LIMIT ?
```

### ViewAICategories Aggregate SQL (SQLite engine)
```sql
-- Source: verified against sqlite.go ViewLabels case (lines 405-411)
-- Add case ViewAICategories in sqlite.go Aggregate():
SELECT l.name AS key,
       COUNT(*) AS count,
       SUM(COALESCE(m.size_estimate, 0)) AS total_size,
       0 AS attachment_size,
       SUM(CASE WHEN m.has_attachments THEN m.attachment_count ELSE 0 END) AS attachment_count,
       COUNT(*) OVER() AS total_unique
FROM messages m
WHERE EXISTS (
    SELECT 1 FROM message_labels ml
    JOIN labels l2 ON l2.id = ml.label_id
    WHERE ml.message_id = m.id AND l2.label_type = 'auto' AND l2.name = l.name
)
-- ... (join labels to get the grouping key)
```

### Timeline Export JSON Structure
```go
// LifeVault-compatible format per REQUIREMENTS.md ENRICH-05
type TimelineEntry struct {
    Date            string `json:"date"`
    Type            string `json:"type"`
    Description     string `json:"description"`
    SourceMessageID string `json:"source_message_id"`
}
// Written as JSON array to --output file (or stdout if not specified)
```

## Environment Availability

Step 2.6: No new external dependencies. Azure OpenAI is already configured via config.toml (required for Phase 12/13). Go build environment already supports CGO_ENABLED=1.

| Dependency | Required By | Available | Notes |
|------------|------------|-----------|-------|
| Azure OpenAI (GPT-4o-mini) | enrich pipeline | Configured in Phase 12 | Same client as embedding |
| SQLite (go-sqlite3) | entities/life_events tables | Yes — existing store | No schema migration needed beyond IF NOT EXISTS |
| Templ codegen | entities.templ | Yes — existing toolchain | `make build` auto-generates |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + table-driven tests |
| Config file | none — `go test ./...` |
| Quick run command | `go test ./internal/enrichment/... ./internal/store/... -run TestEnrich` |
| Full suite command | `make test` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ENRICH-01 | Uncategorized query returns only messages without auto labels | unit | `go test ./internal/enrichment/... -run TestQueryUncategorized` | ❌ Wave 0 |
| ENRICH-02 | GetOrCreateAutoLabel is idempotent (second call returns same ID) | unit | `go test ./internal/store/... -run TestGetOrCreateAutoLabel` | ❌ Wave 0 |
| ENRICH-03 | filter.Label="finance" returns only finance-labeled messages | unit | `go test ./internal/query/... -run TestLabelFilter` | Partial — existing TestLabels |
| ENRICH-04 | InsertLifeEvent stores correct fields | unit | `go test ./internal/store/... -run TestLifeEvents` | ❌ Wave 0 |
| ENRICH-05 | export-timeline JSON output matches LifeVault schema | unit | `go test ./cmd/msgvault/cmd/... -run TestExportTimeline` | ❌ Wave 0 |
| ENRICH-06 | parseEnrichResponse correctly parses combined LLM JSON | unit | `go test ./internal/enrichment/... -run TestParseEnrichResponse` | ❌ Wave 0 |
| ENRICH-07 | /entities?type=person returns only person entities | integration | `go test ./internal/web/... -run TestEntitiesHandler` | ❌ Wave 0 |

### Wave 0 Gaps
- [ ] `internal/enrichment/pipeline_test.go` — covers ENRICH-01, ENRICH-06
- [ ] `internal/store/enrichment_test.go` — covers ENRICH-02, ENRICH-04
- [ ] `internal/web/handlers_entities_test.go` — covers ENRICH-07
- [ ] `cmd/msgvault/cmd/export_timeline_test.go` — covers ENRICH-05

## Security Domain

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation | yes | LLM response JSON strictly validated before DB write; category whitelist enforced in Go, not prompt alone |
| V4 Access Control | no | Local CLI tool, no auth layer |
| V6 Cryptography | no | No new cryptographic operations |

### Known Threat Patterns
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| LLM prompt injection via message subject/snippet | Tampering | Role separation (system vs user messages); category validated against fixed enum in Go code, not trusted from LLM output |
| Invalid category string stored in labels | Tampering | Validate `enrichResult.Category` against `allowedCategories` map before writing to DB — reject and log if unexpected value |
| Malformed entities causing SQL injection | Tampering | Parameterized queries in all store methods (existing pattern throughout codebase) |

**Category validation (CRITICAL):**
```go
var allowedCategories = map[string]bool{
    "finance": true, "travel": true, "legal": true, "health": true,
    "shopping": true, "newsletters": true, "personal": true, "work": true,
}
// After parsing LLM response:
if !allowedCategories[enrichResult.Category] {
    logger.Warn("invalid category from LLM, defaulting to personal", "got", enrichResult.Category)
    enrichResult.Category = "personal"
}
```

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | DuckDB Aggregate for ViewAICategories follows same pattern as ViewLabels | Architecture Patterns | Low — DuckDB and SQLite engines both need cases; if DuckDB has different label join structure, SQL needs adjustment |
| A2 | GPT-4o-mini deployment name in config.toml will be "chat" (following "text-embedding" convention) | Standard Stack | Low — deployment name is configurable via config.toml; enrich command can use same flag pattern as embed |
| A3 | Within-ProcessFunc concurrency (goroutine pool) is safe with BatchRunner | Architecture Patterns | Low — BatchRunner only checkpoints between batches; within-batch parallelism is independent of checkpoint logic |

**All critical claims verified** against the codebase in this session. Schema, store patterns, ViewType enum, label filter wiring, and Templ/HTMX patterns are all VERIFIED.

## Open Questions

1. **Deployment name for GPT-4o-mini in config.toml**
   - What we know: Phase 12 established `text-embedding` as the deployment name for embeddings
   - What's unclear: No existing config shows what the user named the GPT-4o-mini deployment
   - Recommendation: Use `"chat"` as the logical name (configurable via `--deployment` flag); document in command help

2. **Within-batch parallelism for chat completions**
   - What we know: 100-message batches with sequential API calls = ~10 seconds/batch at 10 QPS
   - What's unclear: Whether user wants to add concurrency or keep sequential
   - Recommendation: Implement sequential first (simpler, safer), add `--concurrency N` flag (default 1) that ProcessFunc can use to run N goroutines within a batch

3. **Partial unique index for auto labels**
   - What we know: SQLite NULL != NULL means UNIQUE(source_id, name) doesn't prevent duplicate auto labels
   - What's unclear: Whether schema migration should add the partial index or handle it purely in application code
   - Recommendation: Add `CREATE UNIQUE INDEX IF NOT EXISTS idx_labels_auto_name ON labels(name) WHERE source_id IS NULL AND label_type = 'auto'` to schema.sql — belt-and-suspenders is correct here

## Sources

### Primary (HIGH confidence — verified in codebase)
- `internal/ai/pipeline.go` — BatchRunner, RunConfig, ProcessFunc interface (lines 1-220)
- `internal/ai/client.go` — ChatCompletion method, ChatRequest/ChatResponse types
- `internal/store/schema.sql` — labels table, label_type='auto' column, pipeline_runs/checkpoints tables
- `internal/store/messages.go` — EnsureLabel() pattern (lines 479-503)
- `internal/query/models.go` — ViewType iota, ViewTypeCount sentinel (lines 84-97)
- `internal/tui/keys.go` — cycleViewType() function (lines 1093-1100)
- `internal/web/params.go` — parseMessageFilter(), filter.Label wiring (line 195)
- `internal/web/server.go` — route registration pattern, ServerOption pattern
- `internal/web/templates/layout.templ` — nav structure, HTMX pattern
- `internal/embedding/embed.go` — ProcessFunc template (lines 1-80)
- `go.mod` — confirms no new dependencies needed

### Secondary (MEDIUM confidence)
- [ASSUMED] GPT-4o-mini JSON output reliability — based on Azure OpenAI documentation patterns and training knowledge; actual prompt may need iteration

## Metadata

**Confidence breakdown:**
- Standard Stack: HIGH — all libraries already in use, verified in go.mod and internal packages
- Schema: HIGH — verified existing patterns; new tables follow established CREATE IF NOT EXISTS convention
- Architecture: HIGH — all patterns traced to existing code in this session
- LLM Prompt: MEDIUM — prompt engineering effectiveness requires empirical testing; category whitelist validation mitigates incorrect output

**Research date:** 2026-04-11
**Valid until:** 2026-05-11 (stable Go codebase; Azure OpenAI API version pinned at 2024-10-21)
