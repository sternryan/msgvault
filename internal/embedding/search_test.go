package embedding

import (
	"context"
	"testing"
	"time"

	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/query"
	_ "github.com/wesm/msgvault/internal/sqlvec"
	"github.com/wesm/msgvault/internal/store"
)

// openSearchTestStore opens an in-memory store for search tests.
func openSearchTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.InitSchema(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return s
}

// insertTestMessage inserts a minimal message into the DB for search tests.
// Returns the message ID.
// Note: uses last_insert_rowid() instead of RETURNING because go-sqlcipher
// bundles SQLite 3.33.0 which predates the RETURNING clause (added 3.35.0).
func insertTestMessage(t *testing.T, s *store.Store, subject, snippet string) int64 {
	t.Helper()
	db := s.DB()

	// Insert a source (idempotent).
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO sources(source_type, identifier, display_name)
		VALUES ('gmail', 'test@example.com', 'Test User')
	`); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	var sourceID int64
	if err := db.QueryRow(`SELECT id FROM sources WHERE identifier='test@example.com'`).Scan(&sourceID); err != nil {
		t.Fatalf("get source id: %v", err)
	}

	// Insert a conversation.
	if _, err := db.Exec(`
		INSERT INTO conversations(source_id, source_conversation_id, conversation_type, title)
		VALUES (?, ?, 'email_thread', ?)
	`, sourceID, subject+"-conv", subject); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	var convID int64
	if err := db.QueryRow(`SELECT last_insert_rowid()`).Scan(&convID); err != nil {
		t.Fatalf("get conversation id: %v", err)
	}

	// Insert a message.
	if _, err := db.Exec(`
		INSERT INTO messages(conversation_id, source_id, source_message_id, message_type, subject, snippet, sent_at)
		VALUES (?, ?, ?, 'email', ?, ?, ?)
	`, convID, sourceID, subject+"-msg", subject, snippet, time.Now().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	var msgID int64
	if err := db.QueryRow(`SELECT last_insert_rowid()`).Scan(&msgID); err != nil {
		t.Fatalf("get message id: %v", err)
	}
	return msgID
}

// TestSemanticSearch_EnrichesWithMetadata verifies that SemanticSearch enriches
// vector results with message metadata (Subject, Snippet) from SQLite.
func TestSemanticSearch_EnrichesWithMetadata(t *testing.T) {
	s := openSearchTestStore(t)
	msgID := insertTestMessage(t, s, "Hello World", "Test snippet")

	// Insert a vector for the message.
	vec := make([]float32, 1536)
	vec[0] = 1.0
	if err := s.InsertEmbeddings([]store.VectorEntry{
		{MessageID: msgID, Embedding: vec},
	}); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	// Mock client that returns the same vector as the query.
	callEmbedding := func(_ context.Context, _ string, texts []string) (*ai.EmbeddingResponse, error) {
		return &ai.EmbeddingResponse{
			Data: []ai.EmbeddingData{
				{Index: 0, Embedding: vec},
			},
			Usage: ai.Usage{PromptTokens: 5},
		}, nil
	}

	results, err := testSemanticSearch(context.Background(), callEmbedding, s, "hello world", 10)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("SemanticSearch returned no results")
	}

	if results[0].Subject != "Hello World" {
		t.Errorf("Subject = %q, want %q", results[0].Subject, "Hello World")
	}
}

// TestSemanticSearch_SimilarityPercentage verifies that SimilarityPct is in 0-100 range.
func TestSemanticSearch_SimilarityPercentage(t *testing.T) {
	s := openSearchTestStore(t)
	msgID := insertTestMessage(t, s, "Test", "snippet")

	vec := make([]float32, 1536)
	vec[0] = 1.0
	if err := s.InsertEmbeddings([]store.VectorEntry{
		{MessageID: msgID, Embedding: vec},
	}); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	callEmbedding := func(_ context.Context, _ string, texts []string) (*ai.EmbeddingResponse, error) {
		return &ai.EmbeddingResponse{
			Data:  []ai.EmbeddingData{{Index: 0, Embedding: vec}},
			Usage: ai.Usage{PromptTokens: 5},
		}, nil
	}

	results, err := testSemanticSearch(context.Background(), callEmbedding, s, "test", 10)
	if err != nil {
		t.Fatalf("SemanticSearch: %v", err)
	}

	for _, r := range results {
		if r.SimilarityPct < 0 || r.SimilarityPct > 100 {
			t.Errorf("SimilarityPct out of range: %f for message %d", r.SimilarityPct, r.MessageSummary.ID)
		}
	}
}

// Compile-time check that SemanticResult embeds query.MessageSummary.
var _ query.MessageSummary = SemanticResult{}.MessageSummary
