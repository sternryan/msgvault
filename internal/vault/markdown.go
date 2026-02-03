package vault

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// GenerateFrontmatter generates YAML frontmatter from a map of data.
func GenerateFrontmatter(data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("---\n%s---\n\n", string(yamlBytes))
}

// GenerateWikiLink creates an Obsidian wiki-link.
// If text is empty, returns [[target]]
// If text is provided, returns [[target|text]]
func GenerateWikiLink(target string, text string) string {
	if text == "" || text == target {
		return fmt.Sprintf("[[%s]]", target)
	}
	return fmt.Sprintf("[[%s|%s]]", target, text)
}

// SanitizeFilename converts a string to a safe filename.
// Removes or replaces characters that are problematic in filenames.
func SanitizeFilename(name string) string {
	// Replace path separators with dashes
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")

	// Replace other problematic characters
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "*", "-")
	name = strings.ReplaceAll(name, "?", "-")
	name = strings.ReplaceAll(name, "\"", "-")
	name = strings.ReplaceAll(name, "<", "-")
	name = strings.ReplaceAll(name, ">", "-")
	name = strings.ReplaceAll(name, "|", "-")

	// Remove control characters
	name = regexp.MustCompile(`[\x00-\x1f\x7f]`).ReplaceAllString(name, "")

	// Trim whitespace and dots from edges
	name = strings.TrimSpace(name)
	name = strings.Trim(name, ".")

	// Replace multiple consecutive spaces or dashes with single dash
	name = regexp.MustCompile(`[\s-]+`).ReplaceAllString(name, "-")

	// Trim dashes from edges
	name = strings.Trim(name, "-")

	// If empty after sanitization, use a default
	if name == "" {
		name = "unnamed"
	}

	// Truncate if too long (filesystem limits)
	if len(name) > 200 {
		name = name[:200]
	}

	return name
}

// FormatSize formats bytes as human-readable size.
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatDate formats a time in the specified format.
// Common formats:
//   - "2006-01-02" for YYYY-MM-DD
//   - "January 2, 2006" for long form
//   - "Jan 2006" for month and year
func FormatDate(t time.Time, format string) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.Format(format)
}

// EscapeMarkdown escapes special markdown characters.
// Note: Generally not needed for most text, but useful for user-generated content.
func EscapeMarkdown(text string) string {
	// Only escape characters that would break wiki-links or structure
	replacer := strings.NewReplacer(
		"[[", "\\[\\[",
		"]]", "\\]\\]",
		"|", "\\|",
	)
	return replacer.Replace(text)
}

// SafeDisplayName returns a display name or falls back to email if empty.
func SafeDisplayName(displayName, email string) string {
	if displayName != "" && displayName != email {
		return displayName
	}
	return email
}

// PersonFilename generates a safe filename for a person note.
func PersonFilename(email string) string {
	return SanitizeFilename(email) + ".md"
}

// ProjectFilename generates a safe filename for a project/label note.
func ProjectFilename(labelName string) string {
	return SanitizeFilename(labelName) + ".md"
}

// TimelineFilename generates a filename for a timeline note.
// For monthly: "2024-01 January.md"
// For yearly: "2024.md"
func TimelineFilename(period string, granularity string) string {
	if granularity == "month" {
		// period is "2024-01"
		t, err := time.Parse("2006-01", period)
		if err != nil {
			return SanitizeFilename(period) + ".md"
		}
		return fmt.Sprintf("%s %s.md", period, t.Format("January"))
	}
	// yearly or other
	return SanitizeFilename(period) + ".md"
}

// MakePath safely joins path components and creates parent directories if needed.
func MakePath(base string, components ...string) string {
	return filepath.Join(append([]string{base}, components...)...)
}

// FormatMessageCount formats a message count with proper pluralization.
func FormatMessageCount(count int) string {
	if count == 1 {
		return "1 message"
	}
	return fmt.Sprintf("%d messages", count)
}

// FormatPeriod formats a time period for display.
// Examples: "2024-01" -> "January 2024", "2024" -> "2024"
func FormatPeriod(period string) string {
	if len(period) == 7 { // YYYY-MM format
		t, err := time.Parse("2006-01", period)
		if err != nil {
			return period
		}
		return t.Format("January 2006")
	}
	return period
}

// GenerateMsgVaultURI generates a msgvault:// URI for deep linking.
// Future enhancement when URI handler is implemented.
func GenerateMsgVaultURI(action string, params map[string]string) string {
	var parts []string
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	if len(parts) > 0 {
		return fmt.Sprintf("msgvault://%s?%s", action, strings.Join(parts, "&"))
	}
	return fmt.Sprintf("msgvault://%s", action)
}
