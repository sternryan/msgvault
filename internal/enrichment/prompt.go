package enrichment

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/wesm/msgvault/internal/ai"
)

// allowedCategories is the fixed set of AI-generated categories.
// Validated post-response to prevent LLM injection of arbitrary labels.
var allowedCategories = map[string]bool{
	"finance":     true,
	"travel":      true,
	"legal":       true,
	"health":      true,
	"shopping":    true,
	"newsletters": true,
	"personal":    true,
	"work":        true,
}

// companySuffixes are stripped during normalization of company entity values.
var companySuffixes = []string{
	" Inc.", " Inc", " LLC", " Corp.", " Corp", " Ltd.", " Ltd",
}

// markdownFenceRe matches JSON wrapped in markdown code fences.
var markdownFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

// jsonObjectRe extracts a JSON object as a last-resort fallback.
var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

// EnrichResult is the parsed LLM response for a single message.
type EnrichResult struct {
	Category   string      `json:"category"`
	LifeEvents []LifeEvent `json:"life_events"`
	Entities   []Entity    `json:"entities"`
}

// LifeEvent is a life event extracted by the AI pipeline.
type LifeEvent struct {
	Date        string `json:"date"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Entity is an entity extracted by the AI pipeline.
type Entity struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// buildEnrichRequest constructs the ChatRequest for a single message.
// Only subject and snippet are sent to Azure OpenAI — never body text.
func buildEnrichRequest(subject, snippet string) ai.ChatRequest {
	systemMsg := `You are a personal email analyst. Extract structured information from emails.
Always return valid JSON matching the schema exactly. Return ONLY the JSON object, no explanation.`

	userMsg := fmt.Sprintf(`Email subject: %s
Email preview: %s

Return JSON:
{
  "category": "<one of: finance|travel|legal|health|shopping|newsletters|personal|work>",
  "life_events": [{"date":"YYYY-MM-DD","type":"<job|move|purchase|travel|milestone>","description":"..."}],
  "entities": [{"type":"<person|company|date|amount>","value":"..."}]
}

Rules:
- category: exactly one, lowercase, from the allowed list only
- life_events: empty array [] if none found; only extract significant life events
- entities: empty array [] if none; extract named people, companies, specific dates, monetary amounts`, subject, snippet)

	return ai.ChatRequest{
		Messages: []ai.ChatMessage{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		},
		Temperature: 0,
		MaxTokens:   512,
	}
}

// parseEnrichResponse parses the LLM response content into an EnrichResult.
// Handles markdown fences and partial JSON extraction as fallbacks.
func parseEnrichResponse(content string) (*EnrichResult, error) {
	content = strings.TrimSpace(content)

	// Try direct JSON unmarshal first.
	var result EnrichResult
	if err := json.Unmarshal([]byte(content), &result); err == nil {
		return &result, nil
	}

	// Try stripping markdown fences.
	if m := markdownFenceRe.FindStringSubmatch(content); len(m) == 2 {
		if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &result); err == nil {
			return &result, nil
		}
	}

	// Last resort: extract bare JSON object.
	if m := jsonObjectRe.FindString(content); m != "" {
		if err := json.Unmarshal([]byte(m), &result); err == nil {
			return &result, nil
		}
	}

	return nil, fmt.Errorf("unable to parse LLM response as JSON: %q", content)
}

// validateCategory returns the category if it's in the allowed set, otherwise "personal".
func validateCategory(cat string) string {
	if allowedCategories[cat] {
		return cat
	}
	return "personal"
}

// normalizeEntityValue normalizes an entity value for storage.
// For companies: strips common legal suffixes and lowercases.
// For all types: trims whitespace and lowercases.
func normalizeEntityValue(entityType, value string) string {
	normalized := strings.TrimSpace(value)
	if entityType == "company" {
		for _, suffix := range companySuffixes {
			if strings.HasSuffix(normalized, suffix) {
				normalized = strings.TrimSuffix(normalized, suffix)
				normalized = strings.TrimSpace(normalized)
				break
			}
		}
	}
	return strings.ToLower(normalized)
}
