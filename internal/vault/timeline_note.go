package vault

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/store"
)

// TimelineNoteGenerator generates timeline notes (year/month).
type TimelineNoteGenerator struct {
	store       *store.Store
	engine      query.Engine
	outputDir   string
	logger      *slog.Logger
	bulkFetcher BulkDataFetcher
}

// NewTimelineNoteGenerator creates a new timeline note generator.
func NewTimelineNoteGenerator(s *store.Store, engine query.Engine, outputDir string, logger *slog.Logger, bulkFetcher BulkDataFetcher) *TimelineNoteGenerator {
	return &TimelineNoteGenerator{
		store:       s,
		engine:      engine,
		outputDir:   outputDir,
		logger:      logger,
		bulkFetcher: bulkFetcher,
	}
}

// TimelineData holds data for a timeline note.
type TimelineData struct {
	Period        string
	Granularity   string // "year" or "month"
	MessageCount  int
	SentCount     int
	ReceivedCount int
	TotalSize     int64
	TopPeople     []PersonStat
	TopLabels     []LabelStat
}

// Generate generates timeline notes.
func (g *TimelineNoteGenerator) Generate(ctx context.Context, opts ExportOptions) (int, error) {
	// Generate monthly timeline notes
	monthlyCount, err := g.generateMonthlyNotes(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to generate monthly notes: %w", err)
	}

	yearlyCount, err := g.generateYearlyNotes(ctx, opts)
	if err != nil {
		return monthlyCount, fmt.Errorf("failed to generate yearly notes: %w", err)
	}

	return monthlyCount + yearlyCount, nil
}

// generateMonthlyNotes generates monthly timeline notes.
func (g *TimelineNoteGenerator) generateMonthlyNotes(ctx context.Context, opts ExportOptions) (int, error) {
	// Pre-fetch all bulk data before the main loop (eliminates N+1 queries)
	g.logger.Debug("pre-fetching bulk data for all timeline periods")
	topPeopleMap, err := g.bulkFetcher.FetchAllTopPeopleByMonth(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch top people: %w", err)
	}

	topLabelsMap, err := g.bulkFetcher.FetchAllTopLabelsByMonth(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch top labels: %w", err)
	}
	g.logger.Debug("bulk data pre-fetch complete")

	// Query monthly aggregates
	query := `
		SELECT
			strftime('%Y-%m', sent_at) as period,
			COUNT(*) as message_count,
			SUM(COALESCE(size_estimate, 0)) as total_size,
			COUNT(CASE WHEN sender_id IS NOT NULL THEN 1 END) as received_count
		FROM messages
		WHERE deleted_at IS NULL
			AND sent_at IS NOT NULL
	`

	args := []interface{}{}

	// Apply filters
	if !opts.After.IsZero() {
		query += ` AND sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
		GROUP BY period
		ORDER BY period DESC
	`

	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit)
	}

	rows, err := g.store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to query timeline: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var data TimelineData
		data.Granularity = "month"

		err := rows.Scan(
			&data.Period,
			&data.MessageCount,
			&data.TotalSize,
			&data.ReceivedCount,
		)
		if err != nil {
			g.logger.Warn("failed to scan timeline row", "error", err)
			continue
		}

		data.SentCount = data.MessageCount - data.ReceivedCount

		// Lookup bulk data from pre-fetched maps (O(1) instead of N queries)
		data.TopPeople = topPeopleMap[data.Period]
		data.TopLabels = topLabelsMap[data.Period]

		// Generate note
		if !opts.DryRun {
			if err := g.generateNote(data); err != nil {
				g.logger.Warn("failed to generate timeline note", "period", data.Period, "error", err)
				continue
			}
		}

		count++
		if count%50 == 0 {
			g.logger.Debug("progress", "timeline_notes_generated", count)
		}
	}

	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("error iterating rows: %w", err)
	}

	return count, nil
}

// generateYearlyNotes generates yearly timeline notes aggregated from message data.
func (g *TimelineNoteGenerator) generateYearlyNotes(ctx context.Context, opts ExportOptions) (int, error) {
	query := `
		SELECT
			strftime('%Y', sent_at) as period,
			COUNT(*) as message_count,
			SUM(COALESCE(size_estimate, 0)) as total_size,
			COUNT(CASE WHEN sender_id IS NOT NULL THEN 1 END) as received_count
		FROM messages
		WHERE deleted_at IS NULL
			AND sent_at IS NOT NULL
	`

	args := []interface{}{}

	if !opts.After.IsZero() {
		query += ` AND sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
		GROUP BY period
		ORDER BY period DESC
	`

	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit)
	}

	rows, err := g.store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to query yearly timeline: %w", err)
	}
	defer rows.Close()

	// Fetch top people/labels per year
	topPeopleMap, err := g.fetchTopPeopleByYear(ctx, opts)
	if err != nil {
		g.logger.Warn("failed to fetch yearly top people", "error", err)
		topPeopleMap = make(map[string][]PersonStat)
	}
	topLabelsMap, err := g.fetchTopLabelsByYear(ctx, opts)
	if err != nil {
		g.logger.Warn("failed to fetch yearly top labels", "error", err)
		topLabelsMap = make(map[string][]LabelStat)
	}

	count := 0
	for rows.Next() {
		var data TimelineData
		data.Granularity = "year"

		err := rows.Scan(
			&data.Period,
			&data.MessageCount,
			&data.TotalSize,
			&data.ReceivedCount,
		)
		if err != nil {
			g.logger.Warn("failed to scan yearly timeline row", "error", err)
			continue
		}

		data.SentCount = data.MessageCount - data.ReceivedCount
		data.TopPeople = topPeopleMap[data.Period]
		data.TopLabels = topLabelsMap[data.Period]

		if !opts.DryRun {
			if err := g.generateNote(data); err != nil {
				g.logger.Warn("failed to generate yearly note", "period", data.Period, "error", err)
				continue
			}
		}

		count++
	}

	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("error iterating yearly rows: %w", err)
	}

	return count, nil
}

// fetchTopPeopleByYear fetches top correspondents grouped by year.
func (g *TimelineNoteGenerator) fetchTopPeopleByYear(ctx context.Context, opts ExportOptions) (map[string][]PersonStat, error) {
	query := `
		WITH period_people AS (
			SELECT
				strftime('%Y', m.sent_at) as period,
				p.email_address,
				COUNT(DISTINCT m.id) as msg_count
			FROM messages m
			JOIN message_recipients mr ON mr.message_id = m.id
			JOIN participants p ON p.id = mr.participant_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	query += `
			GROUP BY period, p.id
		),
		ranked AS (
			SELECT period, email_address, msg_count,
				ROW_NUMBER() OVER (PARTITION BY period ORDER BY msg_count DESC) as rn
			FROM period_people
		)
		SELECT period, email_address, msg_count
		FROM ranked WHERE rn <= 10
		ORDER BY period, rn
	`
	rows, err := g.store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]PersonStat)
	for rows.Next() {
		var period, email string
		var count int
		if err := rows.Scan(&period, &email, &count); err != nil {
			return nil, err
		}
		result[period] = append(result[period], PersonStat{Email: email, Count: count})
	}
	return result, rows.Err()
}

// fetchTopLabelsByYear fetches top labels grouped by year.
func (g *TimelineNoteGenerator) fetchTopLabelsByYear(ctx context.Context, opts ExportOptions) (map[string][]LabelStat, error) {
	query := `
		WITH period_labels AS (
			SELECT
				strftime('%Y', m.sent_at) as period,
				l.name as label_name,
				COUNT(DISTINCT m.id) as msg_count
			FROM messages m
			JOIN message_labels ml ON ml.message_id = m.id
			JOIN labels l ON l.id = ml.label_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	query += `
			GROUP BY period, l.id
		),
		ranked AS (
			SELECT period, label_name, msg_count,
				ROW_NUMBER() OVER (PARTITION BY period ORDER BY msg_count DESC) as rn
			FROM period_labels
		)
		SELECT period, label_name, msg_count
		FROM ranked WHERE rn <= 10
		ORDER BY period, rn
	`
	rows, err := g.store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]LabelStat)
	for rows.Next() {
		var period, labelName string
		var count int
		if err := rows.Scan(&period, &labelName, &count); err != nil {
			return nil, err
		}
		result[period] = append(result[period], LabelStat{Name: labelName, Count: count})
	}
	return result, rows.Err()
}

// generateNote generates a timeline note file.
func (g *TimelineNoteGenerator) generateNote(data TimelineData) error {
	// Parse period for better formatting
	var periodTime time.Time
	var err error
	if data.Granularity == "month" {
		periodTime, err = time.Parse("2006-01", data.Period)
		if err != nil {
			return fmt.Errorf("failed to parse period: %w", err)
		}
	}

	// Generate frontmatter
	frontmatter := map[string]interface{}{
		"created":       time.Now().Format(time.RFC3339),
		"tags":          []string{"timeline"},
		"period":        data.Period,
		"message_count": data.MessageCount,
	}

	if data.Granularity == "month" {
		frontmatter["tags"] = append(frontmatter["tags"].([]string),
			fmt.Sprintf("year/%s", data.Period[:4]),
			fmt.Sprintf("month/%s", data.Period[5:7]))
	} else if data.Granularity == "year" {
		frontmatter["tags"] = append(frontmatter["tags"].([]string),
			fmt.Sprintf("year/%s", data.Period))
	}

	// Build note content
	var sb strings.Builder

	// Frontmatter
	sb.WriteString(GenerateFrontmatter(frontmatter))

	// Title
	if data.Granularity == "month" {
		sb.WriteString(fmt.Sprintf("# %s\n\n", periodTime.Format("January 2006")))
	} else {
		sb.WriteString(fmt.Sprintf("# %s\n\n", data.Period))
	}

	// Overview
	sb.WriteString("## Overview\n\n")
	sb.WriteString(fmt.Sprintf("- **Messages:** %s", FormatMessageCount(data.MessageCount)))
	if data.MessageCount > 0 {
		var avgPerDay float64
		if data.Granularity == "month" {
			avgPerDay = float64(data.MessageCount) / float64(daysInMonth(periodTime))
		} else {
			avgPerDay = float64(data.MessageCount) / 365.0
		}
		sb.WriteString(fmt.Sprintf(" (avg %.0f/day)", avgPerDay))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("- **Sent:** %s\n", FormatMessageCount(data.SentCount)))
	sb.WriteString(fmt.Sprintf("- **Received:** %s\n", FormatMessageCount(data.ReceivedCount)))
	sb.WriteString(fmt.Sprintf("- **Total Size:** %s\n", FormatSize(data.TotalSize)))
	sb.WriteString("\n")

	// Top Correspondents
	if len(data.TopPeople) > 0 {
		sb.WriteString("## Top Correspondents\n\n")
		for _, person := range data.TopPeople {
			sb.WriteString(fmt.Sprintf("- [[People/%s|%s]] - %s\n",
				PersonFilename(person.Email), person.Email, FormatMessageCount(person.Count)))
		}
		sb.WriteString("\n")
	}

	// Active Projects
	if len(data.TopLabels) > 0 {
		sb.WriteString("## Active Projects\n\n")
		for _, label := range data.TopLabels {
			sb.WriteString(fmt.Sprintf("- [[Projects/%s|%s]] - %s\n",
				ProjectFilename(label.Name), label.Name, FormatMessageCount(label.Count)))
		}
		sb.WriteString("\n")
	}

	// Quick Links
	sb.WriteString("## Quick Links\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString(fmt.Sprintf("# View messages from %s\n", FormatPeriod(data.Period)))
	if data.Granularity == "month" {
		nextMonth := periodTime.AddDate(0, 1, 0)
		sb.WriteString(fmt.Sprintf("msgvault tui --filter-after %s --filter-before %s\n",
			periodTime.Format("2006-01-02"), nextMonth.Format("2006-01-02")))
	} else if data.Granularity == "year" {
		sb.WriteString(fmt.Sprintf("msgvault tui --filter-after %s-01-01 --filter-before %s-01-01\n",
			data.Period, nextYear(data.Period)))
	}
	sb.WriteString("```\n\n")

	// Navigation
	if data.Granularity == "month" {
		sb.WriteString("## Navigation\n\n")
		prevMonth := periodTime.AddDate(0, -1, 0)
		nextMonth := periodTime.AddDate(0, 1, 0)
		sb.WriteString(fmt.Sprintf("← [[Timeline/%s/%s|%s]] | [[Timeline/%s/%s|%s]] →\n\n",
			prevMonth.Format("2006"), TimelineFilename(prevMonth.Format("2006-01"), "month"), prevMonth.Format("January 2006"),
			nextMonth.Format("2006"), TimelineFilename(nextMonth.Format("2006-01"), "month"), nextMonth.Format("January 2006")))
	} else if data.Granularity == "year" {
		sb.WriteString("## Navigation\n\n")
		prev := prevYear(data.Period)
		next := nextYear(data.Period)
		sb.WriteString(fmt.Sprintf("← [[Timeline/%s/%s|%s]] | [[Timeline/%s/%s|%s]] →\n\n",
			prev, TimelineFilename(prev, "year"), prev,
			next, TimelineFilename(next, "year"), next))
	}

	// MOCs
	sb.WriteString("## MOCs\n\n")
	sb.WriteString("- [[MOC - Email Archive]]\n")

	// Write file
	filename := TimelineFilename(data.Period, data.Granularity)
	yearDir := filepath.Join(g.outputDir, "Timeline", data.Period[:4])

	// Create year directory if it doesn't exist
	if err := os.MkdirAll(yearDir, 0755); err != nil {
		return fmt.Errorf("failed to create year directory: %w", err)
	}

	path := filepath.Join(yearDir, filename)
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// daysInMonth returns the number of days in a month.
func daysInMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// nextYear returns the year string incremented by 1.
func nextYear(year string) string {
	t, err := time.Parse("2006", year)
	if err != nil {
		return year
	}
	return t.AddDate(1, 0, 0).Format("2006")
}

// prevYear returns the year string decremented by 1.
func prevYear(year string) string {
	t, err := time.Parse("2006", year)
	if err != nil {
		return year
	}
	return t.AddDate(-1, 0, 0).Format("2006")
}
