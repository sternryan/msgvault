package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/store"
	"github.com/wesm/msgvault/internal/tui"
)

var forceSQL bool
var skipCacheBuild bool

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive terminal UI",
	Long: `Open an interactive terminal UI for browsing your email archive.

The TUI provides aggregate views of your messages by:
  - Senders: Who sends you the most email
  - Recipients: Who you email most frequently
  - Domains: Which domains you interact with
  - Labels: Gmail label distribution
  - Time: Message volume over time

Navigation:
  ↑/k, ↓/j    Move up/down
  PgUp/PgDn   Page up/down
  Enter       Drill down / view message
  Esc         Go back
  Tab         Switch view (aggregates only)
  s           Cycle sort field
  r           Reverse sort direction
  t           Toggle time granularity (Time view only)

Selection & Deletion:
  Space       Toggle selection
  A           Select all visible
  x           Clear selection
  D           Stage selected for deletion
  q           Quit

Performance:
  For large archives (100k+ messages), the TUI uses Parquet files for fast
  aggregation queries. Run 'msgvault-sync build-parquet' to generate them.
  Use --force-sql to bypass Parquet and query SQLite directly (slow).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()
		analyticsDir := cfg.AnalyticsDir()

		_, engine, cleanup, err := initQueryEngine(dbPath, analyticsDir, forceSQL, skipCacheBuild)
		if err != nil {
			return err
		}
		defer cleanup()

		// Create and run TUI
		model := tui.New(engine, tui.Options{DataDir: cfg.Data.DataDir, Version: Version})
		p := tea.NewProgram(model, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("run tui: %w", err)
		}

		return nil
	},
}

// cacheNeedsBuild checks if the analytics cache needs to be built or updated.
// Returns (needsBuild, reason) where reason describes why.
func cacheNeedsBuild(dbPath, analyticsDir string) (bool, string) {
	messagesDir := filepath.Join(analyticsDir, "messages")
	stateFile := filepath.Join(analyticsDir, "_last_sync.json")

	// Check if cache directory exists with parquet files
	if !query.HasParquetData(analyticsDir) {
		return true, "no cache exists"
	}

	// Load last sync state
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return true, "no sync state found"
	}

	var state syncState
	if err := json.Unmarshal(data, &state); err != nil {
		return true, "invalid sync state"
	}

	// Check if SQLite has newer messages
	// We need to query SQLite directly to check max message ID
	db, err := store.Open(dbPath, store.WithPassphrase(passphrase))
	if err != nil {
		// Can't open DB to check - force rebuild to be safe
		return true, "cannot verify cache status"
	}
	defer db.Close()

	var maxID int64
	err = db.DB().QueryRow(`
		SELECT COALESCE(MAX(id), 0) FROM messages
		WHERE deleted_from_source_at IS NULL AND sent_at IS NOT NULL
	`).Scan(&maxID)
	if err != nil {
		// Can't query - force rebuild to be safe
		return true, "cannot verify cache status"
	}

	if maxID > state.LastMessageID {
		newCount := maxID - state.LastMessageID
		return true, fmt.Sprintf("%d new messages", newCount)
	}

	// Check if parquet files actually exist (directory might be empty)
	files, _ := filepath.Glob(filepath.Join(messagesDir, "*", "*.parquet"))
	if len(files) == 0 {
		return true, "cache directory empty"
	}

	return false, ""
}

func init() {
	rootCmd.AddCommand(tuiCmd)
	tuiCmd.Flags().BoolVar(&forceSQL, "force-sql", false, "Force SQLite queries instead of Parquet (slow for large archives)")
	tuiCmd.Flags().BoolVar(&skipCacheBuild, "no-cache-build", false, "Skip automatic cache build/update")
}
