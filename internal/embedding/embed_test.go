package embedding

import (
	"context"
	"testing"

	"github.com/wesm/msgvault/internal/ai"
)

// TestBuildEmbedText_BothFields verifies "Subject: {subject}\n{snippet}" format.
func TestBuildEmbedText_BothFields(t *testing.T) {
	got := BuildEmbedText("Hello World", "This is the snippet")
	want := "Subject: Hello World\nThis is the snippet"
	if got != want {
		t.Errorf("BuildEmbedText = %q, want %q", got, want)
	}
}

// TestBuildEmbedText_EmptySnippet verifies just "Subject: {subject}" when snippet is empty.
func TestBuildEmbedText_EmptySnippet(t *testing.T) {
	got := BuildEmbedText("Hello World", "")
	want := "Subject: Hello World"
	if got != want {
		t.Errorf("BuildEmbedText (empty snippet) = %q, want %q", got, want)
	}
}

// TestBuildEmbedText_EmptySubject verifies just "{snippet}" when subject is empty.
func TestBuildEmbedText_EmptySubject(t *testing.T) {
	got := BuildEmbedText("", "Just the snippet")
	want := "Just the snippet"
	if got != want {
		t.Errorf("BuildEmbedText (empty subject) = %q, want %q", got, want)
	}
}

// TestBuildEmbedText_Whitespace verifies that whitespace is trimmed.
func TestBuildEmbedText_Whitespace(t *testing.T) {
	got := BuildEmbedText("  Hello  ", "  snippet  ")
	want := "Subject: Hello\nsnippet"
	if got != want {
		t.Errorf("BuildEmbedText (whitespace) = %q, want %q", got, want)
	}
}

// mockEmbeddingClient captures calls to CreateProcessFunc for testing.
type mockEmbeddingResult struct {
	texts    []string
	response *ai.EmbeddingResponse
}

// TestCreateProcessFunc_TokenCounts verifies that BatchResult reports correct token counts.
func TestCreateProcessFunc_TokenCounts(t *testing.T) {
	// Build a mock response with a known usage.
	dim := 4 // small for testing
	resp := &ai.EmbeddingResponse{
		Data: []ai.EmbeddingData{
			{Index: 0, Embedding: []float32{1, 0, 0, 0}},
		},
		Usage: ai.Usage{
			PromptTokens: 42,
			TotalTokens:  42,
		},
	}

	// ProcessFunc factory. We pass a captureFunc to intercept the API call.
	var capturedTexts []string
	callEmbedding := func(_ context.Context, _ string, texts []string) (*ai.EmbeddingResponse, error) {
		capturedTexts = texts
		// Return one embedding per text.
		resp.Data = make([]ai.EmbeddingData, len(texts))
		for i := range texts {
			resp.Data[i] = ai.EmbeddingData{
				Index:     i,
				Embedding: make([]float32, dim),
			}
		}
		return resp, nil
	}

	messages := []ai.MessageRow{
		{ID: 1, Subject: "Test", Snippet: "snippet"},
	}

	result, err := testProcessFunc(callEmbedding, nil, messages)
	if err != nil {
		t.Fatalf("processFunc: %v", err)
	}

	if result.Processed != 1 {
		t.Errorf("Processed: got %d, want 1", result.Processed)
	}
	if result.TokensInput != 42 {
		t.Errorf("TokensInput: got %d, want 42", result.TokensInput)
	}
	_ = capturedTexts // used to avoid "declared and not used"
}

// TestCreateProcessFunc_SkipsExisting verifies that already-embedded messages are skipped.
func TestCreateProcessFunc_SkipsExisting(t *testing.T) {
	// hasEmbedding returns true for message 1
	hasEmbedding := func(id int64) bool { return id == 1 }

	callEmbedding := func(_ context.Context, _ string, texts []string) (*ai.EmbeddingResponse, error) {
		resp := &ai.EmbeddingResponse{
			Data: make([]ai.EmbeddingData, len(texts)),
			Usage: ai.Usage{
				PromptTokens: 10,
				TotalTokens:  10,
			},
		}
		for i := range texts {
			resp.Data[i] = ai.EmbeddingData{Index: i, Embedding: make([]float32, 4)}
		}
		return resp, nil
	}

	messages := []ai.MessageRow{
		{ID: 1, Subject: "Existing", Snippet: "already embedded"},
		{ID: 2, Subject: "New", Snippet: "not yet embedded"},
	}

	result, err := testProcessFuncWithSkip(callEmbedding, hasEmbedding, messages)
	if err != nil {
		t.Fatalf("processFunc: %v", err)
	}

	if result.Processed != 1 {
		t.Errorf("Processed: got %d, want 1", result.Processed)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped: got %d, want 1", result.Skipped)
	}
}

// TestCreateQueryFunc_OrderedByID verifies that query returns messages ordered by ID ascending
// starting after afterID.
func TestCreateQueryFunc_OrderedByID(t *testing.T) {
	messages := []ai.MessageRow{
		{ID: 1, Subject: "A"},
		{ID: 2, Subject: "B"},
		{ID: 3, Subject: "C"},
		{ID: 4, Subject: "D"},
		{ID: 5, Subject: "E"},
	}

	queryFn := buildTestQueryFunc(messages)

	// Start after ID 2, limit 2
	got, err := queryFn(2, 2)
	if err != nil {
		t.Fatalf("queryFn: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	if got[0].ID != 3 {
		t.Errorf("got[0].ID = %d, want 3", got[0].ID)
	}
	if got[1].ID != 4 {
		t.Errorf("got[1].ID = %d, want 4", got[1].ID)
	}
}
