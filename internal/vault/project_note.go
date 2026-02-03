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

// ProjectNoteGenerator generates project/label notes.
type ProjectNoteGenerator struct {
	store       *store.Store
	engine      query.Engine
	outputDir   string
	logger      *slog.Logger
	bulkFetcher BulkDataFetcher
}

// NewProjectNoteGenerator creates a new project note generator.
func NewProjectNoteGenerator(s *store.Store, engine query.Engine, outputDir string, logger *slog.Logger, bulkFetcher BulkDataFetcher) *ProjectNoteGenerator {
	return &ProjectNoteGenerator{
		store:       s,
		engine:      engine,
		outputDir:   outputDir,
		logger:      logger,
		bulkFetcher: bulkFetcher,
	}
}

// ProjectData holds data for a project/label note.
type ProjectData struct {
	LabelName    string
	LabelType    string
	MessageCount int
	TotalSize    int64
	FirstMessage time.Time
	LastMessage  time.Time
	TopPeople    []PersonStat
	RecentMonths []string
}

// PersonStat represents person statistics for a project.
type PersonStat struct {
	Email string
	Count int
}

// Generate generates project/label notes.
func (g *ProjectNoteGenerator) Generate(ctx context.Context, opts ExportOptions) (int, error) {
	// Pre-fetch all bulk data before the main loop (eliminates N+1 queries)
	g.logger.Debug("pre-fetching bulk data for all projects")
	topPeopleMap, err := g.bulkFetcher.FetchAllTopPeopleByProject(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch top people: %w", err)
	}

	recentMonthsMap, err := g.bulkFetcher.FetchAllRecentMonthsByProject(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch recent months: %w", err)
	}
	g.logger.Debug("bulk data pre-fetch complete")

	// Query all labels with aggregated stats
	query := `
		SELECT
			l.name,
			l.label_type,
			COUNT(DISTINCT m.id) as message_count,
			SUM(COALESCE(m.size_estimate, 0)) as total_size,
			MIN(m.sent_at) as first_message,
			MAX(m.sent_at) as last_message
		FROM labels l
		JOIN message_labels ml ON ml.label_id = l.id
		JOIN messages m ON m.id = ml.message_id
		WHERE m.deleted_at IS NULL
	`

	args := []interface{}{}

	// Apply filters
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
		GROUP BY l.id
		ORDER BY message_count DESC
	`

	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit)
	}

	rows, err := g.store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to query labels: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var data ProjectData
		var firstMessage, lastMessage interface{}

		err := rows.Scan(
			&data.LabelName,
			&data.LabelType,
			&data.MessageCount,
			&data.TotalSize,
			&firstMessage,
			&lastMessage,
		)
		if err != nil {
			g.logger.Warn("failed to scan project row", "error", err)
			continue
		}

		// Parse timestamps (handles both SQLite sql.NullTime and DuckDB strings)
		if t, ok := ParseNullableTimestamp(firstMessage); ok {
			data.FirstMessage = t
		}
		if t, ok := ParseNullableTimestamp(lastMessage); ok {
			data.LastMessage = t
		}

		// Lookup bulk data from pre-fetched maps (O(1) instead of N queries)
		data.TopPeople = topPeopleMap[data.LabelName]
		data.RecentMonths = recentMonthsMap[data.LabelName]

		// Generate note
		if !opts.DryRun {
			if err := g.generateNote(data); err != nil {
				g.logger.Warn("failed to generate project note", "label", data.LabelName, "error", err)
				continue
			}
		}

		count++
		if count%50 == 0 {
			g.logger.Debug("progress", "projects_generated", count)
		}
	}

	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("error iterating rows: %w", err)
	}

	return count, nil
}

// generateNote generates a project note file.
func (g *ProjectNoteGenerator) generateNote(data ProjectData) error {
	// Generate frontmatter
	frontmatter := map[string]interface{}{
		"created":       time.Now().Format(time.RFC3339),
		"tags":          []string{"project", "label"},
		"label":         data.LabelName,
		"message_count": data.MessageCount,
	}

	if !data.FirstMessage.IsZero() {
		frontmatter["first_message"] = data.FirstMessage.Format("2006-01-02")
	}
	if !data.LastMessage.IsZero() {
		frontmatter["last_message"] = data.LastMessage.Format("2006-01-02")
	}

	// Build note content
	var sb strings.Builder

	// Frontmatter
	sb.WriteString(GenerateFrontmatter(frontmatter))

	// Title
	sb.WriteString(fmt.Sprintf("# Project: %s\n\n", data.LabelName))

	// Overview
	sb.WriteString("## Overview\n\n")
	sb.WriteString(fmt.Sprintf("- **Label:** %s\n", data.LabelName))
	if data.LabelType != "" {
		sb.WriteString(fmt.Sprintf("- **Type:** %s\n", data.LabelType))
	}
	if !data.FirstMessage.IsZero() && !data.LastMessage.IsZero() {
		duration := data.LastMessage.Sub(data.FirstMessage)
		years := duration.Hours() / 24 / 365
		sb.WriteString(fmt.Sprintf("- **Duration:** %s - %s (%.1f years)\n",
			FormatDate(data.FirstMessage, "January 2006"),
			FormatDate(data.LastMessage, "January 2006"),
			years))
	}
	sb.WriteString(fmt.Sprintf("- **Total Messages:** %s\n", FormatMessageCount(data.MessageCount)))
	sb.WriteString(fmt.Sprintf("- **Total Size:** %s\n", FormatSize(data.TotalSize)))
	if len(data.TopPeople) > 0 {
		sb.WriteString(fmt.Sprintf("- **Key Participants:** %d people\n", len(data.TopPeople)))
	}
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

	// Timeline
	if len(data.RecentMonths) > 0 {
		sb.WriteString("## Timeline\n\n")
		for i, month := range data.RecentMonths {
			sb.WriteString(fmt.Sprintf("- [[Timeline/%s/%s|%s]]",
				month[:4], TimelineFilename(month, "month"), FormatPeriod(month)))
			if i == 0 && !data.LastMessage.IsZero() {
				sb.WriteString(" - Most recent")
			} else if i == len(data.RecentMonths)-1 && !data.FirstMessage.IsZero() {
				sb.WriteString(" - Project started")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Quick Links
	sb.WriteString("## Quick Links\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString(fmt.Sprintf("# View all messages with label: %s\n", data.LabelName))
	sb.WriteString(fmt.Sprintf("msgvault tui --filter-label \"%s\"\n", data.LabelName))
	sb.WriteString("```\n\n")

	// MOCs
	sb.WriteString("## MOCs\n\n")
	sb.WriteString("- [[MOC - Projects]]\n")
	sb.WriteString("- [[MOC - Email Archive]]\n")

	// Write file
	filename := ProjectFilename(data.LabelName)
	path := filepath.Join(g.outputDir, "Projects", filename)
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
