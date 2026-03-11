package web

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/web/templates"
)

// aggregate renders the top-level aggregate view for a given view type.
func (h *handlers) aggregate(w http.ResponseWriter, r *http.Request) {
	groupByStr := r.URL.Query().Get("groupBy")
	if groupByStr == "" {
		groupByStr = "senders"
	}

	viewType, ok := parseViewType(groupByStr)
	if !ok {
		h.renderError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid groupBy: %q", groupByStr))
		return
	}

	opts := parseAggregateOptions(r)
	searchQuery := r.URL.Query().Get("search")
	if searchQuery != "" {
		opts.SearchQuery = searchQuery
	}

	rows, err := h.engine.Aggregate(r.Context(), viewType, opts)
	if err != nil {
		h.logger.Error("aggregate query failed", "viewType", viewType, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "failed to load aggregate data")
		return
	}

	// Build base URL preserving groupBy and sourceId for sort/filter links.
	sourceID := r.URL.Query().Get("sourceId")
	baseURL := buildAggregateBaseURL(groupByStr, sourceID)

	sortField := r.URL.Query().Get("sortField")
	if sortField == "" {
		sortField = "count"
	}
	sortDir := r.URL.Query().Get("sortDir")
	if sortDir == "" {
		sortDir = "desc"
	}

	content := templates.AggregatePage(viewType, rows, sortField, sortDir, baseURL, searchQuery, sourceID)
	h.renderPage(w, r, viewType.String(), content)
}

// aggregateDrilldown renders a drill-down into a specific aggregate key,
// either showing messages or a sub-aggregate view.
func (h *handlers) aggregateDrilldown(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	groupByStr := q.Get("groupBy")
	if groupByStr == "" {
		groupByStr = "senders"
	}

	filterKey := q.Get("filterKey")
	filterViewStr := q.Get("filterView")
	sourceID := q.Get("sourceId")

	viewType, ok := parseViewType(groupByStr)
	if !ok {
		h.renderError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid groupBy: %q", groupByStr))
		return
	}

	// Build the message filter based on the drill-down key and sourceId.
	filter := applyKeyToFilter(query.MessageFilter{}, viewType, filterKey)
	if sourceID != "" {
		if id, err := parseInt64(sourceID); err == nil {
			filter.SourceID = &id
		}
	}

	// Build breadcrumbs.
	breadcrumbs := []templates.BreadcrumbItem{
		{Label: "Aggregate", URL: "/aggregate"},
		{Label: viewType.String(), URL: fmt.Sprintf("/aggregate?groupBy=%s", groupByStr)},
	}

	opts := parseAggregateOptions(r)
	sortField := q.Get("sortField")
	if sortField == "" {
		sortField = "count"
	}
	sortDir := q.Get("sortDir")
	if sortDir == "" {
		sortDir = "desc"
	}

	if filterViewStr != "" {
		// Sub-aggregate: show another aggregate table filtered to this key.
		filterViewType, ok := parseViewType(filterViewStr)
		if !ok {
			h.renderError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid filterView: %q", filterViewStr))
			return
		}

		subRows, err := h.engine.SubAggregate(r.Context(), filter, filterViewType, opts)
		if err != nil {
			h.logger.Error("sub-aggregate query failed", "filterViewType", filterViewType, "error", err)
			h.renderError(w, r, http.StatusInternalServerError, "failed to load sub-aggregate data")
			return
		}

		breadcrumbs = append(breadcrumbs, templates.BreadcrumbItem{
			Label: filterKey,
			URL:   drilldownURL(groupByStr, filterKey, "", sourceID),
		})
		breadcrumbs = append(breadcrumbs, templates.BreadcrumbItem{Label: filterViewType.String(), URL: ""})

		baseURL := drilldownURL(groupByStr, filterKey, filterViewStr, sourceID)
		content := templates.AggregateDrilldownPage(
			breadcrumbs, groupByStr, filterKey, filterViewStr,
			subRows, nil, sortField, sortDir, baseURL, sourceID, 0, 0,
		)
		h.renderPage(w, r, filterKey, content)
		return
	}

	// No filterView: show messages for this key.
	offset := intParam(r, "offset", 0)
	msgFilter := filter
	msgFilter.Pagination.Limit = 50
	msgFilter.Pagination.Offset = offset

	messages, err := h.engine.ListMessages(r.Context(), msgFilter)
	if err != nil {
		h.logger.Error("list messages for drilldown failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "failed to load messages")
		return
	}

	// Estimate total for pagination: if we got a full page, there may be more.
	var msgTotal int64
	if len(messages) == msgFilter.Pagination.Limit {
		msgTotal = int64(offset+msgFilter.Pagination.Limit) + 1
	} else {
		msgTotal = int64(offset + len(messages))
	}

	breadcrumbs = append(breadcrumbs, templates.BreadcrumbItem{Label: filterKey, URL: ""})

	baseURL := drilldownURL(groupByStr, filterKey, "", sourceID)
	content := templates.AggregateDrilldownPage(
		breadcrumbs, groupByStr, filterKey, "",
		nil, messages, sortField, sortDir, baseURL, sourceID, msgTotal, offset,
	)
	h.renderPage(w, r, filterKey, content)
}

// applyKeyToFilter sets the appropriate filter field based on viewType and filterKey.
func applyKeyToFilter(filter query.MessageFilter, viewType query.ViewType, filterKey string) query.MessageFilter {
	switch viewType {
	case query.ViewSenders:
		filter.Sender = filterKey
	case query.ViewSenderNames:
		filter.SenderName = filterKey
	case query.ViewRecipients:
		filter.Recipient = filterKey
	case query.ViewRecipientNames:
		filter.RecipientName = filterKey
	case query.ViewDomains:
		filter.Domain = filterKey
	case query.ViewLabels:
		filter.Label = filterKey
	case query.ViewTime:
		filter.TimeRange.Period = filterKey
	}
	return filter
}

// buildAggregateBaseURL constructs a base URL for the aggregate page sort headers.
func buildAggregateBaseURL(groupByStr, sourceID string) string {
	u := &url.URL{Path: "/aggregate"}
	q := url.Values{}
	q.Set("groupBy", groupByStr)
	if sourceID != "" {
		q.Set("sourceId", sourceID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// drilldownURL constructs a drill-down URL.
func drilldownURL(groupBy, filterKey, filterView, sourceID string) string {
	u := &url.URL{Path: "/aggregate/drilldown"}
	q := url.Values{}
	q.Set("groupBy", groupBy)
	q.Set("filterKey", filterKey)
	if filterView != "" {
		q.Set("filterView", filterView)
	}
	if sourceID != "" {
		q.Set("sourceId", sourceID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// parseInt64 parses a string as int64.
func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
