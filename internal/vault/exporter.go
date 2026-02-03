package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ryanstern/msgvault/internal/query"
	"github.com/ryanstern/msgvault/internal/store"
)

// ExportState tracks the state of vault exports for incremental updates.
type ExportState struct {
	LastExportAt      time.Time `json:"last_export_at"`
	LastMessageID     int64     `json:"last_message_id"`
	PeopleGenerated   int       `json:"people_count"`
	ProjectsGenerated int       `json:"projects_count"`
	TimelineMonths    int       `json:"timeline_months"`
	VaultPath         string    `json:"vault_path"`
	FullRebuild       bool      `json:"full_rebuild"`
}

// ExportOptions configures the vault export operation.
type ExportOptions struct {
	FullRebuild bool
	Limit       int
	After       time.Time
	Before      time.Time
	Account     string
	DryRun      bool
}

// VaultExporter exports msgvault data to an Obsidian vault.
type VaultExporter struct {
	store     *store.Store
	engine    query.Engine
	outputDir string
	state     *ExportState
	logger    *slog.Logger
}

// NewVaultExporter creates a new vault exporter.
func NewVaultExporter(s *store.Store, engine query.Engine, outputDir string, logger *slog.Logger) *VaultExporter {
	if logger == nil {
		logger = slog.Default()
	}
	return &VaultExporter{
		store:     s,
		engine:    engine,
		outputDir: outputDir,
		state:     &ExportState{},
		logger:    logger,
	}
}

// Export performs the vault export operation.
func (e *VaultExporter) Export(ctx context.Context, opts ExportOptions) error {
	e.logger.Info("starting vault export",
		"output_dir", e.outputDir,
		"full_rebuild", opts.FullRebuild,
		"dry_run", opts.DryRun)

	// Load existing state
	if !opts.FullRebuild {
		if err := e.LoadState(); err != nil {
			e.logger.Warn("could not load state, performing full rebuild", "error", err)
			opts.FullRebuild = true
		}
	}

	// Create directory structure
	if !opts.DryRun {
		if err := e.createDirectoryStructure(); err != nil {
			return fmt.Errorf("failed to create directory structure: %w", err)
		}
	}

	// Export in phases
	e.logger.Info("exporting people notes")
	peopleCount, err := e.exportPeople(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to export people: %w", err)
	}
	e.logger.Info("people notes exported", "count", peopleCount)

	e.logger.Info("exporting project/label notes")
	projectsCount, err := e.exportProjects(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to export projects: %w", err)
	}
	e.logger.Info("project notes exported", "count", projectsCount)

	e.logger.Info("exporting timeline notes")
	timelineCount, err := e.exportTimeline(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to export timeline: %w", err)
	}
	e.logger.Info("timeline notes exported", "count", timelineCount)

	e.logger.Info("generating MOCs")
	if err := e.generateMOCs(ctx, opts); err != nil {
		return fmt.Errorf("failed to generate MOCs: %w", err)
	}

	e.logger.Info("generating README")
	if err := e.generateREADME(ctx, opts); err != nil {
		return fmt.Errorf("failed to generate README: %w", err)
	}

	// Update and save state
	if !opts.DryRun {
		e.state.LastExportAt = time.Now()
		e.state.PeopleGenerated = peopleCount
		e.state.ProjectsGenerated = projectsCount
		e.state.TimelineMonths = timelineCount
		e.state.VaultPath = e.outputDir
		e.state.FullRebuild = opts.FullRebuild

		// Get last message ID
		lastID, err := e.getLastMessageID(ctx)
		if err != nil {
			e.logger.Warn("could not get last message ID", "error", err)
		} else {
			e.state.LastMessageID = lastID
		}

		if err := e.SaveState(); err != nil {
			e.logger.Warn("could not save state", "error", err)
		}
	}

	e.logger.Info("vault export complete",
		"people", peopleCount,
		"projects", projectsCount,
		"timeline_months", timelineCount)

	return nil
}

// createDirectoryStructure creates the vault directory structure.
func (e *VaultExporter) createDirectoryStructure() error {
	dirs := []string{
		e.outputDir,
		filepath.Join(e.outputDir, "People"),
		filepath.Join(e.outputDir, "Projects"),
		filepath.Join(e.outputDir, "Timeline"),
		filepath.Join(e.outputDir, "Domains"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// exportPeople exports person notes.
func (e *VaultExporter) exportPeople(ctx context.Context, opts ExportOptions) (int, error) {
	generator := NewPersonNoteGenerator(e.store, e.engine, e.outputDir, e.logger)
	return generator.Generate(ctx, opts)
}

// exportProjects exports project/label notes.
func (e *VaultExporter) exportProjects(ctx context.Context, opts ExportOptions) (int, error) {
	generator := NewProjectNoteGenerator(e.store, e.engine, e.outputDir, e.logger)
	return generator.Generate(ctx, opts)
}

// exportTimeline exports timeline notes.
func (e *VaultExporter) exportTimeline(ctx context.Context, opts ExportOptions) (int, error) {
	generator := NewTimelineNoteGenerator(e.store, e.engine, e.outputDir, e.logger)
	return generator.Generate(ctx, opts)
}

// generateMOCs generates Maps of Content.
func (e *VaultExporter) generateMOCs(ctx context.Context, opts ExportOptions) error {
	generator := NewMOCGenerator(e.store, e.engine, e.outputDir, e.logger)
	return generator.Generate(ctx, opts)
}

// generateREADME generates the vault README.
func (e *VaultExporter) generateREADME(ctx context.Context, opts ExportOptions) error {
	if opts.DryRun {
		return nil
	}

	readme := `# msgvault Email Archive Vault

This Obsidian vault is automatically generated from your msgvault email archive.

## Structure

- **People/** - Notes for each email contact
- **Projects/** - Notes for each Gmail label/project
- **Timeline/** - Time-based organization (year/month)
- **Domains/** - Notes for organizations/domains

## Quick Links

- [[MOC - Email Archive]] - Master index
- [[MOC - Top Contacts]] - Your most frequent correspondents
- [[MOC - Projects]] - All projects and labels

## Usage

This vault is an index-only export. Email bodies and attachments remain in msgvault.

To view full messages, use the msgvault TUI:

` + "```bash" + `
msgvault tui
msgvault tui --account you@gmail.com
` + "```" + `

## Updating

To update the vault with new emails:

` + "```bash" + `
msgvault sync-incremental you@gmail.com
msgvault export-vault
` + "```" + `

To rebuild the vault from scratch:

` + "```bash" + `
msgvault export-vault --full-rebuild
` + "```" + `

---

Generated by msgvault on ` + time.Now().Format("2006-01-02 15:04:05") + `
`

	path := filepath.Join(e.outputDir, "README.md")
	return os.WriteFile(path, []byte(readme), 0644)
}

// getLastMessageID returns the highest message ID in the database.
func (e *VaultExporter) getLastMessageID(ctx context.Context) (int64, error) {
	var lastID int64
	err := e.store.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0) FROM messages`).Scan(&lastID)
	return lastID, err
}

// LoadState loads the export state from disk.
func (e *VaultExporter) LoadState() error {
	statePath := filepath.Join(e.outputDir, ".msgvault-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("state file does not exist")
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	if err := json.Unmarshal(data, e.state); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	return nil
}

// SaveState saves the export state to disk.
func (e *VaultExporter) SaveState() error {
	statePath := filepath.Join(e.outputDir, ".msgvault-state.json")
	data, err := json.MarshalIndent(e.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}
