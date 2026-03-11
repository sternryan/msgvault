package templates

import (
	"fmt"
	"strings"
	"time"

	"github.com/wesm/msgvault/internal/query"
)

// BreadcrumbItem is a navigable breadcrumb segment used in drill-down pages.
type BreadcrumbItem struct {
	Label string
	URL   string
}

// FormatBytes converts a byte count to a human-readable string (KB, MB, GB).
func FormatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// FormatNum formats a number with comma separators.
func FormatNum(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

// FormatTime formats a time as "Jan 02, 2006 3:04 PM".
func FormatTime(t time.Time) string {
	return t.Format("Jan 02, 2006 3:04 PM")
}

// FormatDate formats a time as "Jan 02, 2006".
func FormatDate(t time.Time) string {
	return t.Format("Jan 02, 2006")
}

// Pluralize returns singular or plural based on count.
func Pluralize(n int64, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// RelativeDate formats a time relative to now for thread summary display.
func RelativeDate(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	switch {
	case diff < 24*time.Hour:
		return t.Format("3:04 PM")
	case diff < 7*24*time.Hour:
		return t.Format("Mon 3:04 PM")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}

// Truncate truncates a string to n runes, appending ellipsis if truncated.
func Truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// MaxAggregateCount returns the maximum Count across a slice of AggregateRows.
func MaxAggregateCount(rows []query.AggregateRow) int64 {
	var max int64
	for _, r := range rows {
		if r.Count > max {
			max = r.Count
		}
	}
	return max
}

// BarPercent returns the bar width as a CSS percentage string (e.g. "73.4").
func BarPercent(count, maxCount int64) string {
	if maxCount == 0 {
		return "0"
	}
	pct := float64(count) / float64(maxCount) * 100
	return fmt.Sprintf("%.1f", pct)
}
