---
phase: 12-pipeline-infrastructure
plan: 01
subsystem: infra
tags: [azure-openai, rate-limiter, sqlite, config, pipeline]

# Dependency graph
requires: []
provides:
  - AzureOpenAIConfig struct in internal/config with env/file key resolution
  - internal/ai package: Azure OpenAI HTTP client (Embedding + ChatCompletion)
  - internal/ai package: dual TPM/RPM token-bucket rate limiter
  - pipeline_runs and pipeline_checkpoints tables in SQLite schema
  - Store CRUD methods for pipeline run tracking and resumability
affects:
  - 12-02 (batch runner depends on ai.Client and Store pipeline methods)
  - 13-vector-search (embedding client)
  - 14-life-events (chat completion client)

# Tech tracking
tech-stack:
  added:
    - internal/ai package (new, no external deps — stdlib only)
  patterns:
    - Dual token-bucket rate limiter (TPM + RPM) with context cancellation
    - API key resolution from env var or file (never config.toml)
    - Exponential backoff retry with no-retry on 4xx client errors
    - Pipeline run/checkpoint CRUD mirroring sync_runs/sync_checkpoints pattern

key-files:
  created:
    - internal/ai/client.go
    - internal/ai/client_test.go
    - internal/ai/ratelimit.go
    - internal/ai/ratelimit_test.go
    - internal/store/pipeline.go
    - internal/store/pipeline_test.go
  modified:
    - internal/config/config.go
    - internal/store/schema.sql

key-decisions:
  - "API key resolved at client construction time (not per-request) — fail fast on misconfiguration"
  - "Dual token buckets (TPM + RPM) using stdlib sync.Mutex — no external rate-limit library"
  - "RecordActualTokens() refunds over-estimated tokens after response received"
  - "pipeline_runs status values mirror sync_runs: running/completed/failed/cancelled"
  - "api-key header used (not Authorization Bearer) per Azure OpenAI REST spec"
  - "Zero TPM/RPM limits = unlimited mode — safe default for dev/testing"

patterns-established:
  - "Store pipeline methods: StartPipelineRun / UpdatePipelineCheckpoint / CompletePipelineRun / FailPipelineRun (mirrors StartSync / UpdateSyncCheckpoint / CompleteSync / FailSync)"
  - "ai.Client options pattern: WithLogger / WithRateLimiter for testability"
  - "Rate limiter tryConsume returns 0 on success, duration to wait on failure"

requirements-completed:
  - PIPE-01
  - PIPE-04

# Metrics
duration: 22min
completed: 2026-04-11
---

# Phase 12 Plan 01: Pipeline Infrastructure Summary

**Azure OpenAI client with dual TPM/RPM rate limiter, AzureOpenAIConfig in Config struct, and pipeline_runs/pipeline_checkpoints schema with Store CRUD methods**

## Performance

- **Duration:** ~22 min
- **Started:** 2026-04-11T21:45:00Z
- **Completed:** 2026-04-11T21:51:23Z
- **Tasks:** 2
- **Files modified:** 8 (2 modified, 6 created)

## Accomplishments

- AzureOpenAIConfig added to Config struct with endpoint, api_key_env, api_key_file, deployments, tpm_limit, rpm_limit — API key never stored in config.toml
- New `internal/ai` package (stdlib only, no new dependencies) with Azure OpenAI HTTP client supporting Embedding and ChatCompletion endpoints, api-key auth header, and exponential backoff retry on 429/5xx
- Dual TPM/RPM token-bucket rate limiter with RecordActualTokens() for post-response token reconciliation
- pipeline_runs and pipeline_checkpoints tables added to schema.sql, mirroring the sync_runs/sync_checkpoints resumability pattern
- 23 tests passing across internal/ai and internal/store (7 pipeline store tests, 16 AI client/ratelimit tests)

## Task Commits

Each task was committed atomically:

1. **Task 1: Config, schema, and Store pipeline methods** - `59b2cae2` (feat)
2. **Task 2: Azure OpenAI HTTP client and token-bucket rate limiter** - `59360449` (feat)

## Files Created/Modified

- `internal/config/config.go` - Added AzureOpenAIConfig struct, AzureOpenAI field to Config, ResolveAPIKey(), DeploymentName(), expandPath for api_key_file
- `internal/store/schema.sql` - Added pipeline_runs and pipeline_checkpoints tables, two indexes
- `internal/store/pipeline.go` - Store CRUD methods: StartPipelineRun, UpdatePipelineCheckpoint, CompletePipelineRun, FailPipelineRun, GetPipelineCheckpoint, FindResumablePipelineRun
- `internal/store/pipeline_test.go` - 7 table-driven tests for all pipeline Store methods
- `internal/ai/ratelimit.go` - Dual token-bucket rate limiter (TPM + RPM), Wait(), RecordActualTokens()
- `internal/ai/ratelimit_test.go` - 8 tests covering unlimited mode, RPM blocking, TPM exhaustion, token refunds, context cancellation
- `internal/ai/client.go` - Azure OpenAI REST client: NewClient, Embedding, ChatCompletion, retry with backoff
- `internal/ai/client_test.go` - 8 tests using httptest server: endpoint validation, api-key header, URL format, retry on 429/500, no-retry on 400

## Decisions Made

- API key resolved at NewClient() construction time (fail fast on misconfiguration, not per-request)
- Used stdlib sync.Mutex + time.After for rate limiter — no external rate-limit library needed
- Zero TPM/RPM limits mean unlimited (safe default for development, explicit opt-in to limits via config.toml)
- api-key header (not Authorization Bearer) per Azure OpenAI REST API specification
- RecordActualTokens() allows post-response correction of token estimates, preventing both over-throttling and under-accounting

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

To use the Azure OpenAI client, add to `~/.msgvault/config.toml`:

```toml
[azure_openai]
endpoint = "https://your-instance.openai.azure.com"
api_key_env = "AZURE_OPENAI_API_KEY"  # or use api_key_file
tpm_limit = 240000  # tokens per minute (0 = unlimited)
rpm_limit = 1440    # requests per minute (0 = unlimited)

[azure_openai.deployments]
"text-embedding" = "text-embedding-3-small"
"gpt-4o-mini" = "gpt-4o-mini"
```

Set the API key: `export AZURE_OPENAI_API_KEY=your-key-here`

## Next Phase Readiness

- Plan 02 (batch runner) can import `internal/ai` and use `store.StartPipelineRun` / `store.UpdatePipelineCheckpoint` immediately
- Rate limiter is pre-wired in the client; Plan 02 just calls `client.Embedding()` / `client.ChatCompletion()`
- No blockers

---
*Phase: 12-pipeline-infrastructure*
*Completed: 2026-04-11*
