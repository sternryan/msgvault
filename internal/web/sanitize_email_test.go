package web

import (
	"strings"
	"testing"

	"github.com/wesm/msgvault/internal/query"
)

// TestSanitizeEmailHTML_ScriptStripping verifies that script tags are removed.
func TestSanitizeEmailHTML_ScriptStripping(t *testing.T) {
	input := `<p>Hello</p><script>alert('xss')</script><p>World</p>`
	result := sanitizeEmailHTML(input, nil, true)

	if strings.Contains(result, "<script>") {
		t.Errorf("expected script tag to be stripped, got: %s", result)
	}
	if strings.Contains(result, "alert(") {
		t.Errorf("expected script content to be stripped, got: %s", result)
	}
	if !strings.Contains(result, "Hello") || !strings.Contains(result, "World") {
		t.Errorf("expected non-script content to be preserved, got: %s", result)
	}
}

// TestSanitizeEmailHTML_EventHandlerStripping verifies onclick, onerror, onload are stripped.
func TestSanitizeEmailHTML_EventHandlerStripping(t *testing.T) {
	tests := []struct {
		name  string
		input string
		attr  string
	}{
		{"onclick", `<a href="#" onclick="evil()">click</a>`, "onclick"},
		{"onerror", `<img src="x" onerror="evil()">`, "onerror"},
		{"onload", `<body onload="evil()">text</body>`, "onload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeEmailHTML(tt.input, nil, true)
			if strings.Contains(result, tt.attr) {
				t.Errorf("expected %s to be stripped, got: %s", tt.attr, result)
			}
		})
	}
}

// TestSanitizeEmailHTML_TablePreservation verifies table structure is preserved.
func TestSanitizeEmailHTML_TablePreservation(t *testing.T) {
	input := `<table><thead><tr><th>Header</th></tr></thead><tbody><tr><td>Cell</td></tr></tbody></table>`
	result := sanitizeEmailHTML(input, nil, true)

	for _, tag := range []string{"table", "thead", "tbody", "tr", "th", "td"} {
		if !strings.Contains(result, "<"+tag) {
			t.Errorf("expected <%s> to be preserved, got: %s", tag, result)
		}
	}
	if !strings.Contains(result, "Header") || !strings.Contains(result, "Cell") {
		t.Errorf("expected table content to be preserved, got: %s", result)
	}
}

// TestSanitizeEmailHTML_InlineStylePreservation verifies inline styles are preserved.
func TestSanitizeEmailHTML_InlineStylePreservation(t *testing.T) {
	input := `<p style="color: red; font-size: 14px;">Styled text</p>`
	result := sanitizeEmailHTML(input, nil, true)

	if !strings.Contains(result, `style=`) {
		t.Errorf("expected style attribute to be preserved, got: %s", result)
	}
	if !strings.Contains(result, "color") {
		t.Errorf("expected color style to be preserved, got: %s", result)
	}
}

// TestSanitizeEmailHTML_StyleBlockPreservation verifies style blocks are preserved.
func TestSanitizeEmailHTML_StyleBlockPreservation(t *testing.T) {
	input := `<style>.foo { color: blue; }</style><p class="foo">text</p>`
	result := sanitizeEmailHTML(input, nil, true)

	if !strings.Contains(result, "<style>") {
		t.Errorf("expected style block to be preserved, got: %s", result)
	}
	if !strings.Contains(result, ".foo") {
		t.Errorf("expected style rules to be preserved, got: %s", result)
	}
}

// TestSanitizeEmailHTML_LinkTargetBlank verifies links get target=_blank and a rel containing noopener.
// bluemonday's AddTargetBlankToFullyQualifiedLinks sets rel="nofollow noopener".
func TestSanitizeEmailHTML_LinkTargetBlank(t *testing.T) {
	input := `<a href="https://example.com">click here</a>`
	result := sanitizeEmailHTML(input, nil, true)

	if !strings.Contains(result, `target="_blank"`) {
		t.Errorf("expected target=_blank on link, got: %s", result)
	}
	if !strings.Contains(result, `noopener`) {
		t.Errorf("expected rel containing noopener on link, got: %s", result)
	}
}

// TestSubstituteCIDImages_ReplacesWithLocalURL verifies CID src is replaced with /attachments/{id}/inline.
func TestSubstituteCIDImages_ReplacesWithLocalURL(t *testing.T) {
	attachments := []query.AttachmentInfo{
		{ID: 42, ContentID: "<img001@example.com>"},
	}
	input := `<img src="cid:img001@example.com" alt="photo">`
	result := substituteCIDImages(input, attachments)

	if !strings.Contains(result, `/attachments/42/inline`) {
		t.Errorf("expected CID to be replaced with local URL, got: %s", result)
	}
	if strings.Contains(result, "cid:") {
		t.Errorf("expected cid: scheme to be removed, got: %s", result)
	}
}

// TestSubstituteCIDImages_AngleBracketWrapped verifies angle-bracket wrapped CIDs work.
func TestSubstituteCIDImages_AngleBracketWrapped(t *testing.T) {
	attachments := []query.AttachmentInfo{
		{ID: 99, ContentID: "<banner@newsletter.com>"},
	}
	// Some mailers omit angle brackets in CID ref
	input := `<img src="cid:banner@newsletter.com">`
	result := substituteCIDImages(input, attachments)

	if !strings.Contains(result, `/attachments/99/inline`) {
		t.Errorf("expected angle-bracket CID to be resolved, got: %s", result)
	}
}

// TestSubstituteCIDImages_NonCIDUnchanged verifies non-CID src attributes are unchanged.
func TestSubstituteCIDImages_NonCIDUnchanged(t *testing.T) {
	attachments := []query.AttachmentInfo{
		{ID: 1, ContentID: "<img@example.com>"},
	}
	input := `<img src="https://example.com/photo.jpg" alt="external">`
	result := substituteCIDImages(input, attachments)

	if !strings.Contains(result, "https://example.com/photo.jpg") {
		t.Errorf("expected non-CID src to be unchanged, got: %s", result)
	}
}

// TestBlockExternalImages_HTTP verifies http:// src is replaced with empty src.
func TestBlockExternalImages_HTTP(t *testing.T) {
	input := `<img src="http://tracker.example.com/pixel.gif" alt="tracker">`
	result := blockExternalImages(input)

	if strings.Contains(result, "http://tracker.example.com") {
		t.Errorf("expected http:// src to be blocked, got: %s", result)
	}
	if !strings.Contains(result, `src=""`) {
		t.Errorf("expected src to be emptied, got: %s", result)
	}
}

// TestBlockExternalImages_HTTPS verifies https:// src is replaced with empty src.
func TestBlockExternalImages_HTTPS(t *testing.T) {
	input := `<img src="https://cdn.example.com/image.png" alt="image">`
	result := blockExternalImages(input)

	if strings.Contains(result, "https://cdn.example.com") {
		t.Errorf("expected https:// src to be blocked, got: %s", result)
	}
	if !strings.Contains(result, `src=""`) {
		t.Errorf("expected src to be emptied, got: %s", result)
	}
}

// TestSanitizeEmailHTML_ShowImagesFalse_BlocksExternal verifies external images blocked when showImages=false.
func TestSanitizeEmailHTML_ShowImagesFalse_BlocksExternal(t *testing.T) {
	attachments := []query.AttachmentInfo{
		{ID: 10, ContentID: "<logo@company.com>"},
	}
	input := `<img src="cid:logo@company.com" alt="logo"><img src="https://tracker.com/px.gif" alt="tracker">`
	result := sanitizeEmailHTML(input, attachments, false)

	// CID-resolved image should be local URL (preserved)
	if !strings.Contains(result, `/attachments/10/inline`) {
		t.Errorf("expected CID-resolved image to be preserved, got: %s", result)
	}
	// External image should be blocked
	if strings.Contains(result, "https://tracker.com") {
		t.Errorf("expected external image to be blocked when showImages=false, got: %s", result)
	}
}

// TestSanitizeEmailHTML_ShowImagesTrue_PreservesExternal verifies external images preserved when showImages=true.
func TestSanitizeEmailHTML_ShowImagesTrue_PreservesExternal(t *testing.T) {
	input := `<img src="https://cdn.example.com/image.png" alt="image">`
	result := sanitizeEmailHTML(input, nil, true)

	if !strings.Contains(result, "https://cdn.example.com/image.png") {
		t.Errorf("expected external image to be preserved when showImages=true, got: %s", result)
	}
}
