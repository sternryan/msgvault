package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/wesm/msgvault/internal/store"
	"github.com/wesm/msgvault/internal/trustedcontacts"
)

var (
	tcBootstrapTop       int
	tcBootstrapOut       string
	tcBootstrapSince     time.Duration
	tcBootstrapEmail     string
	tcBootstrapNoiseTOML string
)

var trustedContactsCmd = &cobra.Command{
	Use:   "trusted-contacts",
	Short: "Manage trusted contacts (Phase 14 static seed)",
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Generate trusted_contacts.toml from msgvault history",
	Long: `Aggregate top-N senders+recipients (bidirectional volume) over a
configurable lookback window and write a hand-editable TOML allowlist.

Phase 16 will replace this static seed with a computed authority graph.`,
	RunE: runTrustedContactsBootstrap,
}

func init() {
	bootstrapCmd.Flags().IntVar(&tcBootstrapTop, "top", 10, "Top-N senders to include")
	bootstrapCmd.Flags().StringVar(&tcBootstrapOut, "out", "trusted_contacts.toml", "Output TOML path")
	bootstrapCmd.Flags().DurationVar(&tcBootstrapSince, "since", 365*24*time.Hour, "Lookback window")
	bootstrapCmd.Flags().StringVar(&tcBootstrapEmail, "user-email", "", "User's primary email (for bidirectional weighting)")
	bootstrapCmd.Flags().StringVar(&tcBootstrapNoiseTOML, "noise-domains", "", "Noise domains TOML (optional)")

	trustedContactsCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(trustedContactsCmd)
}

func runTrustedContactsBootstrap(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	dbPath := cfg.DatabaseDSN()
	s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = s.Close() }()
	if err := s.InitSchema(); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	out, err := os.Create(tcBootstrapOut)
	if err != nil {
		return fmt.Errorf("open --out: %w", err)
	}
	defer out.Close()

	if err := trustedcontacts.Bootstrap(ctx, s.DB(), tcBootstrapEmail, tcBootstrapTop, tcBootstrapSince, nil, out); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	logger.Info("trusted-contacts bootstrap complete", "out", tcBootstrapOut, "top", tcBootstrapTop)
	return nil
}
