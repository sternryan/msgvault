package web

import (
	"fmt"
	"html"
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

// messageBodyWrapper returns an HTMX-swappable fragment containing the email
// body. Supports ?format=text (plain text pre block) and ?format=html (default,
// iframe with optional showImages). Also handles ?showImages=true for the
// "Load images" toggle within the HTML view.
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

	format := r.URL.Query().Get("format")
	showImages := r.URL.Query().Get("showImages") == "true"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	hasBothFormats := msg.BodyText != "" && msg.BodyHTML != ""

	if format == "text" && msg.BodyText != "" {
		// Text view: toolbar (Text active) + pre block, no iframe/CSP
		htmlBtn := ""
		if hasBothFormats {
			htmlBtn = fmt.Sprintf(
				`<a class="email-toolbar-btn" href="#"`+
					` hx-get="/messages/%d/body-wrapper?format=html"`+
					` hx-target="closest .email-render-wrapper"`+
					` hx-swap="outerHTML"`+
					` hx-replace-url="/messages/%d?format=html">HTML</a>`,
				id, id)
		}
		fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">`+
			`<div class="email-toolbar">`+
			`<span class="email-toolbar-btn active">Text</span>`+
			`%s`+
			`</div>`+
			`<pre class="body-text-pre">%s</pre>`+
			`</div>`,
			htmlBtn,
			html.EscapeString(msg.BodyText))
		return
	}

	// HTML view (default): toolbar (if both formats) + images banner + iframe
	if msg.BodyHTML != "" {
		toolbarHTML := ""
		if hasBothFormats {
			toolbarHTML = fmt.Sprintf(
				`<div class="email-toolbar">`+
					`<a class="email-toolbar-btn" href="#"`+
					` hx-get="/messages/%d/body-wrapper?format=text"`+
					` hx-target="closest .email-render-wrapper"`+
					` hx-swap="outerHTML"`+
					` hx-replace-url="/messages/%d?format=text">Text</a>`+
					`<span class="email-toolbar-btn active">HTML</span>`+
					`<span class="email-toolbar-sep">&middot;</span>`+
					`<span>External images blocked.</span>`+
					`<a href="#"`+
					` hx-get="/messages/%d/body-wrapper?showImages=true"`+
					` hx-target="closest .email-render-wrapper"`+
					` hx-swap="outerHTML">Load images</a>`+
					`</div>`,
				id, id, id)
		}

		if showImages {
			if hasBothFormats {
				// Toolbar without images banner (images already loaded)
				toolbarHTML = fmt.Sprintf(
					`<div class="email-toolbar">`+
						`<a class="email-toolbar-btn" href="#"`+
						` hx-get="/messages/%d/body-wrapper?format=text"`+
						` hx-target="closest .email-render-wrapper"`+
						` hx-swap="outerHTML"`+
						` hx-replace-url="/messages/%d?format=text">Text</a>`+
						`<span class="email-toolbar-btn active">HTML</span>`+
						`</div>`,
					id, id)
			}
			fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">`+
				`%s`+
				`<iframe id="email-body-frame"`+
				` src="/messages/%d/body?showImages=true"`+
				` sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"`+
				` class="email-iframe"`+
				` scrolling="no"`+
				` frameborder="0"`+
				`></iframe>`+
				`</div>`,
				toolbarHTML, id)
		} else {
			bannerHTML := fmt.Sprintf(
				`<div class="email-images-banner">`+
					`<span>External images blocked.</span>`+
					`<a href="#"`+
					` hx-get="/messages/%d/body-wrapper?showImages=true"`+
					` hx-target="closest .email-render-wrapper"`+
					` hx-swap="outerHTML">Load images</a>`+
					`</div>`,
				id)
			if hasBothFormats {
				// Toolbar unifies format toggle + images banner — no separate banner
				bannerHTML = ""
				fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">`+
					`%s`+
					`<iframe id="email-body-frame"`+
					` src="/messages/%d/body"`+
					` sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"`+
					` class="email-iframe"`+
					` scrolling="no"`+
					` frameborder="0"`+
					`></iframe>`+
					`</div>`,
					toolbarHTML, id)
			} else {
				fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">`+
					`%s`+
					`<iframe id="email-body-frame"`+
					` src="/messages/%d/body"`+
					` sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"`+
					` class="email-iframe"`+
					` scrolling="no"`+
					` frameborder="0"`+
					`></iframe>`+
					`</div>`,
					bannerHTML, id)
			}
		}
		return
	}

	// Fallback: only text body available
	if msg.BodyText != "" {
		fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">`+
			`<pre class="body-text-pre">%s</pre>`+
			`</div>`,
			html.EscapeString(msg.BodyText))
		return
	}

	fmt.Fprintf(w, `<div id="email-body-wrapper" class="email-render-wrapper">`+
		`<p class="body-empty">No message body available.</p>`+
		`</div>`)
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

	format := r.URL.Query().Get("format")
	content := templates.MessageDetailPage(msg, format)
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
