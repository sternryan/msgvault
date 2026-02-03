package vault

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryanstern/msgvault/internal/query"
	"github.com/ryanstern/msgvault/internal/store"
)

// TimelineNoteGenerator generates timeline notes (year/month).
type TimelineNoteGenerator struct {
	store     *store.Store
	engine    query.Engine
	outputDir string
	logger    *slog.Logger
}

// NewTimelineNoteGenerator creates a new timeline note generator.
func NewTimelineNoteGenerator(s *store.Store, engine query.Engine, outputDir string, logger *slog.Logger) *TimelineNoteGenerator {
	return &TimelineNoteGenerator{
		store:     s,
		engine:    engine,
		outputDir: outputDir,
		logger:    logger,
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

	// TODO: Generate yearly notes (aggregated from months)
	// For now, just return monthly count
	return monthlyCount, nil
}

// generateMonthlyNotes generates monthly timeline notes.
func (g *TimelineNoteGenerator) generateMonthlyNotes(ctx context.Context, opts ExportOptions) (int, error) {
	// Query monthly aggregates
	query := `
		SELECT
			strftime('%Y-%m', sent_at) as period,
			COUNT(*) as message_count,
			SUM(COALESCE(size_estimate, 0)) as total_size,
			COUNT(CASE WHEN sender_id IS NOT NULL THEN 1 END) as received_count
		FROM messages
		WHERE deleted_at IS NULL
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

	rows, err := g.store.Query(ctx, query, args...)
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

		// Get top people for this period
		data.TopPeople, _ = g.getTopPeople(ctx, data.Period, opts)

		// Get top labels for this period
		data.TopLabels, _ = g.getTopLabels(ctx, data.Period, opts)

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

// getTopPeople gets the top people for a time period.
func (g *TimelineNoteGenerator) getTopPeople(ctx context.Context, period string, opts ExportOptions) ([]PersonStat, error) {
	query := `
		SELECT
			p.email_address,
			COUNT(DISTINCT m.id) as count
		FROM participants p
		JOIN message_recipients mr ON mr.participant_id = p.id
		JOIN messages m ON m.id = mr.message_id
		WHERE strftime('%Y-%m', m.sent_at) = ?
			AND m.deleted_at IS NULL
		GROUP BY p.id
		ORDER BY count DESC
		LIMIT 10
	`

	rows, err := g.store.Query(ctx, query, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var people []PersonStat
	for rows.Next() {
		var person PersonStat
		if err := rows.Scan(&person.Email, &person.Count); err != nil {
			continue
		}
		people = append(people, person)
	}

	return people, nil
}

// getTopLabels gets the top labels for a time period.
func (g *TimelineNoteGenerator) getTopLabels(ctx context.Context, period string, opts ExportOptions) ([]LabelStat, error) {
	query := `
		SELECT
			l.name,
			COUNT(DISTINCT m.id) as count
		FROM labels l
		JOIN message_labels ml ON ml.label_id = l.id
		JOIN messages m ON m.id = ml.message_id
		WHERE strftime('%Y-%m', m.sent_at) = ?
			AND m.deleted_at IS NULL
		GROUP BY l.id
		ORDER BY count DESC
		LIMIT 10
	`

	rows, err := g.store.Query(ctx, query, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []LabelStat
	for rows.Next() {
		var label LabelStat
		if err := rows.Scan(&label.Name, &label.Count); err != nil {
			continue
		}
		labels = append(labels, label)
	}

	return labels, nil
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
		avgPerDay := float64(data.MessageCount) / 30.0
		if data.Granularity == "month" {
			daysInMonth := float64(daysInMonth(periodTime))
			avgPerDay = float64(data.MessageCount) / daysInMonth
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
		// Calculate next month for the before filter
		nextMonth := periodTime.AddDate(0, 1, 0)
		sb.WriteString(fmt.Sprintf("msgvault tui --filter-after %s --filter-before %s\n",
			periodTime.Format("2006-01-02"), nextMonth.Format("2006-01-02")))
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
