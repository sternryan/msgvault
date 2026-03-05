package web

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/search"
)

const maxLimit = 1000

type handlers struct {
	engine         query.Engine
	attachmentsDir string
	deletions      *deletion.Manager
	logger         *slog.Logger
}

// --- Stats & Accounts ---

func (h *handlers) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.engine.GetTotalStats(r.Context(), query.StatsOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("stats failed: %v", err))
		return
	}

	accounts, err := h.engine.ListAccounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("accounts failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{
		Data: map[string]any{
			"stats":    stats,
			"accounts": accounts,
		},
	})
}

func (h *handlers) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.engine.ListAccounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("accounts failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Data: accounts})
}

// --- Aggregation ---

func (h *handlers) aggregate(w http.ResponseWriter, r *http.Request) {
	groupBy := r.URL.Query().Get("groupBy")
	if groupBy == "" {
		writeError(w, http.StatusBadRequest, "groupBy parameter is required")
		return
	}

	viewType, ok := parseViewType(groupBy)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid groupBy: %s", groupBy))
		return
	}

	opts := h.parseAggregateOptions(r)

	rows, err := h.engine.Aggregate(r.Context(), viewType, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("aggregate failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: rows})
}

func (h *handlers) subAggregate(w http.ResponseWriter, r *http.Request) {
	groupBy := r.URL.Query().Get("groupBy")
	if groupBy == "" {
		writeError(w, http.StatusBadRequest, "groupBy parameter is required")
		return
	}

	viewType, ok := parseViewType(groupBy)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid groupBy: %s", groupBy))
		return
	}

	filter := h.parseMessageFilter(r)
	opts := h.parseAggregateOptions(r)

	rows, err := h.engine.SubAggregate(r.Context(), filter, viewType, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("sub-aggregate failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: rows})
}

// --- Messages ---

func (h *handlers) listMessages(w http.ResponseWriter, r *http.Request) {
	filter := h.parseMessageFilter(r)

	messages, err := h.engine.ListMessages(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list messages failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: messages})
}

func (h *handlers) getMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}

	msg, err := h.engine.GetMessage(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("message not found: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: msg})
}

func (h *handlers) getThread(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}

	// Get the message to find its conversation ID
	msg, err := h.engine.GetMessage(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("message not found: %v", err))
		return
	}

	convID := msg.ConversationID
	filter := query.MessageFilter{
		ConversationID: &convID,
		Sorting: query.MessageSorting{
			Field:     query.MessageSortByDate,
			Direction: query.SortAsc,
		},
		Pagination: query.Pagination{Limit: 100},
	}

	messages, err := h.engine.ListMessages(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("thread query failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: messages})
}

// --- Search ---

func (h *handlers) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	mode := r.URL.Query().Get("mode")
	limit := intParam(r, "limit", 100)
	offset := intParam(r, "offset", 0)

	parsed := search.Parse(q)
	filter := h.parseMessageFilter(r)

	var results []query.MessageSummary
	var err error

	if mode == "deep" {
		results, err = h.engine.Search(r.Context(), parsed, limit, offset)
	} else {
		results, err = h.engine.SearchFast(r.Context(), parsed, filter, limit, offset)
		// Fall back to deep search if fast returns nothing and there are text terms
		if err == nil && len(results) == 0 && len(parsed.TextTerms) > 0 {
			results, err = h.engine.Search(r.Context(), parsed, limit, offset)
		}
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("search failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: results})
}

func (h *handlers) searchCount(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	parsed := search.Parse(q)
	filter := h.parseMessageFilter(r)

	count, err := h.engine.SearchFastCount(r.Context(), parsed, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("search count failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: map[string]int64{"count": count}})
}

// --- Attachments ---

func (h *handlers) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	h.serveAttachment(w, r, true)
}

func (h *handlers) inlineAttachment(w http.ResponseWriter, r *http.Request) {
	h.serveAttachment(w, r, false)
}

func (h *handlers) serveAttachment(w http.ResponseWriter, r *http.Request, download bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid attachment id")
		return
	}

	att, err := h.engine.GetAttachment(r.Context(), id)
	if err != nil || att == nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	if h.attachmentsDir == "" {
		writeError(w, http.StatusInternalServerError, "attachments directory not configured")
		return
	}

	if att.ContentHash == "" || len(att.ContentHash) < 2 {
		writeError(w, http.StatusNotFound, "attachment has no stored content")
		return
	}
	if _, err := hex.DecodeString(att.ContentHash); err != nil {
		writeError(w, http.StatusNotFound, "attachment has invalid content hash")
		return
	}

	filePath := filepath.Join(h.attachmentsDir, att.ContentHash[:2], att.ContentHash)
	f, err := os.Open(filePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment file not available")
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

	io.Copy(w, f)
}

// --- Deletions ---

func (h *handlers) stageDeletion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sender        string  `json:"sender"`
		SenderName    string  `json:"senderName"`
		Recipient     string  `json:"recipient"`
		RecipientName string  `json:"recipientName"`
		Domain        string  `json:"domain"`
		Label         string  `json:"label"`
		TimePeriod    string  `json:"timePeriod"`
		SourceID      *int64  `json:"sourceId"`
		MessageIDs    []int64 `json:"messageIds"`
	}

	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	filter := query.MessageFilter{
		Sender:        req.Sender,
		SenderName:    req.SenderName,
		Recipient:     req.Recipient,
		RecipientName: req.RecipientName,
		Domain:        req.Domain,
		Label:         req.Label,
		SourceID:      req.SourceID,
	}
	if req.TimePeriod != "" {
		filter.TimeRange.Period = req.TimePeriod
	}

	gmailIDs, err := h.engine.GetGmailIDsByFilter(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to resolve messages: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{
		Data: map[string]any{
			"gmailIds":     gmailIDs,
			"messageCount": len(gmailIDs),
		},
	})
}

func (h *handlers) confirmDeletion(w http.ResponseWriter, r *http.Request) {
	if h.deletions == nil {
		writeError(w, http.StatusInternalServerError, "deletion manager not available")
		return
	}

	var req struct {
		Description string           `json:"description"`
		GmailIDs    []string         `json:"gmailIds"`
		Filters     deletion.Filters `json:"filters"`
	}

	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if len(req.GmailIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no gmail IDs provided")
		return
	}

	manifest, err := h.deletions.CreateManifest(req.Description, req.GmailIDs, req.Filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create manifest: %v", err))
		return
	}
	manifest.CreatedBy = "web"

	if err := h.deletions.SaveManifest(manifest); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save manifest: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, apiResponse{Data: manifest})
}

func (h *handlers) listDeletions(w http.ResponseWriter, r *http.Request) {
	if h.deletions == nil {
		writeJSON(w, http.StatusOK, apiResponse{Data: []any{}})
		return
	}

	var all []*deletion.Manifest

	for _, listFn := range []func() ([]*deletion.Manifest, error){
		h.deletions.ListPending,
		h.deletions.ListInProgress,
		h.deletions.ListCompleted,
		h.deletions.ListFailed,
	} {
		manifests, err := listFn()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("list deletions failed: %v", err))
			return
		}
		all = append(all, manifests...)
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: all})
}

func (h *handlers) cancelDeletion(w http.ResponseWriter, r *http.Request) {
	if h.deletions == nil {
		writeError(w, http.StatusInternalServerError, "deletion manager not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "deletion id is required")
		return
	}

	if err := h.deletions.CancelManifest(id); err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("cancel failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{Data: map[string]string{"status": "cancelled"}})
}

// --- Parameter parsing helpers ---

func parseViewType(s string) (query.ViewType, bool) {
	m := map[string]query.ViewType{
		"senders":        query.ViewSenders,
		"senderNames":    query.ViewSenderNames,
		"recipients":     query.ViewRecipients,
		"recipientNames": query.ViewRecipientNames,
		"domains":        query.ViewDomains,
		"labels":         query.ViewLabels,
		"time":           query.ViewTime,
	}
	v, ok := m[s]
	return v, ok
}

func parseSortField(s string) query.SortField {
	switch s {
	case "count":
		return query.SortByCount
	case "size":
		return query.SortBySize
	case "attachmentSize":
		return query.SortByAttachmentSize
	case "name":
		return query.SortByName
	default:
		return query.SortByCount
	}
}

func parseSortDirection(s string) query.SortDirection {
	if s == "asc" {
		return query.SortAsc
	}
	return query.SortDesc
}

func parseTimeGranularity(s string) query.TimeGranularity {
	switch s {
	case "year":
		return query.TimeYear
	case "month":
		return query.TimeMonth
	case "day":
		return query.TimeDay
	default:
		return query.TimeMonth
	}
}

func parseMessageSortField(s string) query.MessageSortField {
	switch s {
	case "date":
		return query.MessageSortByDate
	case "size":
		return query.MessageSortBySize
	case "subject":
		return query.MessageSortBySubject
	default:
		return query.MessageSortByDate
	}
}

func (h *handlers) parseAggregateOptions(r *http.Request) query.AggregateOptions {
	opts := query.DefaultAggregateOptions()

	if v := r.URL.Query().Get("sortField"); v != "" {
		opts.SortField = parseSortField(v)
	}
	if v := r.URL.Query().Get("sortDir"); v != "" {
		opts.SortDirection = parseSortDirection(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		opts.Limit = clampInt(intParam(r, "limit", 100), 1, maxLimit)
	}
	if v := r.URL.Query().Get("timeGranularity"); v != "" {
		opts.TimeGranularity = parseTimeGranularity(v)
	}
	if v := r.URL.Query().Get("sourceId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			opts.SourceID = &id
		}
	}
	if r.URL.Query().Get("attachmentsOnly") == "true" {
		opts.WithAttachmentsOnly = true
	}
	if v := r.URL.Query().Get("search"); v != "" {
		opts.SearchQuery = v
	}
	if v := r.URL.Query().Get("after"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			opts.After = &t
		}
	}
	if v := r.URL.Query().Get("before"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			opts.Before = &t
		}
	}

	return opts
}

func (h *handlers) parseMessageFilter(r *http.Request) query.MessageFilter {
	filter := query.MessageFilter{
		Pagination: query.Pagination{
			Limit:  clampInt(intParam(r, "limit", 100), 1, maxLimit),
			Offset: intParam(r, "offset", 0),
		},
		Sorting: query.MessageSorting{
			Field:     parseMessageSortField(r.URL.Query().Get("sortField")),
			Direction: parseSortDirection(r.URL.Query().Get("sortDir")),
		},
	}

	if v := r.URL.Query().Get("sender"); v != "" {
		filter.Sender = v
	}
	if v := r.URL.Query().Get("senderName"); v != "" {
		filter.SenderName = v
	}
	if v := r.URL.Query().Get("recipient"); v != "" {
		filter.Recipient = v
	}
	if v := r.URL.Query().Get("recipientName"); v != "" {
		filter.RecipientName = v
	}
	if v := r.URL.Query().Get("domain"); v != "" {
		filter.Domain = v
	}
	if v := r.URL.Query().Get("label"); v != "" {
		filter.Label = v
	}
	if v := r.URL.Query().Get("sourceId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.SourceID = &id
		}
	}
	if v := r.URL.Query().Get("timePeriod"); v != "" {
		filter.TimeRange.Period = v
		if g := r.URL.Query().Get("timeGranularity"); g != "" {
			filter.TimeRange.Granularity = parseTimeGranularity(g)
		}
	}
	if r.URL.Query().Get("attachmentsOnly") == "true" {
		filter.WithAttachmentsOnly = true
	}
	if v := r.URL.Query().Get("after"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filter.After = &t
		}
	}
	if v := r.URL.Query().Get("before"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filter.Before = &t
		}
	}
	if v := r.URL.Query().Get("conversationId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.ConversationID = &id
		}
	}

	// Handle empty value targets for drill-down into empty buckets
	if evts := r.URL.Query()["emptyTarget"]; len(evts) > 0 {
		for _, evt := range evts {
			if vt, ok := parseViewType(evt); ok {
				filter.SetEmptyTarget(vt)
			}
		}
	}

	return filter
}

func intParam(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func readJSON(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("expected Content-Type: application/json")
	}
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(v)
}
