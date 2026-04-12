// Package enrichment provides the AI enrichment pipeline for categorization,
// life event extraction, and entity extraction via Azure OpenAI GPT-4o-mini.
package enrichment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/store"
)

// costInputPerToken is the GPT-4o-mini input token price: $0.15 per 1M tokens.
const costInputPerToken = 0.15 / 1_000_000

// costOutputPerToken is the GPT-4o-mini output token price: $0.60 per 1M tokens.
const costOutputPerToken = 0.60 / 1_000_000

// costForTokens calculates the USD cost for a single ChatCompletion call.
func costForTokens(usage ai.Usage) float64 {
	return float64(usage.PromptTokens)*costInputPerToken +
		float64(usage.CompletionTokens)*costOutputPerToken
}

// chatFunc is an abstraction over ai.Client.ChatCompletion for testability.
type chatFunc func(ctx context.Context, deployment string, req ai.ChatRequest) (*ai.ChatResponse, error)

// isCategorizedFunc checks whether a message already has an auto label (for idempotency).
type isCategorizedFunc func(messageID int64) bool

// writeResultsFunc writes enrichment results to the store for a single message.
type writeResultsFunc func(messageID int64, result *EnrichResult) error

// createEnrichQueryFunc returns a MessageQueryFunc that fetches messages not yet categorized.
// Uses NOT EXISTS (semi-join) to avoid DISTINCT + JOIN per CLAUDE.md SQL guidelines.
func createEnrichQueryFunc(s *store.Store) ai.MessageQueryFunc {
	return func(afterID int64, batchSize int) ([]ai.MessageRow, error) {
		rows, err := s.DB().Query(`
			SELECT m.id, m.subject, m.snippet FROM messages m
			WHERE m.id > ?
			AND NOT EXISTS (
				SELECT 1 FROM message_labels ml
				JOIN labels l ON l.id = ml.label_id
				WHERE ml.message_id = m.id AND l.label_type = 'auto'
			)
			ORDER BY m.id ASC LIMIT ?
		`, afterID, batchSize)
		if err != nil {
			return nil, fmt.Errorf("query uncategorized messages: %w", err)
		}
		defer rows.Close()

		var result []ai.MessageRow
		for rows.Next() {
			var row ai.MessageRow
			var subject, snippet *string
			if err := rows.Scan(&row.ID, &subject, &snippet); err != nil {
				return nil, fmt.Errorf("scan message row: %w", err)
			}
			if subject != nil {
				row.Subject = *subject
			}
			if snippet != nil {
				row.Snippet = *snippet
			}
			result = append(result, row)
		}
		return result, rows.Err()
	}
}

// buildEnrichProcessFunc returns the ProcessFunc for the enrichment pipeline.
// It calls ChatCompletion once per message and fans out results to three write paths:
// category label, life events, entities.
func buildEnrichProcessFunc(client *ai.Client, s *store.Store, deployment string, logger *slog.Logger) ai.ProcessFunc {
	if logger == nil {
		logger = slog.Default()
	}
	chatFn := func(ctx context.Context, depl string, req ai.ChatRequest) (*ai.ChatResponse, error) {
		return client.ChatCompletion(ctx, depl, req)
	}
	isCategorized := func(messageID int64) bool {
		var exists bool
		err := s.DB().QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM message_labels ml
				JOIN labels l ON l.id = ml.label_id
				WHERE ml.message_id = ? AND l.label_type = 'auto'
			)
		`, messageID).Scan(&exists)
		if err != nil {
			return false
		}
		return exists
	}
	writeResults := func(messageID int64, result *EnrichResult) error {
		return writeEnrichResults(s, messageID, result)
	}
	return buildEnrichProcessFuncTestable(chatFn, isCategorized, writeResults, deployment, logger)
}

// buildEnrichProcessFuncTestable is the injectable version for both production and tests.
func buildEnrichProcessFuncTestable(
	chatFn chatFunc,
	isCategorized isCategorizedFunc,
	writeResults writeResultsFunc,
	deployment string,
	logger *slog.Logger,
) ai.ProcessFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, messages []ai.MessageRow) (*ai.BatchResult, error) {
		result := &ai.BatchResult{}

		for _, msg := range messages {
			// Idempotency: skip already-categorized messages.
			if isCategorized(msg.ID) {
				result.Skipped++
				continue
			}

			// Call LLM.
			resp, err := chatFn(ctx, deployment, buildEnrichRequest(msg.Subject, msg.Snippet))
			if err != nil {
				result.Failed++
				logger.Warn("LLM call failed, skipping message", "msg_id", msg.ID, "err", err)
				continue
			}

			// Extract response content.
			if len(resp.Choices) == 0 {
				result.Failed++
				logger.Warn("LLM returned no choices, skipping", "msg_id", msg.ID)
				continue
			}
			content := resp.Choices[0].Message.Content

			// Parse JSON response.
			enrichResult, err := parseEnrichResponse(content)
			if err != nil {
				result.Failed++
				logger.Warn("malformed LLM response, skipping", "msg_id", msg.ID, "err", err)
				continue
			}

			// Validate category against allowlist (threat model T-14-01).
			enrichResult.Category = validateCategory(enrichResult.Category)

			// Write to store.
			if err := writeResults(msg.ID, enrichResult); err != nil {
				result.Failed++
				logger.Warn("failed to write enrichment results", "msg_id", msg.ID, "err", err)
				continue
			}

			result.Processed++
			result.TokensInput += int64(resp.Usage.PromptTokens)
			result.TokensOutput += int64(resp.Usage.CompletionTokens)
			result.CostUSD += costForTokens(resp.Usage)
			result.LastMessageID = msg.ID
		}

		// Advance LastMessageID even if all skipped, to avoid infinite loop.
		if result.LastMessageID == 0 && len(messages) > 0 {
			result.LastMessageID = messages[len(messages)-1].ID
		}

		return result, nil
	}
}

// writeEnrichResults writes the enrichment result for one message to all three
// storage targets: category label, life events table, entities table.
func writeEnrichResults(s *store.Store, messageID int64, result *EnrichResult) error {
	// 1. Store category as an auto label on the message.
	labelID, err := s.GetOrCreateAutoLabel(result.Category)
	if err != nil {
		return fmt.Errorf("get/create auto label %q: %w", result.Category, err)
	}
	if err := s.AddMessageLabels(messageID, []int64{labelID}); err != nil {
		return fmt.Errorf("add auto label to message %d: %w", messageID, err)
	}

	// 2. Store life events.
	for _, event := range result.LifeEvents {
		if err := s.InsertLifeEvent(messageID, event.Date, event.Type, event.Description); err != nil {
			return fmt.Errorf("insert life event for message %d: %w", messageID, err)
		}
	}

	// 3. Store entities.
	for _, entity := range result.Entities {
		normalized := normalizeEntityValue(entity.Type, entity.Value)
		if err := s.InsertEntity(messageID, entity.Type, entity.Value, normalized, ""); err != nil {
			return fmt.Errorf("insert entity for message %d: %w", messageID, err)
		}
	}

	return nil
}

// RunEnrichPipeline runs the full enrichment pipeline for all uncategorized messages.
// Uses the Phase 12 BatchRunner for resumability and checkpoint-based recovery.
func RunEnrichPipeline(ctx context.Context, client *ai.Client, s *store.Store, logger *slog.Logger, batchSize int, deployment string) error {
	if logger == nil {
		logger = slog.Default()
	}
	if batchSize <= 0 {
		batchSize = 20
	}
	if deployment == "" {
		deployment = "chat"
	}

	runner := ai.NewBatchRunner(ai.RunConfig{
		PipelineType:    "categorize",
		BatchSize:       batchSize,
		CheckpointEvery: 10,
		Store:           s,
		Logger:          logger,
		QueryMessages:   createEnrichQueryFunc(s),
		Process:         buildEnrichProcessFunc(client, s, deployment, logger),
	})

	return runner.Run(ctx)
}

// CountUnenriched returns the number of messages not yet categorized by the enrichment pipeline.
func CountUnenriched(s *store.Store) (int64, error) {
	var count int64
	err := s.DB().QueryRow(`
		SELECT COUNT(*) FROM messages m
		WHERE NOT EXISTS (
			SELECT 1 FROM message_labels ml
			JOIN labels l ON l.id = ml.label_id
			WHERE ml.message_id = m.id AND l.label_type = 'auto'
		)
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unenriched: %w", err)
	}
	return count, nil
}
