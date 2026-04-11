package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PipelineRun represents a batch AI pipeline execution.
type PipelineRun struct {
	ID                int64
	PipelineType      string
	Status            string
	StartedAt         time.Time
	CompletedAt       sql.NullTime
	TotalMessages     int64
	ProcessedMessages int64
	SkippedMessages   int64
	FailedMessages    int64
	TotalTokensInput  int64
	TotalTokensOutput int64
	EstimatedCostUSD  float64
	ErrorMessage      string
	FilterCriteria    map[string]interface{}
}

// PipelineCheckpoint tracks resumption state for a pipeline run.
type PipelineCheckpoint struct {
	ProcessedMessages int64
	SkippedMessages   int64
	FailedMessages    int64
	LastMessageID     int64
	TotalTokensInput  int64
	TotalTokensOutput int64
	EstimatedCostUSD  float64
}

// StartPipelineRun creates a new pipeline run record and returns its ID.
func (s *Store) StartPipelineRun(pipelineType string, totalMessages int64, filterCriteria map[string]interface{}) (int64, error) {
	var filterJSON *string
	if filterCriteria != nil {
		data, err := json.Marshal(filterCriteria)
		if err != nil {
			return 0, fmt.Errorf("marshal filter criteria: %w", err)
		}
		str := string(data)
		filterJSON = &str
	}

	result, err := s.db.Exec(`
		INSERT INTO pipeline_runs (pipeline_type, status, started_at, total_messages, filter_criteria)
		VALUES (?, 'running', datetime('now'), ?, ?)
	`, pipelineType, totalMessages, filterJSON)
	if err != nil {
		return 0, fmt.Errorf("insert pipeline_run: %w", err)
	}
	return result.LastInsertId()
}

// UpdatePipelineCheckpoint saves progress for a pipeline run.
func (s *Store) UpdatePipelineCheckpoint(runID int64, cp *PipelineCheckpoint) error {
	_, err := s.db.Exec(`
		INSERT INTO pipeline_checkpoints (pipeline_run_id, last_message_id, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(pipeline_run_id) DO UPDATE SET
			last_message_id = excluded.last_message_id,
			updated_at = excluded.updated_at
	`, runID, cp.LastMessageID)
	if err != nil {
		return fmt.Errorf("upsert pipeline checkpoint: %w", err)
	}

	_, err = s.db.Exec(`
		UPDATE pipeline_runs
		SET processed_messages = ?,
		    skipped_messages = ?,
		    failed_messages = ?,
		    total_tokens_input = ?,
		    total_tokens_output = ?,
		    estimated_cost_usd = ?
		WHERE id = ?
	`, cp.ProcessedMessages, cp.SkippedMessages, cp.FailedMessages,
		cp.TotalTokensInput, cp.TotalTokensOutput, cp.EstimatedCostUSD, runID)
	return err
}

// CompletePipelineRun marks a run as completed.
func (s *Store) CompletePipelineRun(runID int64) error {
	_, err := s.db.Exec(`
		UPDATE pipeline_runs
		SET status = 'completed', completed_at = datetime('now')
		WHERE id = ?
	`, runID)
	return err
}

// FailPipelineRun marks a run as failed with an error message.
func (s *Store) FailPipelineRun(runID int64, errMsg string) error {
	_, err := s.db.Exec(`
		UPDATE pipeline_runs
		SET status = 'failed', completed_at = datetime('now'), error_message = ?
		WHERE id = ?
	`, errMsg, runID)
	return err
}

// GetPipelineCheckpoint returns the last checkpoint for a pipeline run, or nil if none.
func (s *Store) GetPipelineCheckpoint(runID int64) (*PipelineCheckpoint, error) {
	var cp PipelineCheckpoint
	var run PipelineRun
	err := s.db.QueryRow(`
		SELECT pc.last_message_id, pr.processed_messages, pr.skipped_messages,
		       pr.failed_messages, pr.total_tokens_input, pr.total_tokens_output,
		       pr.estimated_cost_usd
		FROM pipeline_checkpoints pc
		JOIN pipeline_runs pr ON pr.id = pc.pipeline_run_id
		WHERE pc.pipeline_run_id = ?
	`, runID).Scan(&cp.LastMessageID, &run.ProcessedMessages, &run.SkippedMessages,
		&run.FailedMessages, &run.TotalTokensInput, &run.TotalTokensOutput,
		&run.EstimatedCostUSD)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pipeline checkpoint: %w", err)
	}
	cp.ProcessedMessages = run.ProcessedMessages
	cp.SkippedMessages = run.SkippedMessages
	cp.FailedMessages = run.FailedMessages
	cp.TotalTokensInput = run.TotalTokensInput
	cp.TotalTokensOutput = run.TotalTokensOutput
	cp.EstimatedCostUSD = run.EstimatedCostUSD
	return &cp, nil
}

// FindResumablePipelineRun returns the most recent incomplete run of the given type, or 0 if none.
func (s *Store) FindResumablePipelineRun(pipelineType string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		SELECT id FROM pipeline_runs
		WHERE pipeline_type = ? AND status = 'running'
		ORDER BY started_at DESC LIMIT 1
	`, pipelineType).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find resumable pipeline run: %w", err)
	}
	return id, nil
}
