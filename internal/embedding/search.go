package embedding

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/store"
)

// SemanticResult extends query.MessageSummary with a cosine similarity percentage.
type SemanticResult struct {
	query.MessageSummary
	SimilarityPct float64 // 0.0 to 100.0
}

// SemanticSearch embeds queryText, searches vec_messages via KNN, and enriches
// each result with message metadata from SQLite. Returns up to limit results
// ordered by similarity (highest first). SimilarityPct is in [0, 100].
func SemanticSearch(
	ctx context.Context,
	client *ai.Client,
	s *store.Store,
	queryText string,
	limit int,
) ([]SemanticResult, error) {
	// 1. Embed the query.
	resp, err := client.Embedding(ctx, textEmbeddingDeployment, []string{queryText})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding API returned no vectors")
	}
	queryVec := resp.Data[0].Embedding

	return semanticSearchWithVec(ctx, s, queryVec, queryText, limit)
}

// semanticSearchWithVec performs the KNN lookup and metadata enrichment.
// Separated for testability (allows injecting pre-computed query vectors).
func semanticSearchWithVec(
	ctx context.Context,
	s *store.Store,
	queryVec []float32,
	_ string,
	limit int,
) ([]SemanticResult, error) {
	// 2. KNN search in vec_messages.
	vecResults, err := s.SearchSemantic(queryVec, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	if len(vecResults) == 0 {
		return nil, nil
	}

	// 3. Enrich with message metadata.
	db := s.DB()
	var results []SemanticResult

	for _, vr := range vecResults {
		summary, err := fetchMessageSummary(ctx, db, vr.MessageID)
		if err != nil {
			// Message may have been deleted — skip rather than fail.
			continue
		}
		results = append(results, SemanticResult{
			MessageSummary: *summary,
			SimilarityPct:  vr.Similarity * 100.0,
		})
	}

	return results, nil
}

// fetchMessageSummary fetches message metadata for a single message ID.
func fetchMessageSummary(ctx context.Context, db *sql.DB, messageID int64) (*query.MessageSummary, error) {
	var s query.MessageSummary
	var sentAt string
	var fromEmail, fromName, sourceConvID sql.NullString

	err := db.QueryRowContext(ctx, `
		SELECT m.id, m.source_message_id, m.conversation_id,
		       COALESCE(c.source_conversation_id, '') AS source_conversation_id,
		       COALESCE(m.subject, '') AS subject,
		       COALESCE(m.snippet, '') AS snippet,
		       COALESCE(p.email_address, '') AS from_email,
		       COALESCE(p.display_name, '') AS from_name,
		       COALESCE(m.sent_at, '') AS sent_at,
		       COALESCE(m.size_estimate, 0) AS size_estimate,
		       COALESCE(m.has_attachments, 0) AS has_attachments,
		       COALESCE(m.attachment_count, 0) AS attachment_count
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		LEFT JOIN participants p ON p.id = m.sender_id
		WHERE m.id = ?
	`, messageID).Scan(
		&s.ID, &s.SourceMessageID, &s.ConversationID,
		&sourceConvID, &s.Subject, &s.Snippet,
		&fromEmail, &fromName,
		&sentAt,
		&s.SizeEstimate, &s.HasAttachments, &s.AttachmentCount,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("message %d not found", messageID)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch message %d: %w", messageID, err)
	}

	if sourceConvID.Valid {
		s.SourceConversationID = sourceConvID.String
	}
	if fromEmail.Valid {
		s.FromEmail = fromEmail.String
	}
	if fromName.Valid {
		s.FromName = fromName.String
	}

	// Parse sent_at — SQLite stores it as text.
	if sentAt != "" {
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05Z",
			time.RFC3339,
			"2006-01-02",
		} {
			if t, err := time.Parse(layout, sentAt); err == nil {
				s.SentAt = t
				break
			}
		}
	}

	return &s, nil
}

// testSemanticSearch is a test-accessible variant that accepts an injectable
// embedding function instead of a real ai.Client.
func testSemanticSearch(
	ctx context.Context,
	callEmbed embeddingFunc,
	s *store.Store,
	queryText string,
	limit int,
) ([]SemanticResult, error) {
	resp, err := callEmbed(ctx, textEmbeddingDeployment, []string{queryText})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding API returned no vectors")
	}
	return semanticSearchWithVec(ctx, s, resp.Data[0].Embedding, queryText, limit)
}
