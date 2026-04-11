package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/wesm/msgvault/internal/testutil"
)

// makeMessages creates a slice of fake MessageRows with IDs 1..n.
func makeMessages(n int) []MessageRow {
	msgs := make([]MessageRow, n)
	for i := range msgs {
		msgs[i] = MessageRow{
			ID:      int64(i + 1),
			Subject: "Test message",
			Snippet: "This is a test.",
		}
	}
	return msgs
}

// sliceQueryFn returns a MessageQueryFunc that pages through msgs.
func sliceQueryFn(msgs []MessageRow) MessageQueryFunc {
	return func(afterID int64, batchSize int) ([]MessageRow, error) {
		var result []MessageRow
		for _, m := range msgs {
			if m.ID > afterID {
				result = append(result, m)
				if len(result) >= batchSize {
					break
				}
			}
		}
		return result, nil
	}
}

// noopProcessFn returns a ProcessFunc that marks all messages processed.
func noopProcessFn(ctx context.Context, messages []MessageRow) (*BatchResult, error) {
	if len(messages) == 0 {
		return &BatchResult{}, nil
	}
	return &BatchResult{
		Processed:     int64(len(messages)),
		TokensInput:   int64(len(messages) * 10),
		CostUSD:       float64(len(messages)) * 0.000001,
		LastMessageID: messages[len(messages)-1].ID,
	}, nil
}

func TestBatchRunner_NormalCompletion(t *testing.T) {
	st := testutil.NewTestStore(t)
	msgs := makeMessages(25)

	runner := NewBatchRunner(RunConfig{
		PipelineType:    "test",
		BatchSize:       10,
		CheckpointEvery: 2,
		Store:           st,
		QueryMessages:   sliceQueryFn(msgs),
		Process:         noopProcessFn,
	})

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Verify run was marked completed
	var status string
	err = st.DB().QueryRow(`SELECT status FROM pipeline_runs WHERE pipeline_type = 'test'`).Scan(&status)
	if err != nil {
		t.Fatalf("query pipeline_runs: %v", err)
	}
	if status != "completed" {
		t.Errorf("pipeline_runs.status = %q, want %q", status, "completed")
	}

	// Verify checkpoint reflects all processed messages
	var runID int64
	err = st.DB().QueryRow(`SELECT id FROM pipeline_runs WHERE pipeline_type = 'test'`).Scan(&runID)
	if err != nil {
		t.Fatalf("query run id: %v", err)
	}
	cp, err := st.GetPipelineCheckpoint(runID)
	if err != nil {
		t.Fatalf("GetPipelineCheckpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("checkpoint is nil after completion")
	}
	if cp.ProcessedMessages != 25 {
		t.Errorf("processed_messages = %d, want 25", cp.ProcessedMessages)
	}
	if cp.LastMessageID != 25 {
		t.Errorf("last_message_id = %d, want 25", cp.LastMessageID)
	}
}

func TestBatchRunner_EmptyMessageSet(t *testing.T) {
	st := testutil.NewTestStore(t)

	runner := NewBatchRunner(RunConfig{
		PipelineType:  "test",
		BatchSize:     10,
		Store:         st,
		QueryMessages: sliceQueryFn(nil), // no messages
		Process:       noopProcessFn,
	})

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() with empty message set returned error: %v", err)
	}

	// Run should be created and completed
	var count int
	err = st.DB().QueryRow(`SELECT COUNT(*) FROM pipeline_runs WHERE pipeline_type = 'test' AND status = 'completed'`).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("completed run count = %d, want 1", count)
	}
}

func TestBatchRunner_Resumability(t *testing.T) {
	st := testutil.NewTestStore(t)
	msgs := makeMessages(20)

	// Track which message IDs were processed in second run
	var secondRunStartID int64

	// First run: process 10 messages then simulate stop by limiting query to first 10
	firstRunMsgs := msgs[:10]
	runner1 := NewBatchRunner(RunConfig{
		PipelineType:    "test",
		BatchSize:       10,
		CheckpointEvery: 1, // checkpoint after every batch
		Store:           st,
		QueryMessages:   sliceQueryFn(firstRunMsgs),
		Process:         noopProcessFn,
	})
	if err := runner1.Run(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// First run completes (status = completed). To test resume we need a 'running' status.
	// Reset the status to simulate an interrupted run.
	_, err := st.DB().Exec(`UPDATE pipeline_runs SET status = 'running', completed_at = NULL`)
	if err != nil {
		t.Fatalf("reset status: %v", err)
	}

	// Second run should resume from last checkpoint (ID=10)
	runner2 := NewBatchRunner(RunConfig{
		PipelineType:    "test",
		BatchSize:       10,
		CheckpointEvery: 1,
		Store:           st,
		QueryMessages:   sliceQueryFn(msgs), // all 20 messages
		Process: func(ctx context.Context, messages []MessageRow) (*BatchResult, error) {
			if secondRunStartID == 0 {
				secondRunStartID = messages[0].ID
			}
			return noopProcessFn(ctx, messages)
		},
	})
	if err := runner2.Run(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}

	// Second run should start from ID > 10
	if secondRunStartID <= 10 {
		t.Errorf("second run started at ID %d, want > 10 (should resume from checkpoint)", secondRunStartID)
	}
}

func TestBatchRunner_BatchFailureContinues(t *testing.T) {
	st := testutil.NewTestStore(t)
	msgs := makeMessages(30)

	batchNum := 0
	failingProcess := func(ctx context.Context, messages []MessageRow) (*BatchResult, error) {
		batchNum++
		// Fail batch 2, succeed all others
		if batchNum == 2 {
			return nil, errors.New("simulated batch failure")
		}
		return noopProcessFn(ctx, messages)
	}

	runner := NewBatchRunner(RunConfig{
		PipelineType:    "test",
		BatchSize:       10,
		CheckpointEvery: 5,
		Store:           st,
		QueryMessages:   sliceQueryFn(msgs),
		Process:         failingProcess,
	})

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() should not error on batch failures: %v", err)
	}

	// Run should still complete
	var status string
	if err := st.DB().QueryRow(`SELECT status FROM pipeline_runs WHERE pipeline_type = 'test'`).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "completed" {
		t.Errorf("status = %q, want completed after batch failures", status)
	}

	// Batch 2 messages should be counted as failed
	var runID int64
	if err := st.DB().QueryRow(`SELECT id FROM pipeline_runs WHERE pipeline_type = 'test'`).Scan(&runID); err != nil {
		t.Fatalf("query run id: %v", err)
	}
	cp, err := st.GetPipelineCheckpoint(runID)
	if err != nil {
		t.Fatalf("GetPipelineCheckpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("checkpoint nil after run with failures")
	}
	if cp.FailedMessages != 10 {
		t.Errorf("failed_messages = %d, want 10 (one failed batch)", cp.FailedMessages)
	}
	// 20 messages should have been processed (batches 1 and 3)
	if cp.ProcessedMessages != 20 {
		t.Errorf("processed_messages = %d, want 20", cp.ProcessedMessages)
	}
}

func TestBatchRunner_CheckpointFrequency(t *testing.T) {
	st := testutil.NewTestStore(t)
	msgs := makeMessages(50)

	checkpointEvery := 5
	runner := NewBatchRunner(RunConfig{
		PipelineType:    "test",
		BatchSize:       5, // 10 batches total
		CheckpointEvery: checkpointEvery,
		Store:           st,
		QueryMessages:   sliceQueryFn(msgs),
		Process:         noopProcessFn,
	})

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	// Verify checkpoint was stored (we can't directly count updates, but verify it exists)
	var runID int64
	if err := st.DB().QueryRow(`SELECT id FROM pipeline_runs WHERE pipeline_type = 'test'`).Scan(&runID); err != nil {
		t.Fatalf("query run id: %v", err)
	}
	cp, err := st.GetPipelineCheckpoint(runID)
	if err != nil {
		t.Fatalf("GetPipelineCheckpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("no checkpoint after run")
	}
	if cp.ProcessedMessages != 50 {
		t.Errorf("processed_messages = %d, want 50", cp.ProcessedMessages)
	}
	if cp.LastMessageID != 50 {
		t.Errorf("last_message_id = %d, want 50", cp.LastMessageID)
	}
}

func TestBatchRunner_ContextCancellation(t *testing.T) {
	st := testutil.NewTestStore(t)
	msgs := makeMessages(100)

	ctx, cancel := context.WithCancel(context.Background())
	batchNum := 0

	runner := NewBatchRunner(RunConfig{
		PipelineType:    "test",
		BatchSize:       10,
		CheckpointEvery: 1,
		Store:           st,
		QueryMessages:   sliceQueryFn(msgs),
		Process: func(c context.Context, messages []MessageRow) (*BatchResult, error) {
			batchNum++
			if batchNum == 3 {
				// Cancel context after 3rd batch
				cancel()
			}
			return noopProcessFn(c, messages)
		},
	})

	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() returned error on cancel: %v", err)
	}

	// Run should remain 'running' (not completed) for resume
	var status string
	if err := st.DB().QueryRow(`SELECT status FROM pipeline_runs WHERE pipeline_type = 'test'`).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want 'running' after cancellation (enables resume)", status)
	}

	// Checkpoint should be saved
	var runID int64
	if err := st.DB().QueryRow(`SELECT id FROM pipeline_runs WHERE pipeline_type = 'test'`).Scan(&runID); err != nil {
		t.Fatalf("query run id: %v", err)
	}
	cp, err := st.GetPipelineCheckpoint(runID)
	if err != nil {
		t.Fatalf("GetPipelineCheckpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("no checkpoint after cancellation")
	}
	if cp.LastMessageID == 0 {
		t.Error("last_message_id should be non-zero after cancellation")
	}
}
