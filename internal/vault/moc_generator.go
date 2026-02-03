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

// MOCGenerator generates Maps of Content (MOCs).
type MOCGenerator struct {
	store     *store.Store
	engine    query.Engine
	outputDir string
	logger    *slog.Logger
}

// NewMOCGenerator creates a new MOC generator.
func NewMOCGenerator(s *store.Store, engine query.Engine, outputDir string, logger *slog.Logger) *MOCGenerator {
	return &MOCGenerator{
		store:     s,
		engine:    engine,
		outputDir: outputDir,
		logger:    logger,
	}
}

// Generate generates all MOCs.
func (g *MOCGenerator) Generate(ctx context.Context, opts ExportOptions) error {
	if opts.DryRun {
		return nil
	}

	// Generate master MOC
	if err := g.generateMasterMOC(ctx); err != nil {
		return fmt.Errorf("failed to generate master MOC: %w", err)
	}

	// Generate Top Contacts MOC
	if err := g.generateTopContactsMOC(ctx); err != nil {
		return fmt.Errorf("failed to generate top contacts MOC: %w", err)
	}

	// Generate Projects MOC
	if err := g.generateProjectsMOC(ctx); err != nil {
		return fmt.Errorf("failed to generate projects MOC: %w", err)
	}

	return nil
}

// generateMasterMOC generates the master Email Archive MOC.
func (g *MOCGenerator) generateMasterMOC(ctx context.Context) error {
	// Get overall statistics
	var totalMessages int
	var totalSize int64
	var firstMessage, lastMessage interface{}
	var totalPeople, totalProjects int

	err := g.store.DB().QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total_messages,
			SUM(COALESCE(size_estimate, 0)) as total_size,
			MIN(sent_at) as first_message,
			MAX(sent_at) as last_message
		FROM messages
		WHERE deleted_at IS NULL
	`).Scan(&totalMessages, &totalSize, &firstMessage, &lastMessage)
	if err != nil {
		return fmt.Errorf("failed to query message stats: %w", err)
	}

	err = g.store.DB().QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT p.id)
		FROM participants p
		JOIN message_recipients mr ON mr.participant_id = p.id
		JOIN messages m ON m.id = mr.message_id
		WHERE m.deleted_at IS NULL
	`).Scan(&totalPeople)
	if err != nil {
		return fmt.Errorf("failed to query people count: %w", err)
	}

	err = g.store.DB().QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT l.id)
		FROM labels l
		JOIN message_labels ml ON ml.label_id = l.id
		JOIN messages m ON m.id = ml.message_id
		WHERE m.deleted_at IS NULL
	`).Scan(&totalProjects)
	if err != nil {
		return fmt.Errorf("failed to query project count: %w", err)
	}

	// Get account count
	var accountCount int
	err = g.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sources`).Scan(&accountCount)
	if err != nil {
		return fmt.Errorf("failed to query account count: %w", err)
	}

	// Build MOC content
	var sb strings.Builder

	// Frontmatter
	frontmatter := map[string]interface{}{
		"created": time.Now().Format(time.RFC3339),
		"tags":    []string{"moc", "mapOfContent", "emailArchive"},
	}
	sb.WriteString(GenerateFrontmatter(frontmatter))

	// Title and intro
	sb.WriteString("# MOC - Email Archive\n\n")
	sb.WriteString("Master index for email knowledge vault. Generated from msgvault archive.\n\n")

	// Statistics
	sb.WriteString("## Statistics\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Messages:** %s\n", FormatMessageCount(totalMessages)))

	// Parse timestamps (handles both SQLite and DuckDB)
	firstTime, firstOk := ParseNullableTimestamp(firstMessage)
	lastTime, lastOk := ParseNullableTimestamp(lastMessage)

	if firstOk && lastOk {
		years := lastTime.Sub(firstTime).Hours() / 24 / 365
		sb.WriteString(fmt.Sprintf("- **Date Range:** %s - %s (%.0f years)\n",
			FormatDate(firstTime, "January 2006"),
			FormatDate(lastTime, "January 2006"),
			years))
	}
	if accountCount > 0 {
		sb.WriteString(fmt.Sprintf("- **Accounts:** %d Gmail accounts\n", accountCount))
	}
	sb.WriteString(fmt.Sprintf("- **Unique Contacts:** %s\n", FormatMessageCount(totalPeople)))
	sb.WriteString(fmt.Sprintf("- **Projects/Labels:** %d\n", totalProjects))
	sb.WriteString(fmt.Sprintf("- **Total Size:** %s\n", FormatSize(totalSize)))
	sb.WriteString("\n")

	// Quick Access
	sb.WriteString("## Quick Access\n\n")
	sb.WriteString("- [[MOC - Top Contacts]] - Your most frequent correspondents\n")
	sb.WriteString("- [[MOC - Projects]] - All projects and labels\n")
	if lastOk {
		currentPeriod := lastTime.Format("2006-01")
		sb.WriteString(fmt.Sprintf("- [[Timeline/%s/%s|%s]] - Current month\n",
			currentPeriod[:4], TimelineFilename(currentPeriod, "month"), FormatPeriod(currentPeriod)))
	}
	sb.WriteString("\n")

	// People section
	sb.WriteString("## People\n\n")
	sb.WriteString("See [[MOC - Top Contacts]] for your most frequent correspondents.\n\n")
	sb.WriteString("All person notes are in the `People/` folder.\n\n")

	// Projects section
	sb.WriteString("## Projects\n\n")
	sb.WriteString("See [[MOC - Projects]] for all projects and labels.\n\n")
	sb.WriteString("All project notes are in the `Projects/` folder.\n\n")

	// Timeline section
	sb.WriteString("## Timeline\n\n")
	if lastOk {
		currentYear := lastTime.Format("2006")
		sb.WriteString(fmt.Sprintf("- [[Timeline/%s/|%s]] - Current year\n", currentYear, currentYear))

		// Add links to significant years
		if firstOk {
			firstYear := firstTime.Year()
			lastYear := lastTime.Year()

			// Show every 5th year for long archives
			for year := lastYear - 5; year >= firstYear; year -= 5 {
				sb.WriteString(fmt.Sprintf("- [[Timeline/%d/|%d]]\n", year, year))
			}
			if firstYear < lastYear-5 {
				sb.WriteString(fmt.Sprintf("- [[Timeline/%d/|%d]] - Archive start\n", firstYear, firstYear))
			}
		}
	}
	sb.WriteString("\n")

	// Usage section
	sb.WriteString("## Usage\n\n")
	sb.WriteString("This vault is an index-only export. Email bodies and attachments remain in msgvault.\n\n")
	sb.WriteString("To view full messages:\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("msgvault tui\n")
	sb.WriteString("```\n")

	// Write file
	path := filepath.Join(g.outputDir, "MOC - Email Archive.md")
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// generateTopContactsMOC generates the Top Contacts MOC.
func (g *MOCGenerator) generateTopContactsMOC(ctx context.Context) error {
	// Get top 50 contacts
	rows, err := g.store.DB().QueryContext(ctx, `
		SELECT
			p.email_address,
			p.display_name,
			COUNT(DISTINCT m.id) as message_count,
			MAX(m.sent_at) as last_contact
		FROM participants p
		JOIN message_recipients mr ON mr.participant_id = p.id
		JOIN messages m ON m.id = mr.message_id
		WHERE m.deleted_at IS NULL
		GROUP BY p.id
		ORDER BY message_count DESC
		LIMIT 50
	`)
	if err != nil {
		return fmt.Errorf("failed to query top contacts: %w", err)
	}
	defer rows.Close()

	type contact struct {
		email       string
		displayName string
		count       int
		lastContact time.Time
		hasContact  bool
	}

	var contacts []contact
	for rows.Next() {
		var c contact
		var lastContact interface{}
		if err := rows.Scan(&c.email, &c.displayName, &c.count, &lastContact); err != nil {
			continue
		}
		if t, ok := ParseNullableTimestamp(lastContact); ok {
			c.lastContact = t
			c.hasContact = true
		}
		contacts = append(contacts, c)
	}

	// Build MOC content
	var sb strings.Builder

	// Frontmatter
	frontmatter := map[string]interface{}{
		"created": time.Now().Format(time.RFC3339),
		"tags":    []string{"moc", "mapOfContent", "contacts"},
	}
	sb.WriteString(GenerateFrontmatter(frontmatter))

	// Title
	sb.WriteString("# MOC - Top Contacts\n\n")
	sb.WriteString(fmt.Sprintf("Your top %d most frequent email correspondents.\n\n", len(contacts)))

	// Contacts list
	sb.WriteString("## Top Correspondents\n\n")
	for i, c := range contacts {
		displayName := SafeDisplayName(c.displayName, c.email)
		sb.WriteString(fmt.Sprintf("%d. [[People/%s|%s]] - %s",
			i+1, PersonFilename(c.email), displayName, FormatMessageCount(c.count)))
		if c.hasContact {
			sb.WriteString(fmt.Sprintf(" (last: %s)", FormatDate(c.lastContact, "Jan 2006")))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Back link
	sb.WriteString("## MOCs\n\n")
	sb.WriteString("- [[MOC - Email Archive]]\n")

	// Write file
	path := filepath.Join(g.outputDir, "MOC - Top Contacts.md")
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// generateProjectsMOC generates the Projects MOC.
func (g *MOCGenerator) generateProjectsMOC(ctx context.Context) error {
	// Get all projects/labels
	rows, err := g.store.DB().QueryContext(ctx, `
		SELECT
			l.name,
			l.label_type,
			COUNT(DISTINCT m.id) as message_count,
			MAX(m.sent_at) as last_message
		FROM labels l
		JOIN message_labels ml ON ml.label_id = l.id
		JOIN messages m ON m.id = ml.message_id
		WHERE m.deleted_at IS NULL
		GROUP BY l.id
		ORDER BY message_count DESC
	`)
	if err != nil {
		return fmt.Errorf("failed to query projects: %w", err)
	}
	defer rows.Close()

	type project struct {
		name        string
		labelType   string
		count       int
		lastMessage time.Time
		hasMessage  bool
	}

	var projects []project
	for rows.Next() {
		var p project
		var lastMessage interface{}
		if err := rows.Scan(&p.name, &p.labelType, &p.count, &lastMessage); err != nil {
			continue
		}
		if t, ok := ParseNullableTimestamp(lastMessage); ok {
			p.lastMessage = t
			p.hasMessage = true
		}
		projects = append(projects, p)
	}

	// Build MOC content
	var sb strings.Builder

	// Frontmatter
	frontmatter := map[string]interface{}{
		"created": time.Now().Format(time.RFC3339),
		"tags":    []string{"moc", "mapOfContent", "projects"},
	}
	sb.WriteString(GenerateFrontmatter(frontmatter))

	// Title
	sb.WriteString("# MOC - Projects\n\n")
	sb.WriteString(fmt.Sprintf("All projects and labels (%d total).\n\n", len(projects)))

	// Group by type if we have that info
	systemLabels := []project{}
	userLabels := []project{}
	for _, p := range projects {
		if p.labelType == "system" {
			systemLabels = append(systemLabels, p)
		} else {
			userLabels = append(userLabels, p)
		}
	}

	// User Labels
	if len(userLabels) > 0 {
		sb.WriteString("## User Labels\n\n")
		for _, p := range userLabels {
			sb.WriteString(fmt.Sprintf("- [[Projects/%s|%s]] - %s",
				ProjectFilename(p.name), p.name, FormatMessageCount(p.count)))
			if p.hasMessage {
				sb.WriteString(fmt.Sprintf(" (last: %s)", FormatDate(p.lastMessage, "Jan 2006")))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// System Labels
	if len(systemLabels) > 0 {
		sb.WriteString("## System Labels\n\n")
		for _, p := range systemLabels {
			sb.WriteString(fmt.Sprintf("- [[Projects/%s|%s]] - %s",
				ProjectFilename(p.name), p.name, FormatMessageCount(p.count)))
			if p.hasMessage {
				sb.WriteString(fmt.Sprintf(" (last: %s)", FormatDate(p.lastMessage, "Jan 2006")))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Back link
	sb.WriteString("## MOCs\n\n")
	sb.WriteString("- [[MOC - Email Archive]]\n")

	// Write file
	path := filepath.Join(g.outputDir, "MOC - Projects.md")
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
