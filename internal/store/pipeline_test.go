package store_test

import (
	"testing"

	"github.com/wesm/msgvault/internal/store"
	"github.com/wesm/msgvault/internal/testutil"
	"github.com/wesm/msgvault/internal/testutil/storetest"
)

func TestPipeline_StartPipelineRun(t *testing.T) {
	f := storetest.New(t)

	runID, err := f.Store.StartPipelineRun("embedding", 1000, nil)
	testutil.MustNoErr(t, err, "StartPipelineRun")

	if runID == 0 {
		t.Error("StartPipelineRun returned 0, want non-zero ID")
	}
}

func TestPipeline_StartPipelineRun_WithFilterCriteria(t *testing.T) {
	f := storetest.New(t)

	filter := map[string]interface{}{
		"source_id": 1,
		"after":     "2024-01-01",
	}
	runID, err := f.Store.StartPipelineRun("categorize", 500, filter)
	testutil.MustNoErr(t, err, "StartPipelineRun with filter")

	if runID == 0 {
		t.Error("StartPipelineRun returned 0, want non-zero ID")
	}
}

func TestPipeline_CheckpointRoundTrip(t *testing.T) {
	f := storetest.New(t)

	runID, err := f.Store.StartPipelineRun("embedding", 100, nil)
	testutil.MustNoErr(t, err, "StartPipelineRun")

	// Initially nil
	got, err := f.Store.GetPipelineCheckpoint(runID)
	testutil.MustNoErr(t, err, "GetPipelineCheckpoint (initial)")
	if got != nil {
		t.Errorf("GetPipelineCheckpoint initial = %+v, want nil", got)
	}

	// Save checkpoint
	cp := &store.PipelineCheckpoint{
		LastMessageID:     42,
		ProcessedMessages: 10,
		SkippedMessages:   2,
		FailedMessages:    1,
		TotalTokensInput:  5000,
		TotalTokensOutput: 800,
		EstimatedCostUSD:  0.0125,
	}
	err = f.Store.UpdatePipelineCheckpoint(runID, cp)
	testutil.MustNoErr(t, err, "UpdatePipelineCheckpoint")

	// Retrieve checkpoint
	got, err = f.Store.GetPipelineCheckpoint(runID)
	testutil.MustNoErr(t, err, "GetPipelineCheckpoint after update")
	if got == nil {
		t.Fatal("GetPipelineCheckpoint returned nil after update")
	}
	if got.LastMessageID != 42 {
		t.Errorf("LastMessageID = %d, want 42", got.LastMessageID)
	}
	if got.ProcessedMessages != 10 {
		t.Errorf("ProcessedMessages = %d, want 10", got.ProcessedMessages)
	}
	if got.SkippedMessages != 2 {
		t.Errorf("SkippedMessages = %d, want 2", got.SkippedMessages)
	}
	if got.FailedMessages != 1 {
		t.Errorf("FailedMessages = %d, want 1", got.FailedMessages)
	}
	if got.TotalTokensInput != 5000 {
		t.Errorf("TotalTokensInput = %d, want 5000", got.TotalTokensInput)
	}
	if got.TotalTokensOutput != 800 {
		t.Errorf("TotalTokensOutput = %d, want 800", got.TotalTokensOutput)
	}

	// Update checkpoint (upsert)
	cp2 := &store.PipelineCheckpoint{
		LastMessageID:     99,
		ProcessedMessages: 20,
		SkippedMessages:   3,
		FailedMessages:    2,
		TotalTokensInput:  10000,
		TotalTokensOutput: 1600,
		EstimatedCostUSD:  0.025,
	}
	err = f.Store.UpdatePipelineCheckpoint(runID, cp2)
	testutil.MustNoErr(t, err, "UpdatePipelineCheckpoint (second)")

	got2, err := f.Store.GetPipelineCheckpoint(runID)
	testutil.MustNoErr(t, err, "GetPipelineCheckpoint after second update")
	if got2 == nil {
		t.Fatal("GetPipelineCheckpoint returned nil after second update")
	}
	if got2.LastMessageID != 99 {
		t.Errorf("LastMessageID = %d, want 99", got2.LastMessageID)
	}
	if got2.ProcessedMessages != 20 {
		t.Errorf("ProcessedMessages = %d, want 20", got2.ProcessedMessages)
	}
}

func TestPipeline_CompletePipelineRun(t *testing.T) {
	f := storetest.New(t)

	runID, err := f.Store.StartPipelineRun("life_events", 50, nil)
	testutil.MustNoErr(t, err, "StartPipelineRun")

	err = f.Store.CompletePipelineRun(runID)
	testutil.MustNoErr(t, err, "CompletePipelineRun")

	// Verify status is 'completed' and completed_at is set
	var status string
	var completedAt string
	err = f.Store.DB().QueryRow(
		`SELECT status, COALESCE(completed_at, '') FROM pipeline_runs WHERE id = ?`, runID,
	).Scan(&status, &completedAt)
	testutil.MustNoErr(t, err, "query pipeline_run status")

	if status != "completed" {
		t.Errorf("status = %q, want %q", status, "completed")
	}
	if completedAt == "" {
		t.Error("completed_at should be set after CompletePipelineRun")
	}
}

func TestPipeline_FailPipelineRun(t *testing.T) {
	f := storetest.New(t)

	runID, err := f.Store.StartPipelineRun("entities", 200, nil)
	testutil.MustNoErr(t, err, "StartPipelineRun")

	err = f.Store.FailPipelineRun(runID, "context deadline exceeded")
	testutil.MustNoErr(t, err, "FailPipelineRun")

	// Verify status, completed_at, and error_message
	var status, completedAt, errMsg string
	err = f.Store.DB().QueryRow(
		`SELECT status, COALESCE(completed_at, ''), COALESCE(error_message, '') FROM pipeline_runs WHERE id = ?`,
		runID,
	).Scan(&status, &completedAt, &errMsg)
	testutil.MustNoErr(t, err, "query pipeline_run after fail")

	if status != "failed" {
		t.Errorf("status = %q, want %q", status, "failed")
	}
	if completedAt == "" {
		t.Error("completed_at should be set after FailPipelineRun")
	}
	if errMsg != "context deadline exceeded" {
		t.Errorf("error_message = %q, want %q", errMsg, "context deadline exceeded")
	}
}

func TestPipeline_FindResumablePipelineRun(t *testing.T) {
	f := storetest.New(t)

	// No runs yet — should return 0
	id, err := f.Store.FindResumablePipelineRun("embedding")
	testutil.MustNoErr(t, err, "FindResumablePipelineRun (empty)")
	if id != 0 {
		t.Errorf("FindResumablePipelineRun (empty) = %d, want 0", id)
	}

	// Start a run
	runID, err := f.Store.StartPipelineRun("embedding", 100, nil)
	testutil.MustNoErr(t, err, "StartPipelineRun")

	// Should now be resumable
	id, err = f.Store.FindResumablePipelineRun("embedding")
	testutil.MustNoErr(t, err, "FindResumablePipelineRun (running)")
	if id != runID {
		t.Errorf("FindResumablePipelineRun = %d, want %d", id, runID)
	}

	// Complete it — should no longer be resumable
	err = f.Store.CompletePipelineRun(runID)
	testutil.MustNoErr(t, err, "CompletePipelineRun")

	id, err = f.Store.FindResumablePipelineRun("embedding")
	testutil.MustNoErr(t, err, "FindResumablePipelineRun (completed)")
	if id != 0 {
		t.Errorf("FindResumablePipelineRun (completed) = %d, want 0", id)
	}

	// Different pipeline type — should not interfere
	runID2, err := f.Store.StartPipelineRun("categorize", 50, nil)
	testutil.MustNoErr(t, err, "StartPipelineRun categorize")

	id, err = f.Store.FindResumablePipelineRun("embedding")
	testutil.MustNoErr(t, err, "FindResumablePipelineRun (embedding, categorize running)")
	if id != 0 {
		t.Errorf("FindResumablePipelineRun (embedding) = %d, want 0 when only categorize is running", id)
	}

	id, err = f.Store.FindResumablePipelineRun("categorize")
	testutil.MustNoErr(t, err, "FindResumablePipelineRun (categorize)")
	if id != runID2 {
		t.Errorf("FindResumablePipelineRun (categorize) = %d, want %d", id, runID2)
	}
}

func TestPipeline_GetPipelineCheckpoint_NilForNonExistent(t *testing.T) {
	f := storetest.New(t)

	// Use a run ID that doesn't exist — should return nil, not an error
	got, err := f.Store.GetPipelineCheckpoint(99999)
	testutil.MustNoErr(t, err, "GetPipelineCheckpoint (nonexistent)")
	if got != nil {
		t.Errorf("GetPipelineCheckpoint (nonexistent) = %+v, want nil", got)
	}
}
