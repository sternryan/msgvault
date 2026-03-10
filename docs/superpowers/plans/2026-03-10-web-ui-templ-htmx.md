# Web UI Rebuild (Templ + HTMX) Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the React SPA Web UI with a server-rendered Templ + HTMX implementation forked from upstream PR #176, then add thread view and inline attachment rendering.

**Architecture:** Server-rendered HTML via Templ templates compiled to Go. HTMX (vendored, 14KB) handles partial page updates. All assets embedded via `go:embed`. Handlers call `query.Engine` (DuckDB/Parquet) directly — no JSON serialization layer. Single `go build` produces the complete binary.

**Tech Stack:** Go 1.25+, Templ (type-safe HTML templates), HTMX 2.x, chi/v5 router (already in go.mod), bluemonday (HTML sanitizer)

**Spec:** `docs/superpowers/specs/2026-03-10-web-ui-templ-htmx-design.md`

**Adaptation notes:** This plan uses `Handler` (exported) as the handler struct name and `chi/v5` as the router. PR #176 may use different naming (`handlers`, `handler`) or routing patterns. After cherry-picking in Task 2, adapt struct names, method signatures, and route registration to match #176's conventions throughout all subsequent tasks. Template code is pseudo-Templ showing data flow — adapt to #176's exact Templ patterns.

---

## Chunk 1: Rip Out React & Adopt PR #176

This chunk removes the React SPA, the current `internal/web/` and `internal/api/` packages, and brings in PR #176's Templ + HTMX implementation.

### Task 1: Remove React Frontend

**Files:**
- Delete: `web/` (entire directory — React SPA, node_modules, Vite config)
- Delete: `internal/web/` (entire directory — Go handlers serving React)
- Delete: `internal/api/` (entire directory — separate API server)
- Delete: `cmd/msgvault/cmd/web.go` (React web command)
- Delete: `cmd/msgvault/cmd/serve.go` (API daemon command)
- Delete: `cmd/msgvault/cmd/serve_test.go` (if exists)
- Modify: `Makefile` (remove web-install, web-dev, web-build targets)

- [ ] **Step 1: Delete the React frontend and Go web/API packages**

```bash
rm -rf web/
rm -rf internal/web/
rm -rf internal/api/
rm -f cmd/msgvault/cmd/web.go
rm -f cmd/msgvault/cmd/serve.go
rm -f cmd/msgvault/cmd/serve_test.go
```

- [ ] **Step 2: Update Makefile — remove web targets**

In `/Users/ryanstern/msgvault/Makefile`, remove the `web-install`, `web-dev`, `web-build` targets and update the `build` target to no longer depend on `web-build`. Also remove `internal/web/dist` from the `clean` target.

The `build` target should become:

```makefile
build:
	go build -tags fts5 -o msgvault ./cmd/msgvault
```

The `clean` target should remove only the binary:

```makefile
clean:
	rm -f msgvault
```

- [ ] **Step 3: Clean up stale dependencies**

```bash
go mod tidy
```

- [ ] **Step 4: Verify the project still compiles without web**

```bash
go build -tags fts5 ./cmd/msgvault
```

Expected: Compiles successfully. Any import errors from removed packages indicate other files that reference `internal/web` or `internal/api` — fix those too.

- [ ] **Step 5: Run existing tests**

```bash
go test -tags fts5 ./...
```

Expected: All tests pass. Some tests in `cmd/msgvault/cmd/` may fail if they import `web.go` or `serve.go` — those files are deleted so the test files should be deleted too if they exist.

- [ ] **Step 6: Commit the removal**

```bash
git add -A
git commit -m "refactor: remove React SPA and API server

Delete web/ (React frontend), internal/web/ (SPA handlers),
internal/api/ (API daemon), and associated CLI commands.
Preparing to adopt Templ + HTMX approach from upstream PR #176."
```

### Task 2: Fetch and Cherry-Pick PR #176

**Files:**
- Add: `internal/web/` (Templ handlers, templates, static assets from PR #176)

- [ ] **Step 1: Add sarcasticbird's remote and fetch the PR branch**

First, check the PR to find sarcasticbird's fork URL:

```bash
gh pr view 176 --repo wesm/msgvault --json headRepositoryOwner,headRefName
```

Then add the remote and fetch:

```bash
git remote add sarcasticbird https://github.com/<owner>/msgvault.git
git fetch sarcasticbird feature-templ-ui
```

- [ ] **Step 2: Inspect what the PR changes**

```bash
git diff main...sarcasticbird/feature-templ-ui --stat
```

Review the file list. The key directory is `internal/web/` — that's what we want. If the PR also modifies shared files (`go.mod`, `cmd/` files, `internal/query/`, etc.), note those for conflict resolution.

- [ ] **Step 3: Cherry-pick the PR's internal/web/ directory**

Use the conflict fallback approach (from spec Planning Notes) — selectively bring in only `internal/web/`:

```bash
git checkout sarcasticbird/feature-templ-ui -- internal/web/
```

If the PR also adds a CLI command file (e.g., `cmd/msgvault/cmd/web.go`), grab that too:

```bash
git checkout sarcasticbird/feature-templ-ui -- cmd/msgvault/cmd/web.go
```

- [ ] **Step 4: Install templ CLI**

```bash
go install github.com/a-h/templ/cmd/templ@v0.3.865
```

Pin to a specific version for reproducibility. Check what version PR #176 uses and match it.

- [ ] **Step 5: Add templ dependency to go.mod**

```bash
go get github.com/a-h/templ@v0.3.865
```

- [ ] **Step 6: Resolve any go.mod conflicts**

```bash
go mod tidy
```

Expected: Clean resolution. If there are version conflicts between your fork's dependencies and what #176 needs, resolve by taking the newer version.

- [ ] **Step 7: Verify the Templ code compiles**

```bash
go build -tags fts5 ./cmd/msgvault
```

Expected: Compiles. If there are import path mismatches (e.g., #176 references packages your fork has moved), fix the import paths in the cherry-picked files.

- [ ] **Step 8: Run the web UI to verify it works**

```bash
./msgvault web
```

Expected: Server starts, opens browser to the dashboard. Verify pages load: dashboard, browse, search, message detail, deletions.

- [ ] **Step 9: Run all tests**

```bash
go test -tags fts5 ./...
```

Expected: All tests pass.

- [ ] **Step 10: Commit the adoption**

```bash
git add -A
git commit -m "feat: adopt Templ + HTMX web UI from upstream PR #176

Cherry-pick server-rendered web UI from sarcasticbird/feature-templ-ui.
Replaces React SPA with Go templates (Templ) + HTMX partial updates.
Single go build produces complete binary — no npm/Node.js required.

Pages: dashboard, browse, messages, message detail, search, deletions.
Includes: Vim-style keyboard nav, Solarized light/dark themes,
keyboard-driven deletion staging."
```

### Task 3: Update Makefile for Templ Workflow

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add templ generate target**

Add to `Makefile`:

```makefile
# Templ code generation (only needed when editing .templ files)
.PHONY: templ
templ:
	templ generate ./internal/web/templates/

# Build with templ generation
build: templ
	go build -tags fts5 -o msgvault ./cmd/msgvault

# Dev mode: watch templ files and rebuild
web-dev:
	templ generate --watch --proxy="http://localhost:8080" ./internal/web/templates/
```

- [ ] **Step 2: Verify make build works end-to-end**

```bash
make build
```

Expected: `templ generate` runs first (regenerates `_templ.go` files), then `go build` succeeds.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add templ generate targets to Makefile"
```

---

## Chunk 2: Add Thread View

This chunk adds the thread/conversation view — a new page showing all messages in a conversation chronologically, with full message bodies and inline attachments.

**Dependency note:** The thread template references `@SanitizedHTML()` which is implemented in Chunk 3 (Task 7). During Chunk 2 development, use a simple `{ msg.BodyText }` placeholder for message bodies. Chunk 3 adds the HTML sanitizer and switches to sanitized HTML rendering.

### Task 4: Write Thread Handler Tests

**Files:**
- Create: `internal/web/handlers_thread_test.go`

The thread handler needs to:
1. Accept a message ID from the URL
2. Look up the message to get its `ConversationID`
3. Load all messages in that conversation via `query.Engine.ListMessages` with `ConversationID` filter
4. Render the thread template

- [ ] **Step 1: Write the test file**

Create `internal/web/handlers_thread_test.go`:

```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/query"
)

// mockThreadEngine implements the subset of query.Engine needed for thread tests.
type mockThreadEngine struct {
	query.Engine
	getMessage   func(ctx context.Context, id int64) (*query.MessageDetail, error)
	listMessages func(ctx context.Context, filter query.MessageFilter) ([]query.MessageSummary, error)
}

func (m *mockThreadEngine) GetMessage(ctx context.Context, id int64) (*query.MessageDetail, error) {
	return m.getMessage(ctx, id)
}

func (m *mockThreadEngine) ListMessages(ctx context.Context, filter query.MessageFilter) ([]query.MessageSummary, error) {
	return m.listMessages(ctx, filter)
}

func TestThreadHandler_Success(t *testing.T) {
	convID := int64(42)
	now := time.Now()

	eng := &mockThreadEngine{
		getMessage: func(ctx context.Context, id int64) (*query.MessageDetail, error) {
			return &query.MessageDetail{
				ID:             1,
				ConversationID: convID,
				Subject:        "Test Thread",
				SentAt:         now,
				From:           []query.Address{{Email: "alice@example.com", Name: "Alice"}},
			}, nil
		},
		listMessages: func(ctx context.Context, filter query.MessageFilter) ([]query.MessageSummary, error) {
			if filter.ConversationID == nil || *filter.ConversationID != convID {
				t.Fatalf("expected ConversationID filter %d, got %v", convID, filter.ConversationID)
			}
			return []query.MessageSummary{
				{ID: 1, ConversationID: convID, Subject: "Test Thread", FromEmail: "alice@example.com", SentAt: now},
				{ID: 2, ConversationID: convID, Subject: "Re: Test Thread", FromEmail: "bob@example.com", SentAt: now.Add(time.Hour)},
			}, nil
		},
	}

	h := &Handler{engine: eng}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")

	req := httptest.NewRequest("GET", "/messages/1/thread", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.threadView(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Test Thread") {
		t.Error("response should contain thread subject")
	}
	if !strings.Contains(body, "alice@example.com") {
		t.Error("response should contain sender email")
	}
}

func TestThreadHandler_MessageNotFound(t *testing.T) {
	eng := &mockThreadEngine{
		getMessage: func(ctx context.Context, id int64) (*query.MessageDetail, error) {
			return nil, nil // not found
		},
	}

	h := &Handler{engine: eng}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")

	req := httptest.NewRequest("GET", "/messages/999/thread", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.threadView(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestThreadHandler_InvalidID(t *testing.T) {
	h := &Handler{}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-number")

	req := httptest.NewRequest("GET", "/messages/not-a-number/thread", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.threadView(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -tags fts5 ./internal/web/ -run TestThread -v
```

Expected: FAIL — `h.threadView` undefined.

- [ ] **Step 3: Commit the failing tests**

```bash
git add internal/web/handlers_thread_test.go
git commit -m "test: add thread view handler tests"
```

### Task 5: Implement Thread Handler

**Files:**
- Create: `internal/web/handlers_thread.go`
- Modify: `internal/web/server.go` (add route)

- [ ] **Step 1: Create the thread handler**

Create `internal/web/handlers_thread.go`:

```go
package web

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/query"
)

// threadView renders all messages in a conversation thread.
func (h *Handler) threadView(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up the message to get its conversation ID.
	msg, err := h.engine.GetMessage(ctx, id)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if msg == nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	// Load all messages in the conversation.
	convID := msg.ConversationID
	threadMessages, err := h.engine.ListMessages(ctx, query.MessageFilter{
		ConversationID: &convID,
		Pagination:     query.Pagination{Limit: 200},
	})
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Load full detail for each message (bodies + attachments).
	var details []*query.MessageDetail
	for _, summary := range threadMessages {
		detail, err := h.engine.GetMessage(ctx, summary.ID)
		if err != nil {
			continue // skip messages that fail to load
		}
		if detail != nil {
			details = append(details, detail)
		}
	}

	// Render the thread template.
	// NOTE: The exact Templ render call depends on PR #176's template patterns.
	// This will be adjusted after cherry-picking to match their rendering approach.
	err = threadTemplate(msg.Subject, details).Render(ctx, w)
	if err != nil {
		http.Error(w, "Template render error", http.StatusInternalServerError)
	}
}
```

**Note:** The `Pagination` field on `MessageFilter` and the exact Templ render call (`threadTemplate(...)`) must be adapted to match PR #176's patterns after cherry-picking. The handler structure and data flow are correct — only the template invocation syntax needs adjustment.

- [ ] **Step 2: Register the route in server.go**

In `internal/web/server.go`, find the route registration section (chi router setup) and add:

```go
r.Get("/messages/{id}/thread", h.threadView)
```

Place it after the existing `/messages/{id}` route.

- [ ] **Step 3: Run the tests**

```bash
go test -tags fts5 ./internal/web/ -run TestThread -v
```

Expected: All 3 tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/web/handlers_thread.go internal/web/server.go
git commit -m "feat: add thread view handler

Load all messages in a conversation by ConversationID,
fetch full details for each, render via Templ template.
Routes: GET /messages/{id}/thread"
```

### Task 6: Create Thread Template

**Files:**
- Create: `internal/web/templates/thread.templ`
- Modify: `internal/web/templates/message_detail.templ` (add "View thread" link)

- [ ] **Step 1: Create the thread template**

Create `internal/web/templates/thread.templ`. This must follow PR #176's template patterns (layout wrapping, component style, CSS classes). The structure below captures the intent — adapt the exact Templ syntax to match #176's conventions after cherry-picking:

```
// thread.templ — adapt to match PR #176's templ patterns
package templates

import (
    "fmt"
    "github.com/wesm/msgvault/internal/query"
)

templ ThreadPage(subject string, messages []*query.MessageDetail) {
    @Layout("Thread: " + subject) {
        <div class="thread-header">
            <h1>{ subject }</h1>
            <span class="message-count">{ fmt.Sprintf("%d messages", len(messages)) }</span>
        </div>
        <div class="thread-messages">
            for _, msg := range messages {
                @ThreadMessage(msg)
            }
        </div>
    }
}

templ ThreadMessage(msg *query.MessageDetail) {
    <div class="thread-message" id={ fmt.Sprintf("msg-%d", msg.ID) }>
        <div class="message-header">
            <span class="from">
                if len(msg.From) > 0 {
                    { msg.From[0].Name }
                    <span class="email">&lt;{ msg.From[0].Email }&gt;</span>
                }
            </span>
            <span class="date">{ msg.SentAt.Format("Jan 2, 2006 3:04 PM") }</span>
        </div>
        <div class="message-body">
            if msg.BodyHTML != "" {
                @SanitizedHTML(msg.BodyHTML)
            } else {
                <pre class="plain-text">{ msg.BodyText }</pre>
            }
        </div>
        if len(msg.Attachments) > 0 {
            <div class="attachments">
                for _, att := range msg.Attachments {
                    @AttachmentLink(att)
                }
            </div>
        }
    </div>
}

templ AttachmentLink(att query.AttachmentInfo) {
    if isInlineImage(att.MimeType) {
        <img src={ fmt.Sprintf("/attachments/%d/inline", att.ID) }
             alt={ att.Filename }
             class="inline-attachment" loading="lazy" />
    } else {
        <a href={ templ.SafeURL(fmt.Sprintf("/attachments/%d/download?hash=%s", att.ID, att.ContentHash)) }
           class="attachment-link">
            { att.Filename } ({ formatBytes(att.Size) })
        </a>
    }
}
```

**Important:** This is pseudo-Templ showing the data flow and structure. After cherry-picking PR #176, adapt to their exact patterns for `Layout`, component naming, CSS class conventions, and helper function locations.

- [ ] **Step 2: Add helper function for inline image detection**

In `internal/web/templates/helpers.go`, add:

```go
package templates

import (
	"fmt"
	"strings"
)

// isInlineImage returns true if the MIME type is a displayable image.
func isInlineImage(mimeType string) bool {
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/svg+xml":
		return true
	}
	return false
}

// formatBytes formats byte count as human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
```

**Note:** PR #176 likely already has `formatBytes` — check and reuse theirs instead of duplicating.

- [ ] **Step 3: Generate Templ code**

```bash
templ generate ./internal/web/templates/
```

Expected: Generates `thread_templ.go` in the same directory.

- [ ] **Step 4: Add "View thread" link to message detail template**

In `internal/web/templates/message_detail.templ`, find the message header area and add a thread link:

```
if msg.ConversationID > 0 {
    <a href={ templ.SafeURL(fmt.Sprintf("/messages/%d/thread", msg.ID)) }
       class="thread-link">
        View thread
    </a>
}
```

- [ ] **Step 5: Regenerate and build**

```bash
templ generate ./internal/web/templates/
go build -tags fts5 ./cmd/msgvault
```

Expected: Compiles successfully.

- [ ] **Step 6: Run all tests**

```bash
go test -tags fts5 ./...
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/web/templates/ internal/web/handlers_thread.go
git commit -m "feat: add thread view template with inline attachments

Templ template showing all messages in a conversation chronologically.
Inline images rendered directly, other attachments as download links.
Adds 'View thread' link to message detail page."
```

---

## Chunk 3: HTML Sanitization & Inline Attachment Handler

This chunk adds HTML sanitization for message bodies (security requirement) and ensures the inline attachment endpoint works with CSP headers.

### Task 7: Add bluemonday HTML Sanitizer

**Files:**
- Modify: `go.mod` (add bluemonday dependency)
- Create: `internal/web/sanitize.go`
- Create: `internal/web/sanitize_test.go`

- [ ] **Step 1: Add bluemonday dependency**

```bash
go get github.com/microcosm-cc/bluemonday@latest
```

- [ ] **Step 2: Write the sanitizer test**

Create `internal/web/sanitize_test.go`:

```go
package web

import (
	"strings"
	"testing"
)

func TestSanitizeHTML_RemovesScript(t *testing.T) {
	input := `<p>Hello</p><script>alert('xss')</script>`
	result := sanitizeHTML(input)
	if strings.Contains(result, "<script>") {
		t.Error("script tag should be removed")
	}
	if !strings.Contains(result, "<p>Hello</p>") {
		t.Error("safe content should be preserved")
	}
}

func TestSanitizeHTML_AllowsBasicFormatting(t *testing.T) {
	input := `<p><strong>Bold</strong> and <em>italic</em></p>`
	result := sanitizeHTML(input)
	if !strings.Contains(result, "<strong>Bold</strong>") {
		t.Error("strong tags should be preserved")
	}
	if !strings.Contains(result, "<em>italic</em>") {
		t.Error("em tags should be preserved")
	}
}

func TestSanitizeHTML_AllowsImages(t *testing.T) {
	input := `<img src="/attachments/1/inline" alt="photo">`
	result := sanitizeHTML(input)
	if !strings.Contains(result, "<img") {
		t.Error("img tags should be preserved for inline attachments")
	}
}

func TestSanitizeHTML_RemovesOnHandlers(t *testing.T) {
	input := `<div onclick="alert('xss')">Click</div>`
	result := sanitizeHTML(input)
	if strings.Contains(result, "onclick") {
		t.Error("event handlers should be removed")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test -tags fts5 ./internal/web/ -run TestSanitize -v
```

Expected: FAIL — `sanitizeHTML` undefined.

- [ ] **Step 4: Implement the sanitizer**

Create `internal/web/sanitize.go`:

```go
package web

import "github.com/microcosm-cc/bluemonday"

// policy is a reusable sanitization policy for email HTML.
var policy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Allow images for inline attachments (only local paths).
	p.AllowImages()
	// Strip JavaScript event handlers, iframes, forms, etc.
	return p
}()

// sanitizeHTML sanitizes email HTML body content for safe rendering.
func sanitizeHTML(html string) string {
	return policy.Sanitize(html)
}
```

- [ ] **Step 5: Run tests**

```bash
go test -tags fts5 ./internal/web/ -run TestSanitize -v
```

Expected: All 4 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/web/sanitize.go internal/web/sanitize_test.go go.mod go.sum
git commit -m "feat: add bluemonday HTML sanitizer for email bodies

Sanitize email HTML before rendering in templates to prevent XSS.
Allows basic formatting, images (for inline attachments), strips
scripts, event handlers, and dangerous elements."
```

### Task 8: Inline Attachment Endpoint with CSP

**Files:**
- Create or modify: `internal/web/handlers_attachments.go` (may already exist from PR #176)
- Create: `internal/web/handlers_attachments_test.go`

**Note:** PR #176 likely includes attachment download handling already. This task ensures the inline rendering path exists with proper CSP headers. Adapt based on what #176 provides.

- [ ] **Step 1: Write inline attachment test**

Create `internal/web/handlers_attachments_test.go`:

```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/query"
)

type mockAttachmentEngine struct {
	query.Engine
	getAttachment func(ctx context.Context, id int64) (*query.AttachmentInfo, error)
}

func (m *mockAttachmentEngine) GetAttachment(ctx context.Context, id int64) (*query.AttachmentInfo, error) {
	return m.getAttachment(ctx, id)
}

func TestInlineAttachment_CSPHeaders(t *testing.T) {
	eng := &mockAttachmentEngine{
		getAttachment: func(ctx context.Context, id int64) (*query.AttachmentInfo, error) {
			return &query.AttachmentInfo{
				ID:          1,
				Filename:    "photo.jpg",
				MimeType:    "image/jpeg",
				Size:        1024,
				ContentHash: "abc123",
			}, nil
		},
	}

	h := &Handler{engine: eng, attachmentsDir: t.TempDir()}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")

	req := httptest.NewRequest("GET", "/attachments/1/inline", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.inlineAttachment(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("CSP header should be set for inline content")
	}

	xContent := rec.Header().Get("X-Content-Type-Options")
	if xContent != "nosniff" {
		t.Error("X-Content-Type-Options should be nosniff")
	}
}
```

- [ ] **Step 2: Ensure the inline handler sets CSP headers**

If `internal/web/handlers_attachments.go` exists from PR #176, add CSP headers to the inline serving path. If not, create it:

```go
package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) inlineAttachment(w http.ResponseWriter, r *http.Request) {
	// Set security headers for inline content.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	h.serveAttachment(w, r, true)
}

func (h *Handler) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	h.serveAttachment(w, r, false)
}

func (h *Handler) serveAttachment(w http.ResponseWriter, r *http.Request, inline bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	att, err := h.engine.GetAttachment(r.Context(), id)
	if err != nil || att == nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	// Validate content hash if provided in query param.
	if hash := r.URL.Query().Get("hash"); hash != "" && hash != att.ContentHash {
		http.Error(w, "Content hash mismatch", http.StatusConflict)
		return
	}

	// Construct content-addressed file path.
	filePath := filepath.Join(h.attachmentsDir, att.ContentHash[:2], att.ContentHash)
	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Attachment file not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	if inline {
		w.Header().Set("Content-Type", att.MimeType)
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, att.Filename))
	}

	http.ServeContent(w, r, att.Filename, time.Time{}, f)
}
```

**Note:** PR #176 already has attachment handling with SHA-256 validation. Adapt this to add CSP headers to their existing implementation rather than writing from scratch.

- [ ] **Step 3: Register routes in server.go**

```go
r.Get("/attachments/{id}/inline", h.inlineAttachment)
r.Get("/attachments/{id}/download", h.downloadAttachment)
```

- [ ] **Step 4: Run tests**

```bash
go test -tags fts5 ./internal/web/ -run TestInline -v
```

Expected: Pass.

- [ ] **Step 5: Commit**

```bash
git add internal/web/handlers_attachments.go internal/web/handlers_attachments_test.go internal/web/server.go
git commit -m "feat: add inline attachment handler with CSP headers

Serve inline attachments (images) with Content-Security-Policy
headers to sandbox content. Download handler for non-image files.
SHA-256 hash validation on both paths."
```

---

## Chunk 4: Thread Keyboard Navigation & CSS

This chunk adds the `t` keyboard shortcut for thread navigation and thread-specific CSS to the Solarized theme.

### Task 9: Add Thread Keyboard Shortcut

**Files:**
- Modify: `internal/web/static/keys.js`

- [ ] **Step 1: Add `t` shortcut to keys.js**

Find the keyboard handler section in `internal/web/static/keys.js` and add:

```javascript
// 't' — view thread for current message (on message detail page)
case 't': {
    const threadLink = document.querySelector('.thread-link');
    if (threadLink) {
        window.location.href = threadLink.href;
    }
    break;
}
```

- [ ] **Step 2: Add thread navigation shortcuts and helper function**

If #176 already uses `n`/`p` for next/prev message, add thread-aware behavior. Within the existing `switch` block in #176's keyboard handler, add thread-page guards inside the existing cases:

```javascript
case 'n': {
    if (document.querySelector('.thread-messages')) {
        scrollToNextThreadMessage(1);
    } else {
        // existing next-message behavior from #176
    }
    break;
}
case 'p': {
    if (document.querySelector('.thread-messages')) {
        scrollToNextThreadMessage(-1);
    } else {
        // existing prev-message behavior from #176
    }
    break;
}
```

Also add the scroll helper function at the bottom of `keys.js`:

```javascript
// Thread message scroll navigation
let currentThreadIdx = -1;
function scrollToNextThreadMessage(direction) {
    const messages = document.querySelectorAll('.thread-message');
    if (!messages.length) return;
    currentThreadIdx = Math.max(0, Math.min(messages.length - 1, currentThreadIdx + direction));
    messages[currentThreadIdx].scrollIntoView({ behavior: 'smooth', block: 'start' });
}
```

**Note:** Adapt to #176's existing keyboard handling patterns — add to their switch statement, don't create a parallel handler.

- [ ] **Step 3: Verify build**

```bash
go build -tags fts5 ./cmd/msgvault
```

Expected: Compiles (JS is embedded via go:embed, no syntax checking at build time — verify manually in browser).

- [ ] **Step 4: Commit**

```bash
git add internal/web/static/keys.js
git commit -m "feat: add keyboard shortcuts for thread navigation

't' opens thread view from message detail.
'n'/'p' scroll between messages within a thread."
```

### Task 10: Add Thread CSS

**Files:**
- Modify: `internal/web/static/style.css`

- [ ] **Step 1: Add thread-specific styles**

Append to `internal/web/static/style.css`:

```css
/* Thread view */
.thread-header {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    margin-bottom: 1.5rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid var(--border);
}

.thread-header .message-count {
    color: var(--secondary);
    font-size: 0.875rem;
}

.thread-message {
    margin-bottom: 1.5rem;
    padding: 1rem;
    border: 1px solid var(--border);
    border-radius: 4px;
}

.thread-message .message-header {
    display: flex;
    justify-content: space-between;
    margin-bottom: 0.75rem;
    font-size: 0.875rem;
}

.thread-message .from .email {
    color: var(--secondary);
    margin-left: 0.25rem;
}

.thread-message .message-body {
    line-height: 1.6;
}

.thread-message .message-body .plain-text {
    white-space: pre-wrap;
    font-family: inherit;
}

.thread-message .attachments {
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--border);
}

.inline-attachment {
    max-width: 100%;
    height: auto;
    margin: 0.5rem 0;
    border-radius: 4px;
}

.attachment-link {
    display: inline-block;
    padding: 0.25rem 0.5rem;
    background: var(--bg-secondary);
    border-radius: 4px;
    font-size: 0.875rem;
    margin-right: 0.5rem;
}

.thread-link {
    font-size: 0.875rem;
    color: var(--link);
}
```

**Note:** CSS variable names (`--border`, `--secondary`, `--bg-secondary`, `--link`) must match #176's Solarized theme variables. Check `style.css` after cherry-picking and use the exact variable names.

- [ ] **Step 2: Commit**

```bash
git add internal/web/static/style.css
git commit -m "style: add thread view and inline attachment CSS

Solarized-themed styles for thread messages, inline images,
attachment links, and thread navigation."
```

---

## Chunk 5: Integration Testing & Help Overlay Update

### Task 11: Integration Smoke Test

**Files:**
- Create: `internal/web/integration_test.go`

- [ ] **Step 1: Write a manual smoke test checklist and verify**

This task replaces a traditional integration test because the Handler and router construction depend entirely on PR #176's patterns. Instead, verify all routes work manually:

```bash
# Build and run
make build
./msgvault web &
WEB_PID=$!

# Test each route (adjust port to match #176's default)
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/          # Dashboard: expect 200
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/browse     # Browse: expect 200
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/search     # Search: expect 200
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/deletions  # Deletions: expect 200
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/messages/1/thread  # Thread: expect 200 or 404

kill $WEB_PID
```

Once you understand #176's Handler constructor pattern, write a proper integration test using their mock/test setup patterns. Do not commit a skeleton with TODO comments.

- [ ] **Step 2: Verify full build and all tests**

```bash
make build
go test -tags fts5 ./...
```

Expected: All tests pass, binary builds clean. No commit needed — this task is manual verification only.

### Task 12: Update Help Overlay

**Files:**
- Modify: `internal/web/templates/layout.templ` (or wherever #176 puts the help overlay)

- [ ] **Step 1: Add thread shortcut to help overlay**

Find the keyboard shortcut help overlay in the layout template and add:

```
t     — View thread (from message detail)
```

to the list of shortcuts.

- [ ] **Step 2: Regenerate and commit**

```bash
templ generate ./internal/web/templates/
git add internal/web/templates/
git commit -m "docs: add thread shortcut to help overlay"
```

---

## Post-Implementation Checklist

After all tasks are complete:

- [ ] `make build` succeeds with no warnings
- [ ] `go test -tags fts5 ./...` all pass
- [ ] `go vet ./...` clean
- [ ] `go fmt ./...` no changes
- [ ] Manual test: `./msgvault web` — verify dashboard, browse, search, message detail, thread view, deletions all work
- [ ] Manual test: Navigate to `/messages/{id}/thread` directly — verify page loads
- [ ] Manual test: Click "View thread" on a message with multiple replies — verify chronological order
- [ ] Manual test: Inline images render in thread view
- [ ] Manual test: `t` keyboard shortcut opens thread from message detail
- [ ] Manual test: `n`/`p` scroll between thread messages
- [ ] No `web/` directory exists (React fully removed)
- [ ] No `node_modules/` anywhere in the project
- [ ] `go build` alone produces a working binary (no npm step)
