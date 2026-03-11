package web

import (
	"net/http"

	"github.com/wesm/msgvault/internal/search"
	"github.com/wesm/msgvault/internal/web/templates"
)

// searchPage handles GET /search. It renders a search page with a debounced
// live-search input. The DuckDB fast path (SearchFast) is tried first; if it
// returns no results and the query has text terms, it falls back to FTS5 (Search).
func (h *handlers) searchPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")

	if q == "" {
		// Empty query — show page with syntax help.
		content := templates.SearchPage("", nil, 0, "/search", "", 0)
		h.renderPage(w, r, "Search", content)
		return
	}

	parsed := search.Parse(q)
	filter := parseMessageFilter(r)
	// Clear pagination from the general filter — we manage limit/offset for search explicitly.
	offset := intParam(r, "offset", 0)
	const limit = 50

	results, err := h.engine.SearchFast(r.Context(), parsed, filter, limit, offset)
	if err != nil {
		h.logger.Error("search fast failed", "q", q, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "search failed")
		return
	}

	mode := "metadata"
	var total int64

	if len(results) == 0 && len(parsed.TextTerms) > 0 {
		// Fast path returned nothing and query has text terms — try FTS5 deep search.
		deepResults, err := h.engine.Search(r.Context(), parsed, limit, offset)
		if err != nil {
			h.logger.Error("search (deep) failed", "q", q, "error", err)
			// Fall through with empty results rather than failing the page.
		} else {
			results = deepResults
			mode = "full-text"
		}
		// For deep search, we don't have a separate count endpoint — use len as approximate.
		if len(results) == limit {
			total = int64(offset+limit) + 1 // more may exist
		} else {
			total = int64(offset + len(results))
		}
	} else {
		// Fast path result — get exact count for pagination.
		count, err := h.engine.SearchFastCount(r.Context(), parsed, filter)
		if err != nil {
			h.logger.Error("search fast count failed", "q", q, "error", err)
			// Fall back to estimate.
			if len(results) == limit {
				total = int64(offset+limit) + 1
			} else {
				total = int64(offset + len(results))
			}
		} else {
			total = count
		}
	}

	baseURL := "/search?q=" + q
	content := templates.SearchPage(q, results, total, baseURL, mode, offset)
	h.renderPage(w, r, "Search", content)
}
