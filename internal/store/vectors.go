package store

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"

	_ "github.com/wesm/msgvault/internal/sqlvec"
)

// VectorEntry holds a message ID and its embedding vector for storage.
type VectorEntry struct {
	MessageID int64
	Embedding []float32 // 1536 dimensions
}

// SemanticResult holds a message ID and its cosine similarity score.
type SemanticResult struct {
	MessageID  int64
	Similarity float64 // 0.0 to 1.0
}

// serializeFloat32 encodes a float32 slice as a little-endian byte slice.
// sqlite-vec expects raw float32 bytes (4 bytes per value, little-endian).
func serializeFloat32(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// InitVectorTable creates the vec_messages virtual table using sqlite-vec.
// It is idempotent: calling it multiple times is safe.
// This must be called after InitSchema() so that the sqlite-vec extension is loaded.
func (s *Store) InitVectorTable() error {
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_messages USING vec0(
			message_id INTEGER PRIMARY KEY,
			embedding FLOAT[1536]
		)
	`)
	if err != nil {
		return fmt.Errorf("create vec_messages virtual table: %w", err)
	}
	return nil
}

// InsertEmbeddings batch-inserts vector entries into vec_messages.
// Idempotent: existing vectors are deleted and re-inserted (sqlite-vec does not
// support INSERT OR REPLACE on vec0 virtual tables in all versions).
// The float32 slice is serialized to raw little-endian bytes as required by sqlite-vec.
func (s *Store) InsertEmbeddings(embeddings []VectorEntry) error {
	if len(embeddings) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	delStmt, err := tx.Prepare(`DELETE FROM vec_messages WHERE message_id = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete: %w", err)
	}
	defer delStmt.Close()

	insStmt, err := tx.Prepare(`INSERT INTO vec_messages(message_id, embedding) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer insStmt.Close()

	for _, e := range embeddings {
		blob := serializeFloat32(e.Embedding)
		if _, err = delStmt.Exec(e.MessageID); err != nil {
			return fmt.Errorf("delete embedding for message %d: %w", e.MessageID, err)
		}
		if _, err = insStmt.Exec(e.MessageID, blob); err != nil {
			return fmt.Errorf("insert embedding for message %d: %w", e.MessageID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit embeddings: %w", err)
	}
	return nil
}

// SearchSemantic performs KNN search against vec_messages using the query vector.
// Returns up to limit results ordered by cosine similarity (highest first).
// Similarity is in [0.0, 1.0]: sqlite-vec returns cosine distance in [0, 2],
// so similarity = 1.0 - (distance / 2.0).
func (s *Store) SearchSemantic(queryEmbedding []float32, limit int) ([]SemanticResult, error) {
	queryBlob := serializeFloat32(queryEmbedding)

	// sqlite-vec requires k= constraint (or LIMIT) to be pushed into the virtual
	// table scan. On older SQLite builds (< 3.38) the LIMIT clause is not pushed
	// down automatically, so we use the explicit k= syntax.
	rows, err := s.db.Query(`
		SELECT message_id, distance
		FROM vec_messages
		WHERE embedding MATCH ?
		  AND k = ?
		ORDER BY distance
	`, queryBlob, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []SemanticResult
	for rows.Next() {
		var r SemanticResult
		var distance float64
		if err := rows.Scan(&r.MessageID, &distance); err != nil {
			return nil, fmt.Errorf("scan vector result: %w", err)
		}
		// Convert cosine distance [0, 2] to similarity [0, 1].
		r.Similarity = 1.0 - (distance / 2.0)
		// Clamp to [0, 1] to handle floating-point edge cases.
		if r.Similarity < 0.0 {
			r.Similarity = 0.0
		}
		if r.Similarity > 1.0 {
			r.Similarity = 1.0
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vector results: %w", err)
	}

	return results, nil
}

// HasEmbedding returns true if a vector embedding exists for the given message ID.
func (s *Store) HasEmbedding(messageID int64) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM vec_messages WHERE message_id = ?`, messageID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has embedding: %w", err)
	}
	return exists == 1, nil
}

// EmbeddingCount returns the total number of embeddings stored in vec_messages.
func (s *Store) EmbeddingCount() (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM vec_messages`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("embedding count: %w", err)
	}
	return count, nil
}
