package web

import (
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/web/templates"
)

type handlers struct {
	engine         query.Engine
	attachmentsDir string
	deletions      *deletion.Manager
	logger         *slog.Logger
}

// renderPage renders the full Layout template wrapping the given content component.
func (h *handlers) renderPage(w http.ResponseWriter, r *http.Request, title string, content templ.Component) {
	accounts, err := h.engine.ListAccounts(r.Context())
	if err != nil {
		h.logger.Error("failed to list accounts for layout", "error", err)
		accounts = nil
	}

	var activeAccountID *int64
	if v := r.URL.Query().Get("sourceId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			activeAccountID = &id
		}
	}

	pendingCount := h.pendingDeletionCount()

	page := templates.Layout(title, accounts, activeAccountID, pendingCount, r.URL.Path)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Render(templ.WithChildren(r.Context(), content), w); err != nil {
		h.logger.Error("failed to render page", "title", title, "error", err)
	}
}

// renderError renders an HTML error page with the given status code and message.
func (h *handlers) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	accounts, _ := h.engine.ListAccounts(r.Context())
	pendingCount := h.pendingDeletionCount()

	content := templates.ErrorContent(status, msg)
	page := templates.Layout("Error", accounts, nil, pendingCount, r.URL.Path)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := page.Render(templ.WithChildren(r.Context(), content), w); err != nil {
		h.logger.Error("failed to render error page", "error", err)
	}
}

// pendingDeletionCount returns the number of pending deletion manifests.
func (h *handlers) pendingDeletionCount() int {
	if h.deletions == nil {
		return 0
	}
	manifests, err := h.deletions.ListPending()
	if err != nil {
		return 0
	}
	return len(manifests)
}

// --- Attachment handlers (serve binary files, not HTML) ---

func (h *handlers) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	h.serveAttachment(w, r, true)
}

func (h *handlers) inlineAttachment(w http.ResponseWriter, r *http.Request) {
	h.serveAttachment(w, r, false)
}

func (h *handlers) serveAttachment(w http.ResponseWriter, r *http.Request, download bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid attachment id", http.StatusBadRequest)
		return
	}

	att, err := h.engine.GetAttachment(r.Context(), id)
	if err != nil || att == nil {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}

	if h.attachmentsDir == "" {
		http.Error(w, "attachments directory not configured", http.StatusInternalServerError)
		return
	}

	if att.ContentHash == "" || len(att.ContentHash) < 2 {
		http.Error(w, "attachment has no stored content", http.StatusNotFound)
		return
	}
	if _, err := hex.DecodeString(att.ContentHash); err != nil {
		http.Error(w, "attachment has invalid content hash", http.StatusNotFound)
		return
	}

	filePath := filepath.Join(h.attachmentsDir, att.ContentHash[:2], att.ContentHash)
	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "attachment file not available", http.StatusNotFound)
		return
	}
	defer f.Close()

	if att.MimeType != "" {
		w.Header().Set("Content-Type", att.MimeType)
	}

	if download {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
	} else {
		w.Header().Set("Content-Disposition", "inline")
	}

	io.Copy(w, f) //nolint:errcheck
}
