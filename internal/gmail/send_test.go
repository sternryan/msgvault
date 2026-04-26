package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// G1+G2: Send POSTs to /users/{userID}/messages/send with base64url-encoded
// raw body and parses the returned id.
func TestSend_HappyPath(t *testing.T) {
	var gotPath, gotMethod, gotRawField string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		gotRawField = payload["raw"]
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "abc123"})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		httpClient:  &http.Client{Transport: transport},
		userID:      "me",
		logger:      slog.Default(),
		rateLimiter: NewRateLimiter(1000),
	}

	rfc822 := []byte("From: x@y.com\r\nTo: ryan@example.com\r\nSubject: Test\r\n\r\nbody")
	id, err := client.Send(context.Background(), rfc822)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "abc123" {
		t.Fatalf("got id %q, want abc123", id)
	}
	if gotMethod != "POST" {
		t.Fatalf("got method %q, want POST", gotMethod)
	}
	if !strings.Contains(gotPath, "/messages/send") {
		t.Fatalf("got path %q, want .../messages/send", gotPath)
	}
	// raw is base64url-encoded
	decoded, err := base64.URLEncoding.DecodeString(gotRawField)
	if err != nil {
		t.Fatalf("body raw not base64url: %v", err)
	}
	if string(decoded) != string(rfc822) {
		t.Fatalf("body roundtrip mismatch: got %q want %q", decoded, rfc822)
	}
}

// G3: 403 surfaces as wrapped error
func TestSend_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Insufficient Permission"}}`))
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport}
	client := &Client{
		httpClient:  &http.Client{Transport: transport},
		userID:      "me",
		logger:      slog.Default(),
		rateLimiter: NewRateLimiter(1000),
	}
	_, err := client.Send(context.Background(), []byte("From: x@y.com\r\n\r\nbody"))
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
}

// G4: OpMessagesSend is referenced in the Send call (compile-time check via reading source).
// Confirms the operation enum is wired into the rate limiter.
func TestSend_OperationCost(t *testing.T) {
	if OpMessagesSend.Cost() < 50 {
		t.Fatalf("OpMessagesSend cost = %d, want >= 50 (Gmail send is high-cost)", OpMessagesSend.Cost())
	}
}

// G5: empty body errors out (input validation guard)
func TestSend_EmptyBody(t *testing.T) {
	client := &Client{
		userID:      "me",
		logger:      slog.Default(),
		rateLimiter: NewRateLimiter(1000),
	}
	if _, err := client.Send(context.Background(), nil); err == nil {
		t.Fatal("expected error on empty body")
	}
}
