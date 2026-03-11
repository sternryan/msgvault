package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/search"
)

// mockEngine implements query.Engine with sensible test data.
type mockEngine struct{}

func (m *mockEngine) Aggregate(_ context.Context, _ query.ViewType, _ query.AggregateOptions) ([]query.AggregateRow, error) {
	return []query.AggregateRow{
		{Key: "alice@example.com", Count: 42, TotalSize: 10240},
		{Key: "bob@example.com", Count: 17, TotalSize: 5120},
		{Key: "carol@example.com", Count: 8, TotalSize: 2048},
	}, nil
}

func (m *mockEngine) SubAggregate(_ context.Context, _ query.MessageFilter, _ query.ViewType, _ query.AggregateOptions) ([]query.AggregateRow, error) {
	return []query.AggregateRow{
		{Key: "example.com", Count: 10, TotalSize: 4096},
	}, nil
}

func (m *mockEngine) ListMessages(_ context.Context, _ query.MessageFilter) ([]query.MessageSummary, error) {
	now := time.Now()
	return []query.MessageSummary{
		{ID: 1, Subject: "Test Subject One", FromEmail: "alice@example.com", SentAt: now},
		{ID: 2, Subject: "Test Subject Two", FromEmail: "bob@example.com", SentAt: now},
		{ID: 3, Subject: "Test Subject Three", FromEmail: "carol@example.com", SentAt: now},
	}, nil
}

func (m *mockEngine) GetMessage(_ context.Context, id int64) (*query.MessageDetail, error) {
	if id == 99999 {
		return nil, nil
	}
	return &query.MessageDetail{
		ID:      id,
		Subject: "Test Message Subject",
		From:    []query.Address{{Email: "alice@example.com", Name: "Alice"}},
		To:      []query.Address{{Email: "bob@example.com", Name: "Bob"}},
		BodyText: "This is the test message body.",
		SentAt:  time.Now(),
	}, nil
}

func (m *mockEngine) GetMessageBySourceID(_ context.Context, _ string) (*query.MessageDetail, error) {
	return nil, nil
}

func (m *mockEngine) GetAttachment(_ context.Context, _ int64) (*query.AttachmentInfo, error) {
	return nil, nil
}

func (m *mockEngine) Search(_ context.Context, _ *search.Query, _, _ int) ([]query.MessageSummary, error) {
	return []query.MessageSummary{
		{ID: 1, Subject: "Search Result", FromEmail: "alice@example.com"},
	}, nil
}

func (m *mockEngine) SearchFast(_ context.Context, _ *search.Query, _ query.MessageFilter, _, _ int) ([]query.MessageSummary, error) {
	return []query.MessageSummary{
		{ID: 1, Subject: "Fast Search Result", FromEmail: "alice@example.com"},
	}, nil
}

func (m *mockEngine) SearchFastCount(_ context.Context, _ *search.Query, _ query.MessageFilter) (int64, error) {
	return 1, nil
}

func (m *mockEngine) SearchFastWithStats(_ context.Context, _ *search.Query, _ string, _ query.MessageFilter, _ query.ViewType, _, _ int) (*query.SearchFastResult, error) {
	return &query.SearchFastResult{
		Messages:   []query.MessageSummary{{ID: 1, Subject: "Fast Search Result"}},
		TotalCount: 1,
		Stats:      &query.TotalStats{MessageCount: 100},
	}, nil
}

func (m *mockEngine) GetGmailIDsByFilter(_ context.Context, _ query.MessageFilter) ([]string, error) {
	return []string{"msg1", "msg2"}, nil
}

func (m *mockEngine) ListAccounts(_ context.Context) ([]query.AccountInfo, error) {
	return []query.AccountInfo{
		{ID: 1, SourceType: "gmail", Identifier: "test@example.com", DisplayName: "Test"},
	}, nil
}

func (m *mockEngine) GetTotalStats(_ context.Context, _ query.StatsOptions) (*query.TotalStats, error) {
	return &query.TotalStats{
		MessageCount:    100,
		TotalSize:       1024000,
		AttachmentCount: 25,
		AttachmentSize:  512000,
		LabelCount:      10,
		AccountCount:    1,
	}, nil
}

func (m *mockEngine) Close() error {
	return nil
}

// setupTestServer creates an httptest.Server using the same router as the production server.
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	deletionsDir := t.TempDir()
	delMgr, err := deletion.NewManager(deletionsDir)
	if err != nil {
		t.Fatalf("failed to create deletion manager: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := NewServer(&mockEngine{}, "", delMgr, logger)
	router := srv.buildRouter()

	return httptest.NewServer(router)
}

// TestHandlersReturnHTML verifies every GET page route returns 200 with HTML.
func TestHandlersReturnHTML(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	routes := []struct {
		name string
		path string
	}{
		{"dashboard", "/"},
		{"messages", "/messages"},
		{"aggregate", "/aggregate"},
		{"search", "/search"},
		{"deletions", "/deletions"},
	}

	for _, tc := range routes {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s: expected status 200, got %d", tc.path, resp.StatusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Errorf("GET %s: expected Content-Type text/html, got %q", tc.path, ct)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading body: %v", err)
			}
			bodyStr := string(body)

			if !strings.Contains(bodyStr, "<html") {
				t.Errorf("GET %s: body does not contain <html", tc.path)
			}
			if !strings.Contains(bodyStr, "msgvault") {
				t.Errorf("GET %s: body does not contain 'msgvault'", tc.path)
			}
		})
	}
}

// TestStaticFiles verifies static assets are served correctly.
func TestStaticFiles(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	cases := []struct {
		name            string
		path            string
		wantContentType string
	}{
		{"style.css", "/static/style.css", "text/css"},
		{"htmx.min.js", "/static/htmx.min.js", "javascript"},
		{"keys.js", "/static/keys.js", "javascript"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s: expected status 200, got %d", tc.path, resp.StatusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, tc.wantContentType) {
				t.Errorf("GET %s: expected Content-Type containing %q, got %q", tc.path, tc.wantContentType, ct)
			}
		})
	}
}

// TestDashboard verifies the dashboard renders stat data.
func TestDashboard(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /: expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "test@example.com") && !strings.Contains(bodyStr, "Test") {
		t.Errorf("dashboard body should contain account name (test@example.com or Test)")
	}
}

// TestMessages verifies the messages list renders.
func TestMessages(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	cases := []string{
		"/messages",
		"/messages?sortField=date&sortDir=desc",
	}

	for _, path := range cases {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: expected status 200, got %d", path, resp.StatusCode)
		}
	}
}

// TestMessageDetail verifies the message detail page renders.
func TestMessageDetail(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/messages/1")
	if err != nil {
		t.Fatalf("GET /messages/1: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /messages/1: expected status 200, got %d", resp.StatusCode)
	}
}

// TestAggregate verifies the aggregate page renders.
func TestAggregate(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	cases := []string{
		"/aggregate",
		"/aggregate?groupBy=domains",
	}

	for _, path := range cases {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: expected status 200, got %d", path, resp.StatusCode)
		}
	}
}

// TestSearch verifies the search page renders.
func TestSearch(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	cases := []string{
		"/search",
		"/search?q=test",
	}

	for _, path := range cases {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: expected status 200, got %d", path, resp.StatusCode)
		}
	}
}

// TestStageDeletion verifies the deletion staging POST works.
func TestStageDeletion(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/deletions/stage", nil)
	if err != nil {
		t.Fatalf("POST /deletions/stage: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /deletions/stage: expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("POST /deletions/stage: expected Content-Type text/html, got %q", ct)
	}
}

// TestAccountFilter verifies sourceId param is accepted and pages render.
func TestAccountFilter(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	cases := []string{
		"/?sourceId=1",
		"/messages?sourceId=1",
		"/aggregate?sourceId=1",
	}

	for _, path := range cases {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: expected status 200, got %d", path, resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		// Verify the account filter select is present in the layout
		if !strings.Contains(bodyStr, "account-filter") {
			t.Errorf("GET %s: body should contain account-filter select element", path)
		}
	}
}
