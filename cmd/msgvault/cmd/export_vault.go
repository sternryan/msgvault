package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/config"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/store"
	"github.com/wesm/msgvault/internal/vault"
)

var exportVaultCmd = &cobra.Command{
	Use:   "export-vault",
	Short: "Export msgvault archive to Obsidian vault",
	Long: `Export msgvault archive to an Obsidian vault with person notes, project notes,
timeline views, and Maps of Content (MOCs).

The vault contains metadata, insights, and links back to msgvault - email bodies
and attachments remain in msgvault for efficient storage.

Example usage:
  msgvault export-vault
  msgvault export-vault --output ~/Documents/msgvault-vault
  msgvault export-vault --after 2024-01-01 --limit 1000
  msgvault export-vault --full-rebuild`,
	RunE: runExportVault,
}

var (
	exportVaultOutputDir   string
	exportVaultFullRebuild bool
	exportVaultLimit       int
	exportVaultAfter       string
	exportVaultBefore      string
	exportVaultAccount     string
	exportVaultDryRun      bool
)

func init() {
	rootCmd.AddCommand(exportVaultCmd)

	exportVaultCmd.Flags().StringVar(&exportVaultOutputDir, "output", "",
		"Output directory for vault (default: ~/Documents/msgvault-vault)")
	exportVaultCmd.Flags().BoolVar(&exportVaultFullRebuild, "full-rebuild", false,
		"Rebuild entire vault from scratch")
	exportVaultCmd.Flags().IntVar(&exportVaultLimit, "limit", 0,
		"Limit export to N most active entities (for testing)")
	exportVaultCmd.Flags().StringVar(&exportVaultAfter, "after", "",
		"Export messages after date (YYYY-MM-DD)")
	exportVaultCmd.Flags().StringVar(&exportVaultBefore, "before", "",
		"Export messages before date (YYYY-MM-DD)")
	exportVaultCmd.Flags().StringVar(&exportVaultAccount, "account", "",
		"Export specific account only")
	exportVaultCmd.Flags().BoolVar(&exportVaultDryRun, "dry-run", false,
		"Show what would be exported without writing files")
}

func runExportVault(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load config
	cfg, err := config.Load("", "")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set default output directory
	outputDir := exportVaultOutputDir
	if outputDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		outputDir = filepath.Join(homeDir, "Documents", "msgvault-vault")
	}

	// Expand home directory if needed
	if outputDir[:2] == "~/" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		outputDir = filepath.Join(homeDir, outputDir[2:])
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("exporting vault",
		"output_dir", outputDir,
		"full_rebuild", exportVaultFullRebuild,
		"dry_run", exportVaultDryRun)

	// Open database
	s, err := store.Open(cfg.DatabaseDSN(), store.WithPassphrase(passphrase))
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer s.Close()

	// Create query engine
	// Try DuckDB first (for Parquet support), fall back to SQLite
	var engine query.Engine
	duckEngine, err := query.NewDuckDBEngine(cfg.AnalyticsDir(), cfg.DatabaseDSN(), s.DB())
	if err != nil {
		logger.Warn("could not create DuckDB engine, falling back to SQLite", "error", err)
		engine = query.NewSQLiteEngine(s.DB())
	} else {
		engine = duckEngine
	}

	// Parse date filters
	opts := vault.ExportOptions{
		FullRebuild: exportVaultFullRebuild,
		Limit:       exportVaultLimit,
		Account:     exportVaultAccount,
		DryRun:      exportVaultDryRun,
	}

	if exportVaultAfter != "" {
		opts.After, err = time.Parse("2006-01-02", exportVaultAfter)
		if err != nil {
			return fmt.Errorf("invalid --after date (use YYYY-MM-DD): %w", err)
		}
	}

	if exportVaultBefore != "" {
		opts.Before, err = time.Parse("2006-01-02", exportVaultBefore)
		if err != nil {
			return fmt.Errorf("invalid --before date (use YYYY-MM-DD): %w", err)
		}
	}

	// Create exporter
	exporter := vault.NewVaultExporter(s, engine, outputDir, logger)

	// Run export
	startTime := time.Now()
	if err := exporter.Export(ctx, opts); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}
	duration := time.Since(startTime)

	logger.Info("export complete",
		"duration", duration.Round(time.Millisecond),
		"output_dir", outputDir)

	if exportVaultDryRun {
		fmt.Println("\nDry run complete. No files were written.")
	} else {
		fmt.Printf("\nVault exported to: %s\n", outputDir)
		fmt.Println("\nOpen in Obsidian:")
		fmt.Printf("  1. Open Obsidian\n")
		fmt.Printf("  2. Click 'Open folder as vault'\n")
		fmt.Printf("  3. Select: %s\n", outputDir)
		fmt.Println("\nTo update vault after syncing new emails:")
		fmt.Println("  msgvault sync-incremental <account>")
		fmt.Println("  msgvault export-vault")
	}

	return nil
}
