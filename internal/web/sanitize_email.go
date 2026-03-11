package web

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"github.com/wesm/msgvault/internal/query"
)

// emailPolicy holds the singleton bluemonday policy for email HTML rendering.
var (
	emailPolicyOnce sync.Once
	emailPolicy     *bluemonday.Policy
)

// newEmailPolicy builds a bluemonday policy tuned for rendering HTML email
// inside a sandboxed iframe. The iframe sandbox is the XSS defense;
// p.AllowUnsafe(true) is required to preserve <style> blocks.
func newEmailPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// AllowUnsafe is required so that <style> blocks survive sanitization.
	// Security note: this policy is ONLY safe when the HTML is rendered inside
	// a sandboxed iframe with allow-same-origin absent. Never render output
	// from this policy directly in the parent document.
	p.AllowUnsafe(true)

	// Structural elements
	p.AllowElements(
		"html", "head", "body",
		"div", "span",
		"p", "br", "hr",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li",
		"dl", "dt", "dd",
		"blockquote", "pre", "code",
		"b", "strong", "i", "em", "u", "s", "strike",
		"sup", "sub", "small", "big", "center",
		"style",
	)

	// Table elements — essential for HTML email layout
	p.AllowElements(
		"table", "thead", "tbody", "tfoot",
		"tr", "td", "th",
		"caption", "colgroup", "col",
	)

	// Font element with common attributes
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowElements("font")

	// Inline styles for email layout
	p.AllowAttrs("style").Globally()

	// Standard HTML attributes (id, title, dir, lang, class, etc.)
	p.AllowStandardAttributes()

	// Standard URL schemes (http, https, mailto, ftp)
	p.AllowStandardURLs()

	// Links
	p.AllowAttrs("href", "name", "target", "rel").OnElements("a")

	// Auto-inject target=_blank and rel=noopener noreferrer on fully-qualified links
	p.AddTargetBlankToFullyQualifiedLinks(true)

	// Images — allow local /attachments/... paths and external URLs (blocked separately when showImages=false)
	p.AllowImages()
	p.AllowDataURIImages()

	// Table layout attributes
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowAttrs("align", "valign", "bgcolor", "width", "height", "border", "cellpadding", "cellspacing").
		OnElements("table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col")

	// Image layout attributes
	p.AllowAttrs("width", "height", "border", "align", "alt").OnElements("img")

	return p
}

// getEmailPolicy returns the singleton email HTML policy.
func getEmailPolicy() *bluemonday.Policy {
	emailPolicyOnce.Do(func() {
		emailPolicy = newEmailPolicy()
	})
	return emailPolicy
}

// cidSrcRe matches src="cid:XXXXX" attributes (case-insensitive, single or double quotes).
var cidSrcRe = regexp.MustCompile(`(?i)src=["']cid:([^"']+)["']`)

// externalSrcRe matches src="http://..." or src="https://..." attributes.
var externalSrcRe = regexp.MustCompile(`(?i)src=["'](https?://[^"']+)["']`)

// substituteCIDImages replaces src="cid:XXXX" references with /attachments/{id}/inline
// using the provided attachment list for lookup. Unmatched CID refs are replaced
// with src="" to avoid broken cid: URLs in the browser.
func substituteCIDImages(html string, attachments []query.AttachmentInfo) string {
	if len(attachments) == 0 {
		// No attachments — strip all cid: refs
		return cidSrcRe.ReplaceAllString(html, `src=""`)
	}

	// Build lookup map: bare CID (no angle brackets) -> attachment ID
	cidToID := make(map[string]int64, len(attachments))
	for _, att := range attachments {
		if att.ContentID == "" {
			continue
		}
		// Strip angle brackets: <img001@example.com> -> img001@example.com
		bare := strings.Trim(att.ContentID, "<>")
		cidToID[bare] = att.ID
	}

	return cidSrcRe.ReplaceAllStringFunc(html, func(match string) string {
		// Extract the CID value from the match
		sub := cidSrcRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return `src=""`
		}
		// The captured group is the CID value (may or may not have angle brackets)
		cid := strings.Trim(sub[1], "<>")
		if id, ok := cidToID[cid]; ok {
			return fmt.Sprintf(`src="/attachments/%d/inline"`, id)
		}
		// Unmatched CID — replace with empty src
		return `src=""`
	})
}

// blockExternalImages replaces src="http://..." and src="https://..." with src=""
// to prevent external image loading (tracking pixels, etc.).
func blockExternalImages(html string) string {
	return externalSrcRe.ReplaceAllString(html, `src=""`)
}

// sanitizeEmailHTML processes email HTML through a three-step pipeline:
//  1. CID image substitution — replaces cid: references with local /attachments/{id}/inline URLs
//  2. External image blocking — replaces http/https src with src="" when showImages=false
//  3. HTML sanitization — strips scripts, event handlers; preserves email layout elements
//
// The result is safe to render inside a sandboxed iframe (no allow-scripts + allow-same-origin).
func sanitizeEmailHTML(html string, attachments []query.AttachmentInfo, showImages bool) string {
	// Step 1: substitute CID images before sanitization (bluemonday strips cid: scheme)
	html = substituteCIDImages(html, attachments)

	// Step 2: block external images before sanitization
	if !showImages {
		html = blockExternalImages(html)
	}

	// Step 3: sanitize with bluemonday email policy
	return getEmailPolicy().Sanitize(html)
}
