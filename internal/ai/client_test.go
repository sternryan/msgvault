package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wesm/msgvault/internal/config"
)

// embeddingHandler returns a minimal valid embeddings response.
func embeddingHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		resp := EmbeddingResponse{
			Data: []EmbeddingData{
				{Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
			},
			Usage: Usage{PromptTokens: 10, TotalTokens: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// chatHandler returns a minimal valid chat completions response.
func chatHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		resp := ChatResponse{
			Choices: []ChatChoice{
				{Index: 0, Message: ChatMessage{Role: "assistant", Content: "Hello!"}},
			},
			Usage: Usage{PromptTokens: 5, CompletionTokens: 5, TotalTokens: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func TestNewClient_RequiresEndpoint(t *testing.T) {
	cfg := config.AzureOpenAIConfig{
		Endpoint:  "",
		APIKeyEnv: "MSGVAULT_TEST_KEY_UNUSED",
	}
	t.Setenv("MSGVAULT_TEST_KEY_UNUSED", "test-key")
	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() with empty endpoint should return error")
	}
}

func TestNewClient_ResolvesAPIKeyFromEnv(t *testing.T) {
	srv := httptest.NewServer(embeddingHandler(t))
	defer srv.Close()

	t.Setenv("AZURE_OPENAI_API_KEY", "env-test-key")

	cfg := config.AzureOpenAIConfig{
		Endpoint: srv.URL,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if c.apiKey != "env-test-key" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "env-test-key")
	}
}

func TestNewClient_APIKeyMissing(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv("AZURE_OPENAI_API_KEY")

	cfg := config.AzureOpenAIConfig{
		Endpoint: "https://example.openai.azure.com",
	}
	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() without API key should return error")
	}
}

func TestEmbedding_CorrectURLAndHeaders(t *testing.T) {
	var capturedPath, capturedAPIKey, capturedContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path + "?" + r.URL.RawQuery
		capturedAPIKey = r.Header.Get("api-key")
		capturedContentType = r.Header.Get("Content-Type")
		embeddingHandler(t)(w, r)
	}))
	defer srv.Close()

	t.Setenv("AZURE_OPENAI_API_KEY", "test-api-key-header")
	cfg := config.AzureOpenAIConfig{
		Endpoint: srv.URL,
		Deployments: map[string]string{
			"text-embedding": "text-embedding-3-small",
		},
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	_, err = c.Embedding(context.Background(), "text-embedding", []string{"hello world"})
	if err != nil {
		t.Fatalf("Embedding() error: %v", err)
	}

	expectedPathPrefix := "/openai/deployments/text-embedding-3-small/embeddings"
	if !strings.HasPrefix(capturedPath, expectedPathPrefix) {
		t.Errorf("URL path = %q, want prefix %q", capturedPath, expectedPathPrefix)
	}
	if !strings.Contains(capturedPath, "api-version="+apiVersion) {
		t.Errorf("URL missing api-version, got %q", capturedPath)
	}
	if capturedAPIKey != "test-api-key-header" {
		t.Errorf("api-key header = %q, want %q", capturedAPIKey, "test-api-key-header")
	}
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedContentType)
	}
}

func TestChatCompletion_CorrectURLAndHeaders(t *testing.T) {
	var capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path + "?" + r.URL.RawQuery
		chatHandler(t)(w, r)
	}))
	defer srv.Close()

	t.Setenv("AZURE_OPENAI_API_KEY", "test-chat-key")
	cfg := config.AzureOpenAIConfig{
		Endpoint: srv.URL,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}
	_, err = c.ChatCompletion(context.Background(), "gpt-4o-mini", req)
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	expectedPathPrefix := "/openai/deployments/gpt-4o-mini/chat/completions"
	if !strings.HasPrefix(capturedPath, expectedPathPrefix) {
		t.Errorf("URL path = %q, want prefix %q", capturedPath, expectedPathPrefix)
	}
	if !strings.Contains(capturedPath, "api-version=") {
		t.Errorf("URL missing api-version, got %q", capturedPath)
	}
}

func TestClient_Retry429(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 2 {
			w.WriteHeader(429)
			return
		}
		embeddingHandler(t)(w, r)
	}))
	defer srv.Close()

	t.Setenv("AZURE_OPENAI_API_KEY", "test-retry-key")
	cfg := config.AzureOpenAIConfig{Endpoint: srv.URL}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	// Override httpClient timeout for fast retry in tests
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Should eventually succeed after retrying the 429
	_, err = c.Embedding(context.Background(), "embed", []string{"test"})
	if err != nil {
		t.Errorf("Embedding() with retry on 429 error: %v", err)
	}
	if attempt < 2 {
		t.Errorf("attempt = %d, want >= 2 (should have retried)", attempt)
	}
}

func TestClient_Retry500(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 2 {
			w.WriteHeader(500)
			return
		}
		chatHandler(t)(w, r)
	}))
	defer srv.Close()

	t.Setenv("AZURE_OPENAI_API_KEY", "test-retry500-key")
	cfg := config.AzureOpenAIConfig{Endpoint: srv.URL}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	c.httpClient = &http.Client{Timeout: 5 * time.Second}

	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}
	_, err = c.ChatCompletion(context.Background(), "gpt", req)
	if err != nil {
		t.Errorf("ChatCompletion() with retry on 500 error: %v", err)
	}
	if attempt < 2 {
		t.Errorf("attempt = %d, want >= 2", attempt)
	}
}

func TestClient_NoRetry400(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		w.WriteHeader(400)
		w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer srv.Close()

	t.Setenv("AZURE_OPENAI_API_KEY", "test-400-key")
	cfg := config.AzureOpenAIConfig{Endpoint: srv.URL}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	_, err = c.Embedding(context.Background(), "embed", []string{"test"})
	if err == nil {
		t.Error("Embedding() with 400 should return error immediately")
	}
	if attempt != 1 {
		t.Errorf("attempt = %d, want 1 (no retry on 400)", attempt)
	}
}
