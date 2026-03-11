package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/web/templates"
)

// deletionsPage renders the deletions management page.
func (h *handlers) deletionsPage(w http.ResponseWriter, r *http.Request) {
	manifests, err := h.collectAllManifests()
	if err != nil {
		h.logger.Error("failed to list manifests", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "failed to load deletions")
		return
	}

	content := templates.DeletionsPage(manifests)
	h.renderPage(w, r, "Deletions", content)
}

// collectAllManifests collects manifests from all status directories, sorted by CreatedAt descending.
func (h *handlers) collectAllManifests() ([]*deletion.Manifest, error) {
	if h.deletions == nil {
		return nil, nil
	}

	var all []*deletion.Manifest

	pending, err := h.deletions.ListPending()
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	all = append(all, pending...)

	inProgress, err := h.deletions.ListInProgress()
	if err != nil {
		return nil, fmt.Errorf("list in_progress: %w", err)
	}
	all = append(all, inProgress...)

	completed, err := h.deletions.ListCompleted()
	if err != nil {
		return nil, fmt.Errorf("list completed: %w", err)
	}
	all = append(all, completed...)

	failed, err := h.deletions.ListFailed()
	if err != nil {
		return nil, fmt.Errorf("list failed: %w", err)
	}
	all = append(all, failed...)

	// Sort by CreatedAt descending (most recent first).
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	return all, nil
}

// stageDeletion handles POST /deletions/stage — creates a manifest from the current filter context.
// This is an HTMX form POST from the aggregate drill-down page.
// The response contains TWO root-level elements: a primary swap target and an OOB badge update.
func (h *handlers) stageDeletion(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	// Build MessageFilter from form values.
	filter := buildFilterFromForm(r)

	// Resolve matching Gmail IDs.
	gmailIDs, err := h.engine.GetGmailIDsByFilter(r.Context(), filter)
	if err != nil {
		h.logger.Error("GetGmailIDsByFilter failed", "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div id="stage-result" class="stage-error">Error: failed to resolve message IDs</div>`)
		return
	}

	if len(gmailIDs) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div id="stage-result" class="stage-error">No messages found matching the current filter</div>`)
		return
	}

	// Build description from filter context if not provided.
	description := r.FormValue("description")
	if description == "" {
		description = buildDescriptionFromForm(r)
	}

	// Build deletion.Filters from form values.
	filters := buildDeletionFiltersFromForm(r)

	// Create the manifest.
	manifest, err := h.deletions.CreateManifest(description, gmailIDs, filters)
	if err != nil {
		h.logger.Error("CreateManifest failed", "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div id="stage-result" class="stage-error">Error: failed to create deletion batch</div>`)
		return
	}

	// Set CreatedBy = "web".
	manifest.CreatedBy = "web"
	if err := h.deletions.SaveManifest(manifest); err != nil {
		h.logger.Error("SaveManifest failed after setting CreatedBy", "error", err)
		// Non-fatal — manifest was already created.
	}

	pendingCount := h.pendingDeletionCount()

	// Write two root-level elements for HTMX OOB swap.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.StageResult(len(gmailIDs), description).Render(r.Context(), w); err != nil {
		h.logger.Error("failed to render StageResult", "error", err)
		return
	}
	if err := templates.DeletionBadgeOOB(pendingCount).Render(r.Context(), w); err != nil {
		h.logger.Error("failed to render DeletionBadgeOOB", "error", err)
	}
}

// cancelDeletion handles DELETE /deletions/{id} — removes a pending deletion manifest.
func (h *handlers) cancelDeletion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "deletion id is required", http.StatusBadRequest)
		return
	}

	if err := h.deletions.CancelManifest(id); err != nil {
		h.logger.Error("CancelManifest failed", "id", id, "error", err)
		http.Error(w, "failed to cancel deletion", http.StatusInternalServerError)
		return
	}

	pendingCount := h.pendingDeletionCount()

	// Respond: empty row replacement + OOB badge update.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The hx-target="closest tr" will replace with empty HTML (row disappears).
	if err := templates.DeletionBadgeOOB(pendingCount).Render(r.Context(), w); err != nil {
		h.logger.Error("failed to render DeletionBadgeOOB", "error", err)
	}
}

// buildFilterFromForm constructs a query.MessageFilter from form POST values.
func buildFilterFromForm(r *http.Request) query.MessageFilter {
	filter := query.MessageFilter{}

	if v := r.FormValue("sender"); v != "" {
		filter.Sender = v
	}
	if v := r.FormValue("senderName"); v != "" {
		filter.SenderName = v
	}
	if v := r.FormValue("recipient"); v != "" {
		filter.Recipient = v
	}
	if v := r.FormValue("recipientName"); v != "" {
		filter.RecipientName = v
	}
	if v := r.FormValue("domain"); v != "" {
		filter.Domain = v
	}
	if v := r.FormValue("label"); v != "" {
		filter.Label = v
	}
	if v := r.FormValue("timePeriod"); v != "" {
		filter.TimeRange.Period = v
	}
	if v := r.FormValue("sourceId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.SourceID = &id
		}
	}

	return filter
}

// buildDeletionFiltersFromForm constructs a deletion.Filters from form POST values.
func buildDeletionFiltersFromForm(r *http.Request) deletion.Filters {
	filters := deletion.Filters{}

	if v := r.FormValue("sender"); v != "" {
		filters.Senders = []string{v}
	}
	if v := r.FormValue("domain"); v != "" {
		filters.SenderDomains = []string{v}
	}
	if v := r.FormValue("recipient"); v != "" {
		filters.Recipients = []string{v}
	}
	if v := r.FormValue("label"); v != "" {
		filters.Labels = []string{v}
	}

	return filters
}

// buildDescriptionFromForm generates a human-readable description from form filter values.
func buildDescriptionFromForm(r *http.Request) string {
	if v := r.FormValue("sender"); v != "" {
		return fmt.Sprintf("Messages from %s", v)
	}
	if v := r.FormValue("senderName"); v != "" {
		return fmt.Sprintf("Messages from %s", v)
	}
	if v := r.FormValue("recipient"); v != "" {
		return fmt.Sprintf("Messages to %s", v)
	}
	if v := r.FormValue("recipientName"); v != "" {
		return fmt.Sprintf("Messages to %s", v)
	}
	if v := r.FormValue("domain"); v != "" {
		return fmt.Sprintf("Messages from domain %s", v)
	}
	if v := r.FormValue("label"); v != "" {
		return fmt.Sprintf("Messages with label %s", v)
	}
	if v := r.FormValue("timePeriod"); v != "" {
		return fmt.Sprintf("Messages from %s", v)
	}
	return "Staged messages"
}
