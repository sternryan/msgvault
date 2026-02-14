package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wesm/msgvault/internal/config"
	"github.com/wesm/msgvault/internal/store"
)

var (
	cfgFile    string
	verbose    bool
	quiet      bool
	cfg        *config.Config
	logger     *slog.Logger
	passphrase string // Database encryption passphrase
)

var rootCmd = &cobra.Command{
	Use:   "msgvault",
	Short: "Offline email archive tool",
	Long: `msgvault is an offline email archive tool that exports and stores
email data locally with full-text search capabilities.

This is the Go implementation providing sync, search, and TUI functionality
in a single binary.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for commands that don't need it
		if cmd.Name() == "version" || cmd.Name() == "update" || cmd.Name() == "quickstart" {
			return nil
		}

		// Set up logging
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		} else if quiet {
			level = slog.LevelWarn
		}
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		}))

		// Load config
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Acquire passphrase if encryption is enabled
		if cfg.Encryption.Enabled {
			if p := os.Getenv("MSGVAULT_PASSPHRASE"); p != "" {
				passphrase = p
			} else {
				fmt.Fprintf(os.Stderr, "Enter database passphrase: ")
				passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(os.Stderr)
				if err != nil {
					return fmt.Errorf("read passphrase: %w", err)
				}
				passphrase = string(passBytes)
			}
		}

		return nil
	},
}

// Execute runs the root command with a background context.
// Prefer ExecuteContext for signal-aware execution.
func Execute() error {
	return ExecuteContext(context.Background())
}

// ExecuteContext runs the root command with the given context,
// enabling graceful shutdown when the context is cancelled.
func ExecuteContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

// oauthSetupHint is the common help text for OAuth configuration issues.
const oauthSetupHint = `
To use msgvault, you need a Google Cloud OAuth credential:
  1. Follow the setup guide: https://msgvault.io/guides/oauth-setup/
  2. Download the client_secret.json file
  3. Add to your config.toml:
       [oauth]
       client_secrets = "/path/to/client_secret.json"`

// errOAuthNotConfigured returns a helpful error when OAuth client secrets are missing.
func errOAuthNotConfigured() error {
	return fmt.Errorf("OAuth client secrets not configured." + oauthSetupHint)
}

// openStore opens the msgvault database with the current passphrase (if any).
func openStore(dbPath string) (*store.Store, error) {
	var opts []store.OpenOption
	if passphrase != "" {
		opts = append(opts, store.WithPassphrase(passphrase))
	}
	return store.Open(dbPath, opts...)
}

// wrapOAuthError wraps an oauth/client-secrets error with setup instructions
// if the root cause is a missing or unreadable secrets file.
func wrapOAuthError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("OAuth client secrets file not found." + oauthSetupHint)
	}
	return err
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.msgvault/config.toml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress progress output")
}
