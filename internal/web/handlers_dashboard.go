package web

import (
	"net/http"
	"strconv"

	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/web/templates"
)

func (h *handlers) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse optional sourceId filter
	var statsOpts query.StatsOptions
	var aggSourceID *int64
	if v := r.URL.Query().Get("sourceId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			statsOpts.SourceID = &id
			aggSourceID = &id
		}
	}

	stats, err := h.engine.GetTotalStats(ctx, statsOpts)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to load stats")
		return
	}

	accounts, err := h.engine.ListAccounts(ctx)
	if err != nil {
		h.logger.Error("failed to list accounts for dashboard", "error", err)
		accounts = nil
	}

	// Top 5 senders
	senderOpts := query.AggregateOptions{
		SourceID:      aggSourceID,
		SortField:     query.SortByCount,
		SortDirection: query.SortDesc,
		Limit:         5,
	}
	topSenders, err := h.engine.Aggregate(ctx, query.ViewSenders, senderOpts)
	if err != nil {
		h.logger.Error("failed to load top senders for dashboard", "error", err)
		topSenders = nil
	}

	// Top 5 domains
	domainOpts := query.AggregateOptions{
		SourceID:      aggSourceID,
		SortField:     query.SortByCount,
		SortDirection: query.SortDesc,
		Limit:         5,
	}
	topDomains, err := h.engine.Aggregate(ctx, query.ViewDomains, domainOpts)
	if err != nil {
		h.logger.Error("failed to load top domains for dashboard", "error", err)
		topDomains = nil
	}

	content := templates.DashboardPage(stats, accounts, topSenders, topDomains)
	h.renderPage(w, r, "Dashboard", content)
}
