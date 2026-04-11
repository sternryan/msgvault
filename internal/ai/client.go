package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wesm/msgvault/internal/config"
)

const (
	apiVersion     = "2024-10-21"
	maxRetries     = 5
	defaultTimeout = 60 * time.Second
)

// Client communicates with Azure OpenAI REST API.
type Client struct {
	httpClient  *http.Client
	endpoint    string // e.g. https://myinstance.openai.azure.com
	apiKey      string
	deployments map[string]string // logical -> deployment name
	rateLimiter *RateLimiter
	logger      *slog.Logger
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) { c.logger = logger }
}

// WithRateLimiter sets a custom rate limiter.
func WithRateLimiter(rl *RateLimiter) ClientOption {
	return func(c *Client) { c.rateLimiter = rl }
}

// NewClient creates an Azure OpenAI client from config.
// Resolves the API key immediately and returns an error if unavailable.
func NewClient(cfg config.AzureOpenAIConfig, opts ...ClientOption) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("azure_openai.endpoint is required")
	}

	apiKey, err := cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}

	c := &Client{
		httpClient:  &http.Client{Timeout: defaultTimeout},
		endpoint:    strings.TrimRight(cfg.Endpoint, "/"),
		apiKey:      apiKey,
		deployments: cfg.Deployments,
		rateLimiter: NewRateLimiter(cfg.TPMLimit, cfg.RPMLimit),
		logger:      slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// EmbeddingRequest is the request body for the embeddings endpoint.
type EmbeddingRequest struct {
	Input []string `json:"input"`
}

// EmbeddingResponse is the response from the embeddings endpoint.
type EmbeddingResponse struct {
	Data  []EmbeddingData `json:"data"`
	Usage Usage           `json:"usage"`
}

// EmbeddingData holds a single embedding vector.
type EmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// ChatRequest is the request body for chat completions.
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatResponse is the response from chat completions.
type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

// ChatChoice represents a single completion choice.
type ChatChoice struct {
	Index   int         `json:"index"`
	Message ChatMessage `json:"message"`
}

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Embedding calls the embeddings endpoint for the given deployment.
// deployment is the logical name (mapped via config deployments).
func (c *Client) Embedding(ctx context.Context, deployment string, input []string) (*EmbeddingResponse, error) {
	deplName := c.deploymentName(deployment)
	url := fmt.Sprintf("%s/openai/deployments/%s/embeddings?api-version=%s", c.endpoint, deplName, apiVersion)

	// Estimate tokens: ~4 chars per token for English text
	estimatedTokens := 0
	for _, s := range input {
		estimatedTokens += len(s) / 4
	}
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}

	body := EmbeddingRequest{Input: input}
	var resp EmbeddingResponse
	if err := c.doRequest(ctx, "POST", url, body, &resp, estimatedTokens); err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}

	// Reconcile actual tokens with estimate
	c.rateLimiter.RecordActualTokens(estimatedTokens, resp.Usage.TotalTokens)

	return &resp, nil
}

// ChatCompletion calls the chat completions endpoint.
// deployment is the logical name (mapped via config deployments).
func (c *Client) ChatCompletion(ctx context.Context, deployment string, req ChatRequest) (*ChatResponse, error) {
	deplName := c.deploymentName(deployment)
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", c.endpoint, deplName, apiVersion)

	// Estimate tokens from input messages
	estimatedTokens := 0
	for _, m := range req.Messages {
		estimatedTokens += len(m.Content) / 4
	}
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}

	var resp ChatResponse
	if err := c.doRequest(ctx, "POST", url, req, &resp, estimatedTokens); err != nil {
		return nil, fmt.Errorf("chat completion request: %w", err)
	}

	c.rateLimiter.RecordActualTokens(estimatedTokens, resp.Usage.TotalTokens)

	return &resp, nil
}

// doRequest executes an HTTP request with rate limiting, retry, and error handling.
func (c *Client) doRequest(ctx context.Context, method, url string, body interface{}, result interface{}, estimatedTokens int) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Rate limit
		if err := c.rateLimiter.Wait(ctx, estimatedTokens); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("api-key", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("request failed, retrying", "attempt", attempt, "err", err)
			time.Sleep(backoff(attempt))
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 {
			// Rate limited by Azure — back off
			retryAfter := time.Duration(backoffSeconds(attempt)) * time.Second
			c.logger.Warn("rate limited by Azure, backing off",
				"status", resp.StatusCode, "retry_after", retryAfter)
			lastErr = fmt.Errorf("rate limited (429)")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryAfter):
			}
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
			c.logger.Warn("server error, retrying", "status", resp.StatusCode, "attempt", attempt)
			time.Sleep(backoff(attempt))
			continue
		}

		if resp.StatusCode != 200 {
			return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		}

		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}

		return nil
	}

	return fmt.Errorf("all %d retries exhausted: %w", maxRetries, lastErr)
}

func (c *Client) deploymentName(logical string) string {
	if d, ok := c.deployments[logical]; ok {
		return d
	}
	return logical
}

func backoff(attempt int) time.Duration {
	return time.Duration(backoffSeconds(attempt)) * time.Second
}

func backoffSeconds(attempt int) int {
	base := 1 << attempt // 1, 2, 4, 8, 16
	if base > 60 {
		base = 60
	}
	return base
}
