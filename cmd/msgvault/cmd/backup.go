package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/backup"
	"github.com/wesm/msgvault/internal/store"
)

var (
	backupOutput      string
	backupTar         bool
	backupIncremental bool
	backupNoTokens    bool
	backupNoDeletions bool
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a backup of the msgvault archive",
	Long: `Create a backup of the entire msgvault archive including the database,
OAuth tokens, deletion manifests, and attachments.

Uses SQLite's online backup API for an atomic, consistent database snapshot.

Examples:
  msgvault backup
  msgvault backup --output /mnt/external/backup
  msgvault backup --tar
  msgvault backup --incremental`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()

		s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		opts := backup.DefaultBackupOptions()
		opts.OutputDir = backupOutput
		opts.Tar = backupTar
		opts.Incremental = backupIncremental
		opts.IncludeTokens = !backupNoTokens
		opts.IncludeDeletions = !backupNoDeletions

		fmt.Println("Creating backup...")

		path, err := backup.Backup(cmd.Context(), s, cfg, opts)
		if err != nil {
			return fmt.Errorf("backup: %w", err)
		}

		// Load manifest to show summary
		if !opts.Tar {
			manifest, err := backup.LoadManifest(path)
			if err == nil {
				fmt.Printf("  Messages:  %d\n", manifest.MessageCount)
				fmt.Printf("  DB size:   %.2f MB\n", float64(manifest.DatabaseSize)/(1024*1024))
				fmt.Printf("  Checksum:  %s\n", manifest.DatabaseHash[:16]+"...")
			}
		}

		fmt.Printf("\nBackup saved to: %s\n", path)
		return nil
	},
}

func init() {
	backupCmd.Flags().StringVar(&backupOutput, "output", "", "output directory (default: ~/.msgvault/backups/backup-{timestamp})")
	backupCmd.Flags().BoolVar(&backupTar, "tar", false, "create .tar.gz archive")
	backupCmd.Flags().BoolVar(&backupIncremental, "incremental", false, "only copy new attachments")
	backupCmd.Flags().BoolVar(&backupNoTokens, "no-tokens", false, "exclude OAuth tokens")
	backupCmd.Flags().BoolVar(&backupNoDeletions, "no-deletions", false, "exclude deletion manifests")
	rootCmd.AddCommand(backupCmd)
}
