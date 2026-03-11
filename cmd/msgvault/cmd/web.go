package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/web"
)

var (
	webPort         int
	webNoBrowser    bool
	webForceSQL     bool
	webNoCacheBuild bool
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Launch the web UI in your browser",
	Long: `Start a local web server and open the msgvault web UI in your browser.

The web UI provides:
  - Dashboard with stats and charts
  - Aggregate views (Senders, Recipients, Domains, Labels, Time)
  - Message list and detail views with HTML email rendering
  - Full-text search with Gmail-like syntax
  - Selection and deletion staging

The server listens on localhost only (no external access).
Press Ctrl+C to stop the server.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()
		analyticsDir := cfg.AnalyticsDir()

		_, engine, cleanup, err := initQueryEngine(dbPath, analyticsDir, webForceSQL, webNoCacheBuild)
		if err != nil {
			return err
		}
		defer cleanup()

		// Set up deletion manager
		deletionsDir := filepath.Join(cfg.Data.DataDir, "deletions")
		delMgr, err := deletion.NewManager(deletionsDir)
		if err != nil {
			return fmt.Errorf("init deletion manager: %w", err)
		}

		// Set up logging
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		}
		webLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		}))

		srv := web.NewServer(engine, cfg.AttachmentsDir(), delMgr, webLogger)

		addr := fmt.Sprintf("127.0.0.1:%d", webPort)
		url := fmt.Sprintf("http://%s", addr)

		fmt.Printf("Starting msgvault web UI at %s\n", url)
		fmt.Println("Press Ctrl+C to stop")

		// Open browser unless disabled
		if !webNoBrowser {
			go openBrowser(url)
		}

		// Set up signal handling for graceful shutdown
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		return srv.Start(ctx, addr)
	},
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	cmd.Run() //nolint:errcheck
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().IntVar(&webPort, "port", 8484, "Port to listen on")
	webCmd.Flags().BoolVar(&webNoBrowser, "no-browser", false, "Don't auto-open browser")
	webCmd.Flags().BoolVar(&webForceSQL, "force-sql", false, "Force SQLite queries instead of Parquet")
	webCmd.Flags().BoolVar(&webNoCacheBuild, "no-cache-build", false, "Skip automatic cache build/update")
}
