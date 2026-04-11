package ai

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wesm/msgvault/internal/store"
)

// MessageRow represents a message to be processed by the pipeline.
// Minimal fields — pipelines query what they need.
type MessageRow struct {
	ID       int64
	Subject  string
	Snippet  string
	BodyText string // only loaded if pipeline needs it
}

// BatchResult reports what happened for a batch of messages.
type BatchResult struct {
	Processed     int64
	Skipped       int64
	Failed        int64
	TokensInput   int64
	TokensOutput  int64
	CostUSD       float64
	LastMessageID int64
}

// ProcessFunc processes a batch of messages. Implementations are pipeline-specific
// (embedding, categorization, etc.). Must be idempotent for checkpoint safety.
type ProcessFunc func(ctx context.Context, messages []MessageRow) (*BatchResult, error)

// MessageQueryFunc returns messages to process, ordered by ID ascending,
// starting after afterID, limited to batchSize.
type MessageQueryFunc func(afterID int64, batchSize int) ([]MessageRow, error)

// RunConfig configures a pipeline run.
type RunConfig struct {
	PipelineType    string       // e.g. "embedding", "categorize"
	BatchSize       int          // messages per batch (default 100)
	CheckpointEvery int          // checkpoint after N batches (default 10)
	Store           *store.Store // for pipeline run/checkpoint persistence
	Logger          *slog.Logger
	QueryMessages   MessageQueryFunc       // fetches next batch of messages
	Process         ProcessFunc            // processes a batch
	FilterCriteria  map[string]interface{} // stored in pipeline_runs for debugging
}

// BatchRunner orchestrates batch processing with checkpoints and progress.
type BatchRunner struct {
	cfg      RunConfig
	progress *ProgressReporter
}

// NewBatchRunner creates a batch runner.
func NewBatchRunner(cfg RunConfig) *BatchRunner {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.CheckpointEvery <= 0 {
		cfg.CheckpointEvery = 10
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &BatchRunner{cfg: cfg}
}

// Run executes the pipeline. Resumes from last checkpoint if a prior run exists.
// Handles SIGINT/SIGTERM by checkpointing and exiting cleanly.
func (br *BatchRunner) Run(ctx context.Context) error {
	cfg := br.cfg

	// Check for resumable run
	runID, err := cfg.Store.FindResumablePipelineRun(cfg.PipelineType)
	if err != nil {
		return fmt.Errorf("find resumable run: %w", err)
	}

	var afterID int64
	var accumulated store.PipelineCheckpoint

	if runID > 0 {
		// Resume existing run
		cp, err := cfg.Store.GetPipelineCheckpoint(runID)
		if err != nil {
			return fmt.Errorf("get checkpoint: %w", err)
		}
		if cp != nil {
			afterID = cp.LastMessageID
			accumulated = *cp
			cfg.Logger.Info("resuming pipeline run",
				"run_id", runID,
				"last_message_id", afterID,
				"processed", cp.ProcessedMessages)
		}
	} else {
		// Start new run
		totalMessages := int64(0) // Progress will show actual/0 until first batch
		runID, err = cfg.Store.StartPipelineRun(cfg.PipelineType, totalMessages, cfg.FilterCriteria)
		if err != nil {
			return fmt.Errorf("start pipeline run: %w", err)
		}
		cfg.Logger.Info("started new pipeline run", "run_id", runID, "type", cfg.PipelineType)
	}

	// Set up graceful shutdown on SIGINT/SIGTERM
	sigCtx, sigCancel := context.WithCancel(ctx)
	defer sigCancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cfg.Logger.Info("interrupt received, finishing current batch and checkpointing...")
			sigCancel()
		case <-sigCtx.Done():
		}
	}()
	defer signal.Stop(sigCh)

	// Initialize progress reporter (total unknown upfront — shows count/0)
	br.progress = NewProgressReporter(cfg.PipelineType, 0)

	batchesSinceCheckpoint := 0

	for {
		// Check for cancellation before querying
		if sigCtx.Err() != nil {
			cfg.Logger.Info("checkpointing on shutdown", "last_message_id", afterID)
			break
		}

		// Fetch next batch
		messages, err := cfg.QueryMessages(afterID, cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("query messages: %w", err)
		}

		if len(messages) == 0 {
			// All messages processed
			break
		}

		// Process batch
		result, err := cfg.Process(sigCtx, messages)
		if err != nil {
			if sigCtx.Err() != nil {
				// Interrupted during processing — checkpoint what we have
				cfg.Logger.Info("interrupted during batch, checkpointing")
				break
			}
			// Record failure but continue with next batch
			cfg.Logger.Error("batch processing failed",
				"after_id", afterID,
				"batch_size", len(messages),
				"err", err)
			// Skip this batch by advancing past it
			afterID = messages[len(messages)-1].ID
			accumulated.FailedMessages += int64(len(messages))
			accumulated.LastMessageID = afterID
			batchesSinceCheckpoint++
			continue
		}

		// Accumulate stats
		accumulated.ProcessedMessages += result.Processed
		accumulated.SkippedMessages += result.Skipped
		accumulated.FailedMessages += result.Failed
		accumulated.TotalTokensInput += result.TokensInput
		accumulated.TotalTokensOutput += result.TokensOutput
		accumulated.EstimatedCostUSD += result.CostUSD
		accumulated.LastMessageID = result.LastMessageID
		afterID = result.LastMessageID

		// Update progress display
		br.progress.Update(result.Processed, result.Skipped, result.Failed,
			result.TokensInput, result.TokensOutput, result.CostUSD)

		batchesSinceCheckpoint++

		// Checkpoint periodically
		if batchesSinceCheckpoint >= cfg.CheckpointEvery {
			if err := cfg.Store.UpdatePipelineCheckpoint(runID, &accumulated); err != nil {
				cfg.Logger.Error("checkpoint failed", "err", err)
				// Non-fatal — continue processing
			} else {
				cfg.Logger.Debug("checkpoint saved", "last_message_id", afterID,
					"processed", accumulated.ProcessedMessages)
			}
			batchesSinceCheckpoint = 0
		}
	}

	// Final checkpoint
	if err := cfg.Store.UpdatePipelineCheckpoint(runID, &accumulated); err != nil {
		cfg.Logger.Error("final checkpoint failed", "err", err)
	}

	// Mark completion or cancellation
	if sigCtx.Err() != nil {
		// Cancelled — leave as 'running' for resume
		cfg.Logger.Info("pipeline paused, will resume from checkpoint",
			"processed", accumulated.ProcessedMessages,
			"last_message_id", accumulated.LastMessageID)
	} else {
		if err := cfg.Store.CompletePipelineRun(runID); err != nil {
			return fmt.Errorf("complete pipeline run: %w", err)
		}
	}

	br.progress.Finish()
	return nil
}
