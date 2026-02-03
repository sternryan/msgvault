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

// PersonNoteGenerator generates person notes from participants.
type PersonNoteGenerator struct {
	store       *store.Store
	engine      query.Engine
	outputDir   string
	logger      *slog.Logger
	bulkFetcher BulkDataFetcher
}

// NewPersonNoteGenerator creates a new person note generator.
func NewPersonNoteGenerator(s *store.Store, engine query.Engine, outputDir string, logger *slog.Logger, bulkFetcher BulkDataFetcher) *PersonNoteGenerator {
	return &PersonNoteGenerator{
		store:       s,
		engine:      engine,
		outputDir:   outputDir,
		logger:      logger,
		bulkFetcher: bulkFetcher,
	}
}

// PersonData holds data for a person note.
type PersonData struct {
	Email           string
	DisplayName     string
	Domain          string
	FirstContact    time.Time
	LastContact     time.Time
	MessageCount    int
	SentCount       int
	ReceivedCount   int
	TotalSize       int64
	AttachmentCount int
	TopLabels       []LabelStat
	RelatedPeople   []RelatedPerson
	RecentMonths    []string
}

// LabelStat represents label usage statistics.
type LabelStat struct {
	Name  string
	Count int
}

// RelatedPerson represents a person frequently in same threads.
type RelatedPerson struct {
	Email         string
	SharedThreads int
}

// Generate generates person notes.
func (g *PersonNoteGenerator) Generate(ctx context.Context, opts ExportOptions) (int, error) {
	// Pre-fetch all bulk data before the main loop (eliminates N+1 queries)
	g.logger.Debug("pre-fetching bulk data for all people")
	topLabelsMap, err := g.bulkFetcher.FetchAllTopLabelsByPerson(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch top labels: %w", err)
	}

	relatedPeopleMap, err := g.bulkFetcher.FetchAllRelatedPeople(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch related people: %w", err)
	}

	recentMonthsMap, err := g.bulkFetcher.FetchAllRecentMonthsByPerson(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch recent months: %w", err)
	}
	g.logger.Debug("bulk data pre-fetch complete")

	// Query all participants with aggregated stats
	query := `
		SELECT
			p.email_address,
			COALESCE(p.display_name, '') as display_name,
			COALESCE(p.domain, '') as domain,
			COUNT(DISTINCT m.id) as message_count,
			SUM(COALESCE(m.size_estimate, 0)) as total_size,
			COUNT(DISTINCT CASE WHEN m.has_attachments THEN m.id END) as attachments_count,
			MIN(m.sent_at) as first_contact,
			MAX(m.sent_at) as last_contact,
			COUNT(DISTINCT CASE WHEN mr.recipient_type = 'from' THEN m.id END) as received_count
		FROM participants p
		JOIN message_recipients mr ON mr.participant_id = p.id
		JOIN messages m ON m.id = mr.message_id
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
		GROUP BY p.id
		ORDER BY message_count DESC
	`

	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit)
	}

	rows, err := g.store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to query participants: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var data PersonData
		var firstContact, lastContact interface{}

		err := rows.Scan(
			&data.Email,
			&data.DisplayName,
			&data.Domain,
			&data.MessageCount,
			&data.TotalSize,
			&data.AttachmentCount,
			&firstContact,
			&lastContact,
			&data.ReceivedCount,
		)
		if err != nil {
			g.logger.Warn("failed to scan person row", "error", err)
			continue
		}

		// Parse timestamps (handles both SQLite sql.NullTime and DuckDB strings)
		if t, ok := ParseNullableTimestamp(firstContact); ok {
			data.FirstContact = t
		}
		if t, ok := ParseNullableTimestamp(lastContact); ok {
			data.LastContact = t
		}
		data.SentCount = data.MessageCount - data.ReceivedCount

		// Lookup bulk data from pre-fetched maps (O(1) instead of N queries)
		data.TopLabels = topLabelsMap[data.Email]
		data.RelatedPeople = relatedPeopleMap[data.Email]
		data.RecentMonths = recentMonthsMap[data.Email]

		// Generate note
		if !opts.DryRun {
			if err := g.generateNote(data); err != nil {
				g.logger.Warn("failed to generate person note", "email", data.Email, "error", err)
				continue
			}
		}

		count++
		if count%100 == 0 {
			g.logger.Debug("progress", "people_generated", count)
		}
	}

	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("error iterating rows: %w", err)
	}

	return count, nil
}

// generateNote generates a person note file.
func (g *PersonNoteGenerator) generateNote(data PersonData) error {
	// Generate frontmatter
	frontmatter := map[string]interface{}{
		"created": time.Now().Format(time.RFC3339),
		"tags":    []string{"person", "contact"},
		"email":   data.Email,
	}

	if data.Domain != "" {
		frontmatter["domain"] = data.Domain
		frontmatter["tags"] = append(frontmatter["tags"].([]string), "domain/"+data.Domain)
	}

	if !data.FirstContact.IsZero() {
		frontmatter["first_message"] = data.FirstContact.Format("2006-01-02")
	}
	if !data.LastContact.IsZero() {
		frontmatter["last_message"] = data.LastContact.Format("2006-01-02")
	}

	frontmatter["message_count"] = data.MessageCount
	frontmatter["total_size"] = FormatSize(data.TotalSize)

	// Build note content
	var sb strings.Builder

	// Frontmatter
	sb.WriteString(GenerateFrontmatter(frontmatter))

	// Title
	displayName := SafeDisplayName(data.DisplayName, data.Email)
	if displayName != data.Email {
		sb.WriteString(fmt.Sprintf("# %s (%s)\n\n", displayName, data.Email))
	} else {
		sb.WriteString(fmt.Sprintf("# %s\n\n", data.Email))
	}

	// Overview
	sb.WriteString("## Overview\n\n")
	if data.Domain != "" {
		sb.WriteString(fmt.Sprintf("- **Domain:** [[Domains/%s|%s]]\n", SanitizeFilename(data.Domain), data.Domain))
	}
	sb.WriteString(fmt.Sprintf("- **Email:** %s\n", data.Email))
	if !data.FirstContact.IsZero() {
		sb.WriteString(fmt.Sprintf("- **First Contact:** %s\n", FormatDate(data.FirstContact, "January 2, 2006")))
	}
	if !data.LastContact.IsZero() {
		sb.WriteString(fmt.Sprintf("- **Last Contact:** %s\n", FormatDate(data.LastContact, "January 2, 2006")))
	}
	sb.WriteString(fmt.Sprintf("- **Total Messages:** %s (%d received, %d sent)\n",
		FormatMessageCount(data.MessageCount), data.ReceivedCount, data.SentCount))
	sb.WriteString(fmt.Sprintf("- **Total Size:** %s\n", FormatSize(data.TotalSize)))
	if data.AttachmentCount > 0 {
		sb.WriteString(fmt.Sprintf("- **Attachments:** %d files\n", data.AttachmentCount))
	}
	sb.WriteString("\n")

	// Top Labels
	if len(data.TopLabels) > 0 {
		sb.WriteString("## Top Labels\n\n")
		for _, label := range data.TopLabels {
			sb.WriteString(fmt.Sprintf("- [[Projects/%s|%s]] (%s)\n",
				ProjectFilename(label.Name), label.Name, FormatMessageCount(label.Count)))
		}
		sb.WriteString("\n")
	}

	// Related People
	if len(data.RelatedPeople) > 0 {
		sb.WriteString("## Related People\n\n")
		for _, person := range data.RelatedPeople {
			sb.WriteString(fmt.Sprintf("- [[People/%s|%s]] (%d shared threads)\n",
				PersonFilename(person.Email), person.Email, person.SharedThreads))
		}
		sb.WriteString("\n")
	}

	// Recent Activity
	if len(data.RecentMonths) > 0 {
		sb.WriteString("## Recent Activity\n\n")
		for _, month := range data.RecentMonths {
			sb.WriteString(fmt.Sprintf("- [[Timeline/%s/%s|%s]]\n",
				month[:4], TimelineFilename(month, "month"), FormatPeriod(month)))
		}
		sb.WriteString("\n")
	}

	// Quick Links
	sb.WriteString("## Quick Links\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString(fmt.Sprintf("# View all messages from %s\n", data.Email))
	sb.WriteString(fmt.Sprintf("msgvault tui --filter-sender %s\n", data.Email))
	sb.WriteString("```\n\n")

	// MOCs
	sb.WriteString("## MOCs\n\n")
	sb.WriteString("- [[MOC - Top Contacts]]\n")
	sb.WriteString("- [[MOC - Email Archive]]\n")

	// Write file
	filename := PersonFilename(data.Email)
	path := filepath.Join(g.outputDir, "People", filename)
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
