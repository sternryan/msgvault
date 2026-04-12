package web

import (
	"net/http"

	"github.com/wesm/msgvault/internal/web/templates"
)

// validEntityTypes is the allowed set of entity type filter values.
var validEntityTypes = map[string]bool{
	"person":  true,
	"company": true,
	"date":    true,
	"amount":  true,
}

const defaultEntitiesPerPage = 50

// entitiesPage handles GET /entities — renders the full entities page.
func (h *handlers) entitiesPage(w http.ResponseWriter, r *http.Request) {
	entityType, q, offset, limit := parseEntitiesParams(r)

	if h.store == nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "enrichment store not available")
		return
	}

	entities, total, err := h.store.GetEntities(entityType, q, limit, offset)
	if err != nil {
		h.logger.Error("get entities failed", "entityType", entityType, "q", q, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "failed to load entities")
		return
	}

	content := templates.EntitiesPage(entities, entityType, q, offset, limit, total)

	// If HTMX request, render only the partial (same as full page but caller just needs the body).
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := content.Render(r.Context(), w); err != nil {
			h.logger.Error("failed to render entities partial", "error", err)
		}
		return
	}

	h.renderPage(w, r, "Entities", content)
}

// entitiesPartial handles GET /entities/partial — renders only the table content for HTMX swaps.
func (h *handlers) entitiesPartial(w http.ResponseWriter, r *http.Request) {
	entityType, q, offset, limit := parseEntitiesParams(r)

	if h.store == nil {
		http.Error(w, "enrichment store not available", http.StatusServiceUnavailable)
		return
	}

	entities, total, err := h.store.GetEntities(entityType, q, limit, offset)
	if err != nil {
		h.logger.Error("get entities partial failed", "entityType", entityType, "q", q, "error", err)
		http.Error(w, "failed to load entities", http.StatusInternalServerError)
		return
	}

	partial := templates.EntitiesTableContent(entities, entityType, q, offset, limit, total)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partial.Render(r.Context(), w); err != nil {
		h.logger.Error("failed to render entities table content", "error", err)
	}
}

// parseEntitiesParams extracts and validates entity query params from the request.
// Returns (entityType, q, offset, limit).
func parseEntitiesParams(r *http.Request) (string, string, int, int) {
	entityType := r.URL.Query().Get("type")
	// Validate type against allowlist (T-14-05 mitigation).
	if entityType != "" && !validEntityTypes[entityType] {
		entityType = ""
	}

	q := r.URL.Query().Get("q")
	offset := intParam(r, "offset", 0)
	limit := clampInt(intParam(r, "limit", defaultEntitiesPerPage), 1, maxLimit)

	return entityType, q, offset, limit
}
