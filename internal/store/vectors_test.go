package store

import (
	"math"
	"testing"

	_ "github.com/wesm/msgvault/internal/sqlvec"
)

// makeTestVector returns a unit-normalized float32 slice of the given dimension.
// index controls direction: index 0 points along the first axis, etc.
func makeTestVector(dim int, dominant int) []float32 {
	v := make([]float32, dim)
	v[dominant] = 1.0
	return v
}

// makeGradientVector returns a vector where all values are equal and normalized.
func makeGradientVector(dim int, scale float32) []float32 {
	v := make([]float32, dim)
	sum := float64(0)
	for i := range v {
		v[i] = scale
		sum += float64(scale) * float64(scale)
	}
	norm := float32(math.Sqrt(sum))
	for i := range v {
		v[i] /= norm
	}
	return v
}

func openVectorTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.InitSchema(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return s
}

// TestVectorInitVectorTable verifies that InitVectorTable creates the vec_messages
// virtual table without error on a fresh database.
func TestVectorInitVectorTable(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}
}

// TestVectorInitVectorTableIdempotent verifies that calling InitVectorTable
// twice does not produce an error.
func TestVectorInitVectorTableIdempotent(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("first InitVectorTable: %v", err)
	}
	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("second InitVectorTable: %v", err)
	}
}

// TestVectorInsertAndRetrieve verifies that InsertEmbeddings stores N vectors
// and they can be found via EmbeddingCount.
func TestVectorInsertAndRetrieve(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	const dim = 1536
	entries := []VectorEntry{
		{MessageID: 1, Embedding: makeTestVector(dim, 0)},
		{MessageID: 2, Embedding: makeTestVector(dim, 1)},
		{MessageID: 3, Embedding: makeTestVector(dim, 2)},
	}

	if err := s.InsertEmbeddings(entries); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	count, err := s.EmbeddingCount()
	if err != nil {
		t.Fatalf("EmbeddingCount: %v", err)
	}
	if count != 3 {
		t.Errorf("EmbeddingCount: got %d, want 3", count)
	}
}

// TestVectorSearchSemantic verifies that SearchSemantic returns results ordered
// by cosine similarity (highest first).
func TestVectorSearchSemantic(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	const dim = 1536

	// Insert three orthogonal unit vectors.
	entries := []VectorEntry{
		{MessageID: 1, Embedding: makeTestVector(dim, 0)}, // points along axis 0
		{MessageID: 2, Embedding: makeTestVector(dim, 1)}, // points along axis 1
		{MessageID: 3, Embedding: makeTestVector(dim, 2)}, // points along axis 2
	}
	if err := s.InsertEmbeddings(entries); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	// Query with a vector closest to message 1 (pointing along axis 0).
	queryVec := makeTestVector(dim, 0)
	results, err := s.SearchSemantic(queryVec, 3)
	if err != nil {
		t.Fatalf("SearchSemantic: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("SearchSemantic returned no results")
	}

	// The first result should be message 1 (identical vector, highest similarity).
	if results[0].MessageID != 1 {
		t.Errorf("first result: got message_id=%d, want 1", results[0].MessageID)
	}

	// Results should be in descending similarity order.
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not sorted by similarity: results[%d].Similarity=%f > results[%d].Similarity=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
	}
}

// TestVectorSearchSemanticLimit verifies that SearchSemantic respects the limit parameter.
func TestVectorSearchSemanticLimit(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	const dim = 1536

	entries := []VectorEntry{
		{MessageID: 1, Embedding: makeTestVector(dim, 0)},
		{MessageID: 2, Embedding: makeTestVector(dim, 1)},
		{MessageID: 3, Embedding: makeTestVector(dim, 2)},
		{MessageID: 4, Embedding: makeTestVector(dim, 3)},
		{MessageID: 5, Embedding: makeTestVector(dim, 4)},
	}
	if err := s.InsertEmbeddings(entries); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	results, err := s.SearchSemantic(makeTestVector(dim, 0), 2)
	if err != nil {
		t.Fatalf("SearchSemantic: %v", err)
	}

	if len(results) > 2 {
		t.Errorf("SearchSemantic limit=2: got %d results, want <= 2", len(results))
	}
}

// TestVectorSearchSemanticSimilarityRange verifies that SearchSemantic returns
// similarity values in [0.0, 1.0].
func TestVectorSearchSemanticSimilarityRange(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	const dim = 1536

	entries := []VectorEntry{
		{MessageID: 1, Embedding: makeTestVector(dim, 0)},
		{MessageID: 2, Embedding: makeTestVector(dim, 1)},
	}
	if err := s.InsertEmbeddings(entries); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	results, err := s.SearchSemantic(makeTestVector(dim, 0), 10)
	if err != nil {
		t.Fatalf("SearchSemantic: %v", err)
	}

	for _, r := range results {
		if r.Similarity < 0.0 || r.Similarity > 1.0 {
			t.Errorf("similarity out of range [0,1]: got %f for message_id=%d",
				r.Similarity, r.MessageID)
		}
	}
}

// TestVectorHasEmbedding verifies HasEmbedding returns true for embedded messages
// and false for others.
func TestVectorHasEmbedding(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	const dim = 1536

	entries := []VectorEntry{
		{MessageID: 10, Embedding: makeTestVector(dim, 0)},
	}
	if err := s.InsertEmbeddings(entries); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	tests := []struct {
		messageID int64
		want      bool
	}{
		{10, true},
		{99, false},
	}
	for _, tt := range tests {
		got, err := s.HasEmbedding(tt.messageID)
		if err != nil {
			t.Fatalf("HasEmbedding(%d): %v", tt.messageID, err)
		}
		if got != tt.want {
			t.Errorf("HasEmbedding(%d): got %v, want %v", tt.messageID, got, tt.want)
		}
	}
}

// TestVectorEmbeddingCount verifies EmbeddingCount returns the correct count.
func TestVectorEmbeddingCount(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	// Initially zero.
	count, err := s.EmbeddingCount()
	if err != nil {
		t.Fatalf("EmbeddingCount (empty): %v", err)
	}
	if count != 0 {
		t.Errorf("EmbeddingCount (empty): got %d, want 0", count)
	}

	const dim = 1536

	entries := []VectorEntry{
		{MessageID: 1, Embedding: makeTestVector(dim, 0)},
		{MessageID: 2, Embedding: makeTestVector(dim, 1)},
	}
	if err := s.InsertEmbeddings(entries); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	count, err = s.EmbeddingCount()
	if err != nil {
		t.Fatalf("EmbeddingCount: %v", err)
	}
	if count != 2 {
		t.Errorf("EmbeddingCount: got %d, want 2", count)
	}
}

// TestVectorInsertIdempotent verifies that InsertEmbeddings with INSERT OR REPLACE
// does not create duplicates.
func TestVectorInsertIdempotent(t *testing.T) {
	s := openVectorTestStore(t)

	if err := s.InitVectorTable(); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	const dim = 1536

	entry := []VectorEntry{
		{MessageID: 1, Embedding: makeTestVector(dim, 0)},
	}

	if err := s.InsertEmbeddings(entry); err != nil {
		t.Fatalf("first InsertEmbeddings: %v", err)
	}
	if err := s.InsertEmbeddings(entry); err != nil {
		t.Fatalf("second InsertEmbeddings: %v", err)
	}

	count, err := s.EmbeddingCount()
	if err != nil {
		t.Fatalf("EmbeddingCount: %v", err)
	}
	if count != 1 {
		t.Errorf("EmbeddingCount after duplicate insert: got %d, want 1", count)
	}
}
