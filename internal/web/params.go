package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/wesm/msgvault/internal/query"
)

const maxLimit = 1000

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

func parseAggregateOptions(r *http.Request) query.AggregateOptions {
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

func parseMessageFilter(r *http.Request) query.MessageFilter {
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
