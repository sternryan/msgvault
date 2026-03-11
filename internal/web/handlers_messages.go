package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/web/templates"
)

func (h *handlers) messagesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse filter — override limit to 50 (locked: 50 rows/page)
	filter := parseMessageFilter(r)
	filter.Pagination.Limit = 50

	messages, err := h.engine.ListMessages(ctx, filter)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to load messages")
		return
	}

	// Get total count for pagination using GetTotalStats (fast, avoids a full scan)
	var statsOpts query.StatsOptions
	if filter.SourceID != nil {
		statsOpts.SourceID = filter.SourceID
	}
	stats, err := h.engine.GetTotalStats(ctx, statsOpts)
	if err != nil {
		h.logger.Error("failed to get total stats for messages pagination", "error", err)
	}

	var total int64
	if stats != nil {
		total = stats.MessageCount
	}

	// Build base URL preserving sort/filter params but NOT offset
	baseURL := buildMessagesBaseURL(r)

	content := templates.MessagesPage(messages, filter, total, baseURL)
	h.renderPage(w, r, "Messages", content)
}

func (h *handlers) messageDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid message ID")
		return
	}

	msg, err := h.engine.GetMessage(ctx, id)
	if err != nil {
		h.logger.Error("failed to get message", "id", id, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Failed to load message")
		return
	}
	if msg == nil {
		h.renderError(w, r, http.StatusNotFound, "Message not found")
		return
	}

	title := msg.Subject
	if title == "" {
		title = fmt.Sprintf("Message #%d", msg.ID)
	}

	content := templates.MessageDetailPage(msg)
	h.renderPage(w, r, title, content)
}

// buildMessagesBaseURL builds the base URL for message list pagination,
// preserving relevant query params (sortField, sortDir, sourceId) but NOT offset.
func buildMessagesBaseURL(r *http.Request) string {
	q := r.URL.Query()
	q.Del("offset")
	q.Del("limit")
	base := r.URL.Path
	if encoded := q.Encode(); encoded != "" {
		base = base + "?" + encoded
	}
	return base
}
