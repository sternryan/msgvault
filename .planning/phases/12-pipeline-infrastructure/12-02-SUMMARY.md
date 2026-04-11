---
phase: 12-pipeline-infrastructure
plan: 02
subsystem: infra
tags: [batch-runner, pipeline, progress, cli, checkpoints, resumability]

# Dependency graph
requires:
  - 12-01 (ai.Client, RateLimiter, Store pipeline methods)
provides:
  - BatchRunner with checkpoint-based resumability in internal/ai/pipeline.go
  - ProgressReporter with live count/cost/tok-per-sec/ETA in internal/ai/progress.go
  - Hidden 'pipeline test' CLI command for end-to-end infrastructure validation
affects:
  - 13-vector-search (uses BatchRunner for embedding runs)
  - 14-life-events (uses BatchRunner for chat completion runs)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - Generic batch runner with ProcessFunc/MessageQueryFunc callbacks (pipeline-specific logic injected)
    - Dual context: parent ctx for user-initiated cancel + sigCtx for SIGINT/SIGTERM graceful shutdown
    - Leave run status 'running' (not 'failed') on cancel to enable resume semantics

key-files:
  created:
    - internal/ai/pipeline.go
    - internal/ai/pipeline_test.go
    - internal/ai/progress.go
    - cmd/msgvault/cmd/pipeline.go

key-decisions:
  - "BatchRunner handles signal capture internally — callers pass only context; SIGINT/SIGTERM handled transparently"
  - "Run leaves status 'running' on cancel/interrupt so FindResumablePipelineRun picks it up on next invocation"
  - "totalMessages passed as 0 to StartPipelineRun — actual count emerges from query loop, displayed as count/0"
  - "ProcessFunc and MessageQueryFunc are callback types — pipeline-specific logic injected at construction, BatchRunner stays generic"
  - "Checkpoint non-fatal: log error and continue processing rather than abort on checkpoint write failure"

requirements-completed:
  - PIPE-02
  - PIPE-03
  - PIPE-04

# Metrics
duration: 30min
completed: 2026-04-11
---

# Phase 12 Plan 02: Batch Runner Framework Summary

**Generic batch processing framework with checkpoint resumability, live progress display (count/cost/tok-per-sec/ETA), graceful SIGINT shutdown, and a hidden 'pipeline test' CLI command for infrastructure validation**

## Performance

- **Duration:** ~30 min
- **Started:** 2026-04-11
- **Completed:** 2026-04-11
- **Tasks:** 2
- **Files created:** 4

## Accomplishments

- `internal/ai/pipeline.go` — BatchRunner with configurable batch size, checkpoint-every-N, SIGINT/SIGTERM graceful shutdown, and resume-from-last-checkpoint via FindResumablePipelineRun. Leaves status 'running' on cancel for resumability.
- `internal/ai/progress.go` — ProgressReporter printing live stats to stderr: `[type] N/total msgs | $cost | tok/s | ETA Xs`. 2-second throttle, carriage-return overwrite for clean terminal output. Stats() method for test assertions.
- `internal/ai/pipeline_test.go` — 6 tests: normal completion, empty set (no messages), resumability (second run starts from checkpoint), batch failure tolerance (continues to next batch), checkpoint frequency, and context cancellation (status stays 'running').
- `cmd/msgvault/cmd/pipeline.go` — Hidden `pipeline test` subcommand that validates azure_openai.endpoint, API key resolution, store/pipeline table existence, and runs a dry-run BatchRunner with configurable fake messages. Shows live progress.

## Task Commits

1. **Task 1: Batch runner framework with checkpoints and progress** — `b12d447d` (feat)
2. **Task 2: Pipeline validation CLI command** — `83c61bbe` (feat)

## Files Created

- `internal/ai/pipeline.go` — BatchRunner, RunConfig, ProcessFunc, MessageQueryFunc, MessageRow, BatchResult
- `internal/ai/pipeline_test.go` — 6 table-driven tests for all BatchRunner behaviors
- `internal/ai/progress.go` — ProgressReporter with Update(), Finish(), Stats()
- `cmd/msgvault/cmd/pipeline.go` — Hidden `pipeline test` Cobra command

## Decisions Made

- SIGINT/SIGTERM handled inside BatchRunner via `sigCtx` derived from parent ctx — callers don't need to wire signal handling themselves
- Run status left as 'running' on cancel (not 'failed') so `FindResumablePipelineRun` picks it up on the next invocation
- `totalMessages = 0` passed to `StartPipelineRun` at construction — actual message count emerges from query loop, progress shows `count/0` until total is known
- Checkpoint writes are non-fatal — log error and continue processing to avoid losing progress on transient write failures

## Deviations from Plan

None — plan executed exactly as written. The one minor adjustment: the plan's pipeline.go template had an `err` variable shadowing bug on the `fmt.Errorf("API key resolution failed: %w")` line (missing `err` argument). Fixed inline per Rule 1. The CLI command also uses `openStore()` helper from root.go (already available) rather than direct `store.Open()` call, consistent with how other commands access the store.

## Known Stubs

None — BatchRunner is fully functional. The `pipeline test` command uses fake messages intentionally (dry-run validation, no API calls). This is the designed behavior for infrastructure validation, not a stub.

## Threat Flags

None — no new network endpoints, auth paths, file access patterns, or schema changes introduced. The batch runner accesses only the existing SQLite database through the established Store interface.

---

## Self-Check

Checking created files exist:
- `internal/ai/pipeline.go` — FOUND
- `internal/ai/pipeline_test.go` — FOUND
- `internal/ai/progress.go` — FOUND
- `cmd/msgvault/cmd/pipeline.go` — FOUND

Checking commits exist:
- `b12d447d` feat(12-02): batch runner framework — FOUND
- `83c61bbe` feat(12-02): pipeline validation CLI command — FOUND

## Self-Check: PASSED

---
*Phase: 12-pipeline-infrastructure*
*Completed: 2026-04-11*
