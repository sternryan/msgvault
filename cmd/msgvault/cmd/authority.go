package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/wesm/msgvault/internal/authority"
	"github.com/wesm/msgvault/internal/store"
)

var (
	authorityForgeSourcesDir string
	authorityForgeSourcesDB  string
	authorityUserEmail       string
	authorityFullRecompute   bool
)

var authorityCmd = &cobra.Command{
	Use:   "authority",
	Short: "Authority graph operations (per-sender [0,1] scoring for triage)",
	Long: `Operations on the per-sender authority score that feeds Phase 14
triage criterion #7 (replacing the static trusted_contacts.toml allowlist).

The score combines volume, response_rate_7d, and link_quality (0.2/0.4/0.4
weighted composite). See .planning/phases/16-* for the full spec.`,
}

var authorityRecomputeCmd = &cobra.Command{
	Use:   "recompute",
	Short: "Recompute per-sender authority scores",
	Long: `Recompute incrementally walks messages above last_msg_rowid and
unions touched senders with senders whose 7d reply window drifted into the
past since the previous run (D-10 two-pass union).

Use --full to bypass the watermark and rescore every sender from scratch
(D-11 parity check; should match an incremental run within ±0.001).

Reads forge sources.db read-only (?mode=ro) and walks forge sources/ on
disk to refresh the URL→source_hash cache before scoring.`,
	RunE: runAuthorityRecompute,
}

func init() {
	authorityRecomputeCmd.Flags().StringVar(
		&authorityForgeSourcesDir,
		"forge-sources-dir",
		os.Getenv("FORGE_SOURCES_DIR"),
		"Path to forge sources/ directory (default $FORGE_SOURCES_DIR or /opt/services/forge/sources)",
	)
	authorityRecomputeCmd.Flags().StringVar(
		&authorityForgeSourcesDB,
		"forge-sources-db",
		os.Getenv("FORGE_SOURCES_DB"),
		"Path to forge sources.db (default $FORGE_SOURCES_DB or /opt/services/forge/sources.db)",
	)
	authorityRecomputeCmd.Flags().StringVar(
		&authorityUserEmail,
		"user-email",
		os.Getenv("MSGVAULT_USER_EMAIL"),
		"User primary email; required only if is_from_me is unbacked and we fall back to sender_email matching",
	)
	authorityRecomputeCmd.Flags().BoolVar(
		&authorityFullRecompute,
		"full",
		false,
		"Bypass watermark; recompute every sender from scratch (D-11)",
	)
	authorityCmd.AddCommand(authorityRecomputeCmd)
	rootCmd.AddCommand(authorityCmd)
}

func runAuthorityRecompute(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Defaults — match the launchd .env conventions on the Mac Mini.
	forgeSourcesDir := authorityForgeSourcesDir
	if forgeSourcesDir == "" {
		forgeSourcesDir = "/opt/services/forge/sources"
	}
	forgeSourcesDB := authorityForgeSourcesDB
	if forgeSourcesDB == "" {
		forgeSourcesDB = "/opt/services/forge/sources.db"
	}

	// Open msgvault store RW (mirrors triage.go pattern).
	dbPath := cfg.DatabaseDSN()
	s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
	if err != nil {
		return fmt.Errorf("open msgvault store: %w", err)
	}
	defer func() { _ = s.Close() }()

	// Open forge sources.db read-only via Phase 14 seam.
	// mutecomm/go-sqlcipher/v4 driver is registered as "sqlite3" by the
	// triage package's blank import; we re-import it via internal/store
	// transitively. ?mode=ro keeps the open RO; _query_only=true is a
	// belt-and-suspenders guard.
	srcDSN := fmt.Sprintf("file:%s?mode=ro&_query_only=true", url.QueryEscape(forgeSourcesDB))
	srcDB, err := sql.Open("sqlite3", srcDSN)
	if err != nil {
		return fmt.Errorf("open forge sources.db %s: %w", forgeSourcesDB, err)
	}
	defer func() { _ = srcDB.Close() }()
	if err := srcDB.Ping(); err != nil {
		return fmt.Errorf("ping forge sources.db %s: %w", forgeSourcesDB, err)
	}

	// Recompute (incremental or full).
	var result authority.RecomputeResult
	if authorityFullRecompute {
		result, err = authority.RecomputeFull(ctx, s.DB(), srcDB, forgeSourcesDir, authorityUserEmail)
	} else {
		result, err = authority.Recompute(ctx, s.DB(), srcDB, forgeSourcesDir, authorityUserEmail)
	}
	if err != nil {
		return fmt.Errorf("authority recompute: %w", err)
	}

	logger.Info("authority recompute complete",
		"senders_updated", result.SendersUpdated,
		"new_watermark", result.NewWatermark,
		"max_volume", result.MaxVolume,
		"reply_mode", result.ReplyMode,
		"full", authorityFullRecompute,
		"forge_sources_dir", forgeSourcesDir,
		"forge_sources_db", forgeSourcesDB,
	)
	return nil
}
