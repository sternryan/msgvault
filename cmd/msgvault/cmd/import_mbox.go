package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/mbox"
	"github.com/wesm/msgvault/internal/store"
	"github.com/wesm/msgvault/internal/sync"
)

var (
	mboxSourceName string
	mboxBefore     string
	mboxAfter      string
	mboxLimit      int
	mboxNoResume   bool
)

var importMboxCmd = &cobra.Command{
	Use:   "import-mbox <path>...",
	Short: "Import messages from MBOX files",
	Long: `Import messages from MBOX files (e.g., Google Takeout exports).

Accepts one or more MBOX file paths, or a directory to recursively find .mbox files.

Date filters:
  --after 2024-01-01     Only messages on or after this date
  --before 2024-12-31    Only messages before this date

Examples:
  msgvault import-mbox ~/Takeout/Mail/*.mbox
  msgvault import-mbox ~/Takeout/Mail/
  msgvault import-mbox export.mbox --source-name you@gmail.com
  msgvault import-mbox export.mbox --after 2024-01-01 --limit 100`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Open msgvault database
		dbPath := cfg.DatabaseDSN()
		s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		if err := s.InitSchema(); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}

		// Resolve MBOX file paths (expand directories recursively)
		mboxPaths, err := resolveMboxPaths(args)
		if err != nil {
			return err
		}
		if len(mboxPaths) == 0 {
			return fmt.Errorf("no .mbox files found in the given paths")
		}

		fmt.Printf("Found %d MBOX file(s)\n", len(mboxPaths))
		for _, p := range mboxPaths {
			fmt.Printf("  %s\n", p)
		}

		// Build client options from flags
		var clientOpts []mbox.ClientOption
		clientOpts = append(clientOpts, mbox.WithMboxLogger(logger))

		if mboxAfter != "" {
			t, err := time.Parse("2006-01-02", mboxAfter)
			if err != nil {
				return fmt.Errorf("invalid --after date: %w (use YYYY-MM-DD format)", err)
			}
			clientOpts = append(clientOpts, mbox.WithMboxAfterDate(t))
		}

		if mboxBefore != "" {
			t, err := time.Parse("2006-01-02", mboxBefore)
			if err != nil {
				return fmt.Errorf("invalid --before date: %w (use YYYY-MM-DD format)", err)
			}
			clientOpts = append(clientOpts, mbox.WithMboxBeforeDate(t))
		}

		if mboxLimit > 0 {
			clientOpts = append(clientOpts, mbox.WithMboxLimit(mboxLimit))
		}

		// Determine source name
		sourceName := mboxSourceName
		if sourceName == "" {
			sourceName = "mbox-import"
		}

		// Create MBOX client
		mboxClient, err := mbox.NewClient(mboxPaths, sourceName, clientOpts...)
		if err != nil {
			return fmt.Errorf("open MBOX files: %w", err)
		}
		defer mboxClient.Close()

		// Set up context with cancellation
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		// Handle Ctrl+C gracefully
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\nInterrupted. Saving checkpoint...")
			cancel()
		}()

		// Set up sync options — SourceType is "gmail" since these are Gmail exports.
		opts := sync.DefaultOptions()
		opts.NoResume = mboxNoResume
		opts.SourceType = "gmail"
		opts.AttachmentsDir = cfg.AttachmentsDir()

		// Create syncer with progress reporter
		syncer := sync.New(mboxClient, s, opts).
			WithLogger(logger).
			WithProgress(&CLIProgress{})

		// Run sync
		startTime := time.Now()
		fmt.Printf("\nStarting MBOX import as %q\n", sourceName)
		if mboxAfter != "" || mboxBefore != "" {
			parts := []string{}
			if mboxAfter != "" {
				parts = append(parts, "after "+mboxAfter)
			}
			if mboxBefore != "" {
				parts = append(parts, "before "+mboxBefore)
			}
			fmt.Printf("Date filter: %s\n", strings.Join(parts, ", "))
		}
		if mboxLimit > 0 {
			fmt.Printf("Limit: %d messages\n", mboxLimit)
		}
		fmt.Println()

		summary, err := syncer.Full(ctx, sourceName)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\nImport interrupted. Run again to resume.")
				return nil
			}
			return fmt.Errorf("import failed: %w", err)
		}

		// Print summary
		fmt.Println()
		fmt.Println("MBOX import complete!")
		fmt.Printf("  Duration:      %s\n", summary.Duration.Round(time.Second))
		fmt.Printf("  Messages:      %d found, %d added, %d skipped\n",
			summary.MessagesFound, summary.MessagesAdded, summary.MessagesSkipped)
		if summary.Errors > 0 {
			fmt.Printf("  Errors:        %d\n", summary.Errors)
		}
		if summary.WasResumed {
			fmt.Printf("  (Resumed from checkpoint)\n")
		}

		if summary.MessagesAdded > 0 {
			elapsed := time.Since(startTime)
			messagesPerSec := float64(summary.MessagesAdded) / elapsed.Seconds()
			fmt.Printf("  Rate:          %.1f messages/sec\n", messagesPerSec)
		}

		return nil
	},
}

// resolveMboxPaths expands directories to find .mbox files recursively.
func resolveMboxPaths(args []string) ([]string, error) {
	var paths []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", arg, err)
		}
		if info.IsDir() {
			// Walk directory for .mbox files.
			err := filepath.Walk(arg, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !fi.IsDir() && strings.HasSuffix(strings.ToLower(fi.Name()), ".mbox") {
					paths = append(paths, path)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("walk %s: %w", arg, err)
			}
		} else {
			paths = append(paths, arg)
		}
	}
	return paths, nil
}

func init() {
	importMboxCmd.Flags().StringVar(&mboxSourceName, "source-name", "", "source identifier (default: mbox-import)")
	importMboxCmd.Flags().StringVar(&mboxBefore, "before", "", "only messages before this date (YYYY-MM-DD)")
	importMboxCmd.Flags().StringVar(&mboxAfter, "after", "", "only messages after this date (YYYY-MM-DD)")
	importMboxCmd.Flags().IntVar(&mboxLimit, "limit", 0, "limit number of messages (for testing)")
	importMboxCmd.Flags().BoolVar(&mboxNoResume, "noresume", false, "force fresh import (don't resume)")
	rootCmd.AddCommand(importMboxCmd)
}
