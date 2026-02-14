package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/backup"
)

var (
	restoreVerifyOnly bool
	restoreForce      bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore <backup-path>",
	Short: "Restore from a backup",
	Long: `Restore a msgvault archive from a backup directory or .tar.gz file.

The backup is verified before restoring:
  - Database checksum (SHA-256) is verified against the manifest
  - SQLite integrity check is run on the backed-up database

Examples:
  msgvault restore ~/.msgvault/backups/backup-20240101-120000
  msgvault restore /mnt/external/backup.tar.gz
  msgvault restore backup-path --verify-only
  msgvault restore backup-path --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backupPath := args[0]

		opts := &backup.RestoreOptions{
			VerifyOnly: restoreVerifyOnly,
			Force:      restoreForce,
		}

		if opts.VerifyOnly {
			fmt.Println("Verifying backup...")
		} else {
			fmt.Println("Restoring from backup...")
		}

		if err := backup.Restore(cmd.Context(), backupPath, cfg, opts); err != nil {
			return fmt.Errorf("restore: %w", err)
		}

		if opts.VerifyOnly {
			fmt.Println("Backup verification passed.")
		} else {
			fmt.Printf("Restore complete. Data restored to: %s\n", cfg.Data.DataDir)
		}

		return nil
	},
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreVerifyOnly, "verify-only", false, "verify backup integrity without restoring")
	restoreCmd.Flags().BoolVar(&restoreForce, "force", false, "overwrite existing data")
	rootCmd.AddCommand(restoreCmd)
}
