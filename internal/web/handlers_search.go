package web

import (
	"net/http"

	"github.com/wesm/msgvault/internal/embedding"
	"github.com/wesm/msgvault/internal/search"
	"github.com/wesm/msgvault/internal/web/templates"
)

// searchPage handles GET /search.
//
// mode= query parameter controls the search path:
//   - "" or "keyword": keyword search (DuckDB fast path + FTS5 fallback)
//   - "semantic": vector similarity search via Azure OpenAI
//   - "hybrid": RRF-combined FTS5 + vector search
//
// When h.aiClient == nil and mode is semantic/hybrid, falls back to keyword
// search and shows an unconfigured notice.
func (h *handlers) searchPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	mode := r.URL.Query().Get("mode")

	// Validate mode — T-13-06: reject unknown values, fall back to keyword.
	switch mode {
	case "", "keyword", "semantic", "hybrid":
		// valid
	default:
		mode = "keyword"
	}

	hasAI := h.aiClient != nil && h.store != nil

	if q == "" {
		// Empty query — show page with syntax help.
		content := templates.SearchPage("", nil, 0, "/search", mode, 0, nil, hasAI)
		h.renderPage(w, r, "Search", content)
		return
	}

	// Route to the appropriate search handler.
	if (mode == "semantic" || mode == "hybrid") && hasAI {
		h.semanticOrHybridSearch(w, r, q, mode, hasAI)
		return
	}

	// If semantic/hybrid requested but AI not configured, fall back to keyword
	// with a graceful degradation (tabs hidden when hasAI is false).
	h.keywordSearch(w, r, q, mode, hasAI)
}

// keywordSearch runs the existing DuckDB fast path + FTS5 fallback search.
func (h *handlers) keywordSearch(w http.ResponseWriter, r *http.Request, q, mode string, hasAI bool) {
	parsed := search.Parse(q)
	filter := parseMessageFilter(r)
	offset := intParam(r, "offset", 0)
	const limit = 50

	results, err := h.engine.SearchFast(r.Context(), parsed, filter, limit, offset)
	if err != nil {
		h.logger.Error("search fast failed", "q", q, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "search failed")
		return
	}

	displayMode := "metadata"
	var total int64

	if len(results) == 0 && len(parsed.TextTerms) > 0 {
		// Fast path returned nothing and query has text terms — try FTS5 deep search.
		deepResults, err := h.engine.Search(r.Context(), parsed, limit, offset)
		if err != nil {
			h.logger.Error("search (deep) failed", "q", q, "error", err)
			// Fall through with empty results.
		} else {
			results = deepResults
			displayMode = "full-text"
		}
		if len(results) == limit {
			total = int64(offset+limit) + 1
		} else {
			total = int64(offset + len(results))
		}
	} else {
		count, err := h.engine.SearchFastCount(r.Context(), parsed, filter)
		if err != nil {
			h.logger.Error("search fast count failed", "q", q, "error", err)
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
	content := templates.SearchPage(q, results, total, baseURL, displayMode, offset, nil, hasAI)
	h.renderPage(w, r, "Search", content)
}

// semanticOrHybridSearch handles mode=semantic and mode=hybrid requests.
func (h *handlers) semanticOrHybridSearch(w http.ResponseWriter, r *http.Request, q, mode string, hasAI bool) {
	const limit = 50

	var (
		results []embedding.SemanticResult
		err     error
	)

	if mode == "semantic" {
		results, err = embedding.SemanticSearch(r.Context(), h.aiClient, h.store, q, limit)
	} else {
		results, err = embedding.HybridSearch(r.Context(), h.aiClient, h.store, h.engine, q, limit)
	}

	if err != nil {
		h.logger.Error("semantic/hybrid search failed", "q", q, "mode", mode, "error", err)
		// Fall back to keyword search rather than showing an error page.
		h.keywordSearch(w, r, q, "keyword", hasAI)
		return
	}

	content := templates.SearchPage(q, nil, 0, "/search?q="+q, mode, 0, results, hasAI)
	h.renderPage(w, r, "Search", content)
}
