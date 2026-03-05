package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
)

var (
	stageDomain  string
	stageSender  string
	stageLabel   string
	stageAfter   string
	stageBefore  string
	stageAccount string
	stageDesc    string
)

var stageDeletionCmd = &cobra.Command{
	Use:   "stage-deletion",
	Short: "Stage messages for deletion by filter",
	Long: `Stage messages matching a filter for deletion.

Creates a deletion manifest in the pending directory. Use delete-staged to execute.

Examples:
  msgvault stage-deletion --domain shopittome.com --account you@gmail.com
  msgvault stage-deletion --sender news@example.com --account you@gmail.com
  msgvault stage-deletion --domain example.com --after 2020-01-01 --before 2023-01-01
  msgvault stage-deletion --label "CATEGORY_PROMOTIONS" --account you@gmail.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if stageDomain == "" && stageSender == "" && stageLabel == "" {
			return fmt.Errorf("at least one filter is required: --domain, --sender, or --label")
		}

		dbPath := cfg.DatabaseDSN()
		analyticsDir := cfg.AnalyticsDir()

		s, engine, cleanup, err := initQueryEngine(dbPath, analyticsDir, false, true)
		if err != nil {
			return err
		}
		defer cleanup()

		// Resolve account to source_id
		if stageAccount == "" {
			sources, err := s.ListSources("")
			if err != nil {
				return fmt.Errorf("list sources: %w", err)
			}
			if len(sources) == 1 {
				stageAccount = sources[0].Identifier
			} else {
				return fmt.Errorf("multiple accounts found, use --account to specify which one")
			}
		}

		source, err := s.GetSourceByIdentifier(stageAccount)
		if err != nil {
			return fmt.Errorf("account %q not found: %w", stageAccount, err)
		}

		// Build message filter
		filter := query.MessageFilter{
			Domain: stageDomain,
			Sender: stageSender,
			Label:  stageLabel,
			SourceID: &source.ID,
		}

		if stageAfter != "" {
			t, err := time.Parse("2006-01-02", stageAfter)
			if err != nil {
				return fmt.Errorf("invalid --after date: %w", err)
			}
			filter.After = &t
		}
		if stageBefore != "" {
			t, err := time.Parse("2006-01-02", stageBefore)
			if err != nil {
				return fmt.Errorf("invalid --before date: %w", err)
			}
			filter.Before = &t
		}

		// Resolve matching Gmail IDs
		ctx := context.Background()
		gmailIDs, err := engine.GetGmailIDsByFilter(ctx, filter)
		if err != nil {
			return fmt.Errorf("query messages: %w", err)
		}

		if len(gmailIDs) == 0 {
			fmt.Println("No messages match the filter.")
			return nil
		}

		// Build description
		description := stageDesc
		if description == "" {
			if stageDomain != "" {
				description = fmt.Sprintf("Domain: %s", stageDomain)
			} else if stageSender != "" {
				description = fmt.Sprintf("Sender: %s", stageSender)
			} else if stageLabel != "" {
				description = fmt.Sprintf("Label: %s", stageLabel)
			}
		}

		// Create manifest
		manifest := deletion.NewManifest(description, gmailIDs)
		manifest.CreatedBy = "cli"
		manifest.Filters = deletion.Filters{
			Account: stageAccount,
		}
		if stageDomain != "" {
			manifest.Filters.SenderDomains = []string{stageDomain}
		}
		if stageSender != "" {
			manifest.Filters.Senders = []string{stageSender}
		}
		if stageLabel != "" {
			manifest.Filters.Labels = []string{stageLabel}
		}
		if stageAfter != "" {
			manifest.Filters.After = stageAfter
		}
		if stageBefore != "" {
			manifest.Filters.Before = stageBefore
		}

		// Save manifest
		deletionsDir := filepath.Join(cfg.Data.DataDir, "deletions")
		manager, err := deletion.NewManager(deletionsDir)
		if err != nil {
			return fmt.Errorf("create deletion manager: %w", err)
		}

		if err := manager.SaveManifest(manifest); err != nil {
			return fmt.Errorf("save manifest: %w", err)
		}

		fmt.Printf("Staged %d messages for deletion\n", len(gmailIDs))
		fmt.Printf("  Domain:   %s\n", stageDomain)
		fmt.Printf("  Account:  %s\n", stageAccount)
		fmt.Printf("  Manifest: %s\n", manifest.ID)
		fmt.Printf("\nUse 'msgvault delete-staged --list' to review, or 'msgvault delete-staged' to execute.\n")

		return nil
	},
}

func init() {
	stageDeletionCmd.Flags().StringVar(&stageDomain, "domain", "", "Filter by sender domain")
	stageDeletionCmd.Flags().StringVar(&stageSender, "sender", "", "Filter by sender email")
	stageDeletionCmd.Flags().StringVar(&stageLabel, "label", "", "Filter by label")
	stageDeletionCmd.Flags().StringVar(&stageAfter, "after", "", "Only messages after this date (YYYY-MM-DD)")
	stageDeletionCmd.Flags().StringVar(&stageBefore, "before", "", "Only messages before this date (YYYY-MM-DD)")
	stageDeletionCmd.Flags().StringVar(&stageAccount, "account", "", "Gmail account email")
	stageDeletionCmd.Flags().StringVar(&stageDesc, "description", "", "Manifest description (auto-generated if omitted)")

	rootCmd.AddCommand(stageDeletionCmd)
}
