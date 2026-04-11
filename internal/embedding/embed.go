// Package embedding provides the embedding pipeline and semantic search engine.
// It uses the Phase 12 BatchRunner to embed messages via Azure OpenAI and
// stores vectors in sqlite-vec for KNN retrieval.
package embedding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/store"
)

// textEmbeddingDeployment is the logical deployment name for text embeddings.
// Maps to the azure_openai.deployments.text-embedding entry in config.toml.
const textEmbeddingDeployment = "text-embedding"

// costPerTokenUSD is the cost per input token for text-embedding-3-small.
// $0.02 per 1M tokens = $0.00000002 per token.
const costPerTokenUSD = 0.02 / 1_000_000

// BuildEmbedText constructs the text to embed for a message.
// Format: "Subject: {subject}\n{snippet}" trimming empty parts.
func BuildEmbedText(subject, snippet string) string {
	subject = strings.TrimSpace(subject)
	snippet = strings.TrimSpace(snippet)

	if subject == "" && snippet == "" {
		return ""
	}
	if subject == "" {
		return snippet
	}
	if snippet == "" {
		return "Subject: " + subject
	}
	return "Subject: " + subject + "\n" + snippet
}

// embeddingFunc is an abstraction over ai.Client.Embedding for testability.
type embeddingFunc func(ctx context.Context, deployment string, texts []string) (*ai.EmbeddingResponse, error)

// hasEmbeddingFunc checks whether a message already has an embedding.
type hasEmbeddingFunc func(messageID int64) bool

// createQueryFunc returns a MessageQueryFunc that fetches messages not yet in vec_messages.
func createQueryFunc(s *store.Store) ai.MessageQueryFunc {
	return func(afterID int64, batchSize int) ([]ai.MessageRow, error) {
		rows, err := s.DB().Query(`
			SELECT m.id, m.subject, m.snippet
			FROM messages m
			WHERE m.id > ? AND NOT EXISTS (
				SELECT 1 FROM vec_messages v WHERE v.message_id = m.id
			)
			ORDER BY m.id ASC
			LIMIT ?
		`, afterID, batchSize)
		if err != nil {
			return nil, fmt.Errorf("query unembedded messages: %w", err)
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

// createProcessFunc returns a ProcessFunc that embeds a batch of messages.
// callEmbed is the embedding API function (injectable for testing).
// checkExisting returns true if a message already has an embedding (for idempotency).
func createProcessFunc(
	callEmbed embeddingFunc,
	checkExisting func(id int64) (bool, error),
	insertEmbed func(entries []store.VectorEntry) error,
) ai.ProcessFunc {
	return func(ctx context.Context, messages []ai.MessageRow) (*ai.BatchResult, error) {
		// Filter out already-embedded messages.
		var toEmbed []ai.MessageRow
		var skipped int64
		for _, m := range messages {
			if checkExisting != nil {
				has, err := checkExisting(m.ID)
				if err != nil {
					return nil, fmt.Errorf("check existing embedding for %d: %w", m.ID, err)
				}
				if has {
					skipped++
					continue
				}
			}
			toEmbed = append(toEmbed, m)
		}

		if len(toEmbed) == 0 {
			lastID := int64(0)
			if len(messages) > 0 {
				lastID = messages[len(messages)-1].ID
			}
			return &ai.BatchResult{
				Skipped:       skipped,
				LastMessageID: lastID,
			}, nil
		}

		// Build embed texts.
		texts := make([]string, len(toEmbed))
		for i, m := range toEmbed {
			texts[i] = BuildEmbedText(m.Subject, m.Snippet)
		}

		// Call the embedding API.
		resp, err := callEmbed(ctx, textEmbeddingDeployment, texts)
		if err != nil {
			return nil, fmt.Errorf("embedding API: %w", err)
		}

		// Map embeddings back to message IDs.
		entries := make([]store.VectorEntry, len(resp.Data))
		for _, d := range resp.Data {
			if d.Index >= len(toEmbed) {
				continue
			}
			entries[d.Index] = store.VectorEntry{
				MessageID: toEmbed[d.Index].ID,
				Embedding: d.Embedding,
			}
		}

		// Store vectors.
		if err := insertEmbed(entries); err != nil {
			return nil, fmt.Errorf("store embeddings: %w", err)
		}

		promptTokens := int64(resp.Usage.PromptTokens)
		lastID := toEmbed[len(toEmbed)-1].ID

		return &ai.BatchResult{
			Processed:     int64(len(toEmbed)),
			Skipped:       skipped,
			TokensInput:   promptTokens,
			CostUSD:       float64(promptTokens) * costPerTokenUSD,
			LastMessageID: lastID,
		}, nil
	}
}

// RunEmbedPipeline runs the full embedding pipeline, embedding all messages that
// don't yet have a vector. Uses the Phase 12 BatchRunner for resumability and
// checkpoint-based recovery.
func RunEmbedPipeline(ctx context.Context, client *ai.Client, s *store.Store, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	// Ensure the vector table exists.
	if err := s.InitVectorTable(); err != nil {
		return fmt.Errorf("init vector table: %w", err)
	}

	callEmbed := func(ctx context.Context, deployment string, texts []string) (*ai.EmbeddingResponse, error) {
		return client.Embedding(ctx, deployment, texts)
	}

	processFn := createProcessFunc(
		callEmbed,
		s.HasEmbedding,
		s.InsertEmbeddings,
	)

	runner := ai.NewBatchRunner(ai.RunConfig{
		PipelineType:    "embedding",
		BatchSize:       100,
		CheckpointEvery: 10,
		Store:           s,
		Logger:          logger,
		QueryMessages:   createQueryFunc(s),
		Process:         processFn,
	})

	return runner.Run(ctx)
}

// CountUnembedded returns the number of messages that do not yet have embeddings.
func CountUnembedded(s *store.Store) (int64, error) {
	var count int64
	err := s.DB().QueryRow(`
		SELECT COUNT(*)
		FROM messages m
		WHERE NOT EXISTS (SELECT 1 FROM vec_messages v WHERE v.message_id = m.id)
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unembedded: %w", err)
	}
	return count, nil
}

// --- Test helpers (exported via _test suffix files) ---

// testProcessFunc is a test-accessible wrapper around createProcessFunc
// that skips the "check existing" step (no-skip variant).
func testProcessFunc(
	callEmbed embeddingFunc,
	_ interface{},
	messages []ai.MessageRow,
) (*ai.BatchResult, error) {
	fn := createProcessFunc(callEmbed, nil, func(entries []store.VectorEntry) error { return nil })
	return fn(context.Background(), messages)
}

// testProcessFuncWithSkip is a test-accessible wrapper that uses a custom hasEmbedding.
func testProcessFuncWithSkip(
	callEmbed embeddingFunc,
	hasEmbedding func(id int64) bool,
	messages []ai.MessageRow,
) (*ai.BatchResult, error) {
	checkExisting := func(id int64) (bool, error) {
		return hasEmbedding(id), nil
	}
	fn := createProcessFunc(callEmbed, checkExisting, func(entries []store.VectorEntry) error { return nil })
	return fn(context.Background(), messages)
}

// buildTestQueryFunc returns a MessageQueryFunc backed by an in-memory slice.
func buildTestQueryFunc(messages []ai.MessageRow) ai.MessageQueryFunc {
	return func(afterID int64, batchSize int) ([]ai.MessageRow, error) {
		var result []ai.MessageRow
		for _, m := range messages {
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
