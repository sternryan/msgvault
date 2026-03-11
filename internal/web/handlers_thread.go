package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/web/templates"
)

// ThreadData holds all data needed to render the thread view.
type ThreadData struct {
	Messages     []query.MessageSummary
	HighlightID  int64
	Subject      string
	Participants []string // deduplicated sender names, in insertion order
	FirstDate    time.Time
	LastDate     time.Time
	MessageCount int
}

// threadView renders the thread (conversation) view for a given conversation ID.
func (h *handlers) threadView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	convIDStr := chi.URLParam(r, "conversationId")
	convID, err := strconv.ParseInt(convIDStr, 10, 64)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid conversation ID")
		return
	}

	filter := query.MessageFilter{
		ConversationID: &convID,
		Pagination:     query.Pagination{Limit: 500},
		Sorting: query.MessageSorting{
			Field:     query.MessageSortByDate,
			Direction: query.SortAsc,
		},
	}

	messages, err := h.engine.ListMessages(ctx, filter)
	if err != nil {
		h.logger.Error("failed to list messages for thread", "conversationId", convID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Failed to load thread")
		return
	}
	if len(messages) == 0 {
		h.renderError(w, r, http.StatusNotFound, "Thread not found")
		return
	}

	// Parse optional ?highlight={messageId} query param
	var highlightID int64
	if hStr := r.URL.Query().Get("highlight"); hStr != "" {
		if hID, err := strconv.ParseInt(hStr, 10, 64); err == nil {
			highlightID = hID
		}
	}

	// Build subject
	subject := messages[0].Subject
	if subject == "" {
		subject = "No subject"
	}

	// Deduplicate sender names preserving insertion order
	seen := make(map[string]bool)
	var participants []string
	for _, msg := range messages {
		name := msg.FromName
		if name == "" {
			name = msg.FromEmail
		}
		if !seen[name] {
			seen[name] = true
			participants = append(participants, name)
		}
	}

	threadData := ThreadData{
		Messages:     messages,
		HighlightID:  highlightID,
		Subject:      subject,
		Participants: participants,
		FirstDate:    messages[0].SentAt,
		LastDate:     messages[len(messages)-1].SentAt,
		MessageCount: len(messages),
	}

	content := templates.ThreadPage(
		threadData.Messages,
		threadData.HighlightID,
		threadData.Subject,
		threadData.Participants,
		threadData.FirstDate,
		threadData.LastDate,
		threadData.MessageCount,
	)
	h.renderPage(w, r, subject, content)
}
