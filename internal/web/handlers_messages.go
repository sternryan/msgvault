package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/web/templates"
)

// messageBody serves a sanitized standalone HTML page for the email body.
// This endpoint is loaded by the sandboxed iframe on the message detail page.
func (h *handlers) messageBody(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	msg, err := h.engine.GetMessage(ctx, id)
	if err != nil {
		h.logger.Error("failed to get message for body", "id", id, "error", err)
		http.Error(w, "Failed to load message", http.StatusInternalServerError)
		return
	}
	if msg == nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	showImages := r.URL.Query().Get("showImages") == "true"

	// Build Content-Security-Policy
	var imgSrc string
	if showImages {
		imgSrc = "img-src * data:"
	} else {
		imgSrc = "img-src 'self' data:"
	}
	csp := fmt.Sprintf("default-src 'none'; script-src 'unsafe-inline'; %s; style-src 'unsafe-inline' *; font-src *", imgSrc)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", csp)
	// Do NOT set X-Frame-Options — this page is designed to be framed

	if msg.BodyHTML == "" {
		fmt.Fprint(w, `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body><p>No HTML body available.</p></body></html>`)
		return
	}

	sanitized := sanitizeEmailHTML(msg.BodyHTML, msg.Attachments, showImages)

	// Write complete standalone HTML document with postMessage resize script
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
body { margin: 0; padding: 8px; font-family: sans-serif; }
img[src=""] { display: inline-block; min-width: 16px; min-height: 16px;
  background: #eee; border: 1px dashed #ccc; }
img[src=""]:after { content: attr(alt); font-size: 12px; color: #666; }
</style></head><body>
%s
<script>
(function(){
  var lastH = 0;
  function report() {
    var h = document.documentElement.scrollHeight;
    if (h !== lastH) { lastH = h; window.parent.postMessage({type:'msgvault-resize',height:h},'*'); }
  }
  report();
  if (window.ResizeObserver) { new ResizeObserver(report).observe(document.body); }
  window.addEventListener('load', report);
  var timer;
  if (window.MutationObserver) {
    new MutationObserver(function(){ clearTimeout(timer); timer = setTimeout(report, 50); })
      .observe(document.body, {subtree:true, childList:true, attributes:true});
  }
})();
</script></body></html>`, sanitized)
}

// messageBodyWrapper returns an HTMX-swappable fragment containing the iframe
// wrapper div. Used by "Load images" toggle to replace the wrapper (with banner
// and blocked iframe) with a new wrapper (no banner, showImages iframe).
func (h *handlers) messageBodyWrapper(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	msg, err := h.engine.GetMessage(ctx, id)
	if err != nil {
		h.logger.Error("failed to get message for body-wrapper", "id", id, "error", err)
		http.Error(w, "Failed to load message", http.StatusInternalServerError)
		return
	}
	if msg == nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	showImages := r.URL.Query().Get("showImages") == "true"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if showImages {
		// No banner — iframe src includes showImages=true
		fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">
    <iframe id="email-body-frame"
        src="/messages/%d/body?showImages=true"
        sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
        class="email-iframe"
        scrolling="no"
        frameborder="0"
    ></iframe>
</div>`, id)
	} else {
		// Include the external images banner above the iframe
		fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">
    <div class="email-images-banner">
        <span>External images blocked.</span>
        <a href="#"
           hx-get="/messages/%d/body-wrapper?showImages=true"
           hx-target="#email-body-wrapper"
           hx-swap="outerHTML">Load images</a>
    </div>
    <iframe id="email-body-frame"
        src="/messages/%d/body"
        sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
        class="email-iframe"
        scrolling="no"
        frameborder="0"
    ></iframe>
</div>`, id, id)
	}
}

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
