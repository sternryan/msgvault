package enrichment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wesm/msgvault/internal/ai"
)

// TestParseEnrichResponse_Valid verifies parsing of a well-formed JSON response.
func TestParseEnrichResponse_Valid(t *testing.T) {
	input := `{"category":"finance","life_events":[{"date":"2024-06-01","type":"purchase","description":"Bought a car"}],"entities":[{"type":"company","value":"Ford Motor"}]}`
	result, err := parseEnrichResponse(input)
	if err != nil {
		t.Fatalf("parseEnrichResponse: %v", err)
	}
	if result.Category != "finance" {
		t.Errorf("Category = %q, want %q", result.Category, "finance")
	}
	if len(result.LifeEvents) != 1 {
		t.Fatalf("LifeEvents len = %d, want 1", len(result.LifeEvents))
	}
	if result.LifeEvents[0].Type != "purchase" {
		t.Errorf("LifeEvents[0].Type = %q, want %q", result.LifeEvents[0].Type, "purchase")
	}
	if len(result.Entities) != 1 {
		t.Fatalf("Entities len = %d, want 1", len(result.Entities))
	}
	if result.Entities[0].Value != "Ford Motor" {
		t.Errorf("Entities[0].Value = %q, want %q", result.Entities[0].Value, "Ford Motor")
	}
}

// TestParseEnrichResponse_MarkdownFences verifies stripping of ```json ... ``` fences.
func TestParseEnrichResponse_MarkdownFences(t *testing.T) {
	input := "```json\n{\"category\":\"travel\",\"life_events\":[],\"entities\":[]}\n```"
	result, err := parseEnrichResponse(input)
	if err != nil {
		t.Fatalf("parseEnrichResponse (markdown): %v", err)
	}
	if result.Category != "travel" {
		t.Errorf("Category = %q, want %q", result.Category, "travel")
	}
}

// TestParseEnrichResponse_Unparseable verifies that a completely invalid response returns an error.
func TestParseEnrichResponse_Unparseable(t *testing.T) {
	_, err := parseEnrichResponse("I cannot categorize this email as it contains no useful content.")
	if err == nil {
		t.Error("expected error for unparseable response, got nil")
	}
}

// TestBuildEnrichRequest_ContainsSubjectAndSnippet verifies the request structure.
func TestBuildEnrichRequest_ContainsSubjectAndSnippet(t *testing.T) {
	subject := "Your flight to Tokyo"
	snippet := "Your booking is confirmed for March 15th"
	req := buildEnrichRequest(subject, snippet)

	if len(req.Messages) < 2 {
		t.Fatalf("ChatRequest.Messages len = %d, want >= 2 (system + user)", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("Messages[0].Role = %q, want %q", req.Messages[0].Role, "system")
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("Messages[1].Role = %q, want %q", req.Messages[1].Role, "user")
	}
	userContent := req.Messages[1].Content
	if !strings.Contains(userContent, subject) {
		t.Errorf("user message does not contain subject %q: %q", subject, userContent)
	}
	if !strings.Contains(userContent, snippet) {
		t.Errorf("user message does not contain snippet %q: %q", snippet, userContent)
	}
}

// TestValidateCategory_InvalidDefaultsToPersonal verifies the fallback behavior.
func TestValidateCategory_InvalidDefaultsToPersonal(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"finance", "finance"},
		{"travel", "travel"},
		{"legal", "legal"},
		{"health", "health"},
		{"shopping", "shopping"},
		{"newsletters", "newsletters"},
		{"personal", "personal"},
		{"work", "work"},
		{"unknown-value", "personal"},
		{"FINANCE", "personal"}, // case sensitive
		{"", "personal"},
		{"spam", "personal"},
	}
	for _, tc := range cases {
		got := validateCategory(tc.input)
		if got != tc.want {
			t.Errorf("validateCategory(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestProcessFunc_SkipsAlreadyCategorized verifies idempotency (already-labeled messages skipped).
func TestProcessFunc_SkipsAlreadyCategorized(t *testing.T) {
	callCount := 0
	mockChat := func(_ context.Context, _ string, _ ai.ChatRequest) (*ai.ChatResponse, error) {
		callCount++
		return &ai.ChatResponse{
			Choices: []ai.ChatChoice{{Message: ai.ChatMessage{Role: "assistant", Content: `{"category":"finance","life_events":[],"entities":[]}`}}},
			Usage:   ai.Usage{PromptTokens: 10, CompletionTokens: 5},
		}, nil
	}

	// Message 1 is already categorized (has auto label), message 2 is not.
	isCategorized := func(id int64) bool { return id == 1 }

	processFn := testProcessFuncWithSkip(mockChat, isCategorized)
	messages := []ai.MessageRow{
		{ID: 1, Subject: "Already done", Snippet: "skip me"},
		{ID: 2, Subject: "New message", Snippet: "process me"},
	}
	result, err := processFn(context.Background(), messages)
	if err != nil {
		t.Fatalf("ProcessFunc: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if callCount != 1 {
		t.Errorf("ChatCompletion called %d times, want 1", callCount)
	}
}

// TestProcessFunc_FailsGracefullyOnLLMError verifies that per-message failures don't abort the batch.
func TestProcessFunc_FailsGracefullyOnLLMError(t *testing.T) {
	callCount := 0
	mockChat := func(_ context.Context, _ string, _ ai.ChatRequest) (*ai.ChatResponse, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("azure openai: rate limit exceeded")
		}
		return &ai.ChatResponse{
			Choices: []ai.ChatChoice{{Message: ai.ChatMessage{Role: "assistant", Content: `{"category":"work","life_events":[],"entities":[]}`}}},
			Usage:   ai.Usage{PromptTokens: 10, CompletionTokens: 5},
		}, nil
	}

	writeResults := func(_ int64, _ *EnrichResult) error { return nil }

	processFn := testProcessFuncWithWrite(mockChat, writeResults)
	messages := []ai.MessageRow{
		{ID: 1, Subject: "Failed message", Snippet: "this will error"},
		{ID: 2, Subject: "Good message", Snippet: "this will succeed"},
	}
	result, err := processFn(context.Background(), messages)
	if err != nil {
		t.Fatalf("ProcessFunc should not return error on per-message failure: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if result.Processed != 1 {
		t.Errorf("Processed = %d, want 1", result.Processed)
	}
	if callCount != 2 {
		t.Errorf("ChatCompletion called %d times, want 2", callCount)
	}
}

// TestNormalizeEntityValue verifies company suffix stripping.
func TestNormalizeEntityValue(t *testing.T) {
	cases := []struct {
		entityType string
		value      string
		want       string
	}{
		{"company", "Apple Inc.", "apple"},
		{"company", "Google LLC", "google"},
		{"company", "Microsoft Corp.", "microsoft"},
		{"company", "Amazon.com Inc", "amazon.com"},
		{"person", "  Tim Cook  ", "tim cook"},
		{"date", "January 15, 2024", "january 15, 2024"},
	}
	for _, tc := range cases {
		got := normalizeEntityValue(tc.entityType, tc.value)
		if got != tc.want {
			t.Errorf("normalizeEntityValue(%q, %q) = %q, want %q", tc.entityType, tc.value, got, tc.want)
		}
	}
}

// testProcessFuncWithSkip is a test helper that creates a ProcessFunc with custom isCategorized.
func testProcessFuncWithSkip(
	chatFn func(ctx context.Context, deployment string, req ai.ChatRequest) (*ai.ChatResponse, error),
	isCategorized func(id int64) bool,
) ai.ProcessFunc {
	writeResults := func(_ int64, _ *EnrichResult) error { return nil }
	return buildEnrichProcessFuncTestable(chatFn, isCategorized, writeResults, "chat", nil)
}

// testProcessFuncWithWrite is a test helper that creates a ProcessFunc with custom writeResults.
func testProcessFuncWithWrite(
	chatFn func(ctx context.Context, deployment string, req ai.ChatRequest) (*ai.ChatResponse, error),
	writeResults func(messageID int64, result *EnrichResult) error,
) ai.ProcessFunc {
	isCategorized := func(_ int64) bool { return false }
	return buildEnrichProcessFuncTestable(chatFn, isCategorized, writeResults, "chat", nil)
}

// Ensure the package compiles with a format reference to avoid unused import warning.
var _ = fmt.Sprintf
