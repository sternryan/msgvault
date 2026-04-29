package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/wesm/msgvault/internal/authority"
	"github.com/wesm/msgvault/internal/store"
	"github.com/wesm/msgvault/internal/triage"
)

var (
	triageSince        string
	triageOut          string
	triageForgeGraph   string
	triageForgeSources string
	triageNoise        string
	triageMaxN         int
	triageThreshold    float64
	triageRyanEmail    string
	triageLimit        int
)

var triageCmd = &cobra.Command{
	Use:   "triage",
	Short: "Score archived Gmail and emit JSONL digest candidates",
	Long: `Score archived messages over a recent window using the 7-criterion
composite scorer (TRIAGE-02), apply hard filters (TRIAGE-03), and emit
JSONL of the top-N candidates (default 25, threshold 0.55) sorted by
score desc.

Reads forge graph.db / sources.db in read-only mode. Per Phase 14 D-04
the scoring path is pure-lexical/SQL — no LLM/encoder calls.`,
	RunE: runTriage,
}

func init() {
	triageCmd.Flags().StringVar(&triageSince, "since", "7d", "Lookback duration (e.g. 7d, 24h)")
	triageCmd.Flags().StringVar(&triageOut, "out", "", "Output JSONL path (default stdout)")
	triageCmd.Flags().StringVar(&triageForgeGraph, "forge-graph", "", "Path to forge graph.db (read-only)")
	triageCmd.Flags().StringVar(&triageForgeSources, "forge-sources", "", "Path to forge sources.db (read-only)")
	triageCmd.Flags().StringVar(&triageNoise, "noise-domains", "", "Noise domains TOML (optional override)")
	triageCmd.Flags().IntVar(&triageMaxN, "max", 25, "Top-N candidates to emit")
	triageCmd.Flags().Float64Var(&triageThreshold, "threshold", 0.55, "Composite score threshold (0..1)")
	triageCmd.Flags().StringVar(&triageRyanEmail, "user-email", "", "User's primary email (used for curiosity/decision scoring)")
	triageCmd.Flags().IntVar(&triageLimit, "limit", 5000, "Max messages to score per run")
	rootCmd.AddCommand(triageCmd)
}

func runTriage(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	since, err := triage.ParseSinceDuration(triageSince)
	if err != nil {
		return fmt.Errorf("parse --since: %w", err)
	}

	// Open msgvault store (RW — same process may be sharing it).
	dbPath := cfg.DatabaseDSN()
	s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	// Load candidate messages.
	msgs, err := triage.LoadMessages(ctx, s.DB(), since, triageLimit)
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}

	// Open forge state (read-only). Each is optional — degraded scoring if absent.
	var lex *triage.Lexicon
	var tp *triage.TopicPairs
	if triageForgeGraph != "" {
		if l, err := triage.OpenLexicon(triageForgeGraph); err == nil {
			lex = l
			defer lex.Close()
		} else {
			logger.Warn("open forge graph", "path", triageForgeGraph, "err", err)
		}
		if t, err := triage.OpenTopicPairs(triageForgeGraph); err == nil {
			tp = t
			defer tp.Close()
		}
	}
	var sources *triage.SourcesDB
	if triageForgeSources != "" {
		if src, err := triage.OpenSources(triageForgeSources); err == nil {
			sources = src
			defer sources.Close()
		} else {
			logger.Warn("open forge sources", "path", triageForgeSources, "err", err)
		}
	}

	// Phase 16 (AUTHGRAPH-03): the static contact-allowlist file is
	// RETIRED. Criterion #7 (expert) consumes authority.Store; criterion
	// #3 (curiosity) loses the trusted-contact bucket and degrades to the
	// 0.2 unknown-sender bucket until a future plan supplies a computed
	// trusted-contact list (likely a thresholded cut over authority_scores).
	authStore := authority.NewSQLiteStore(s.DB())

	// Build recurrence index from the same window.
	rec := triage.NewRecurrence(msgs, time.Now())

	// Output writer
	out := os.Stdout
	if triageOut != "" {
		f, err := os.Create(triageOut)
		if err != nil {
			return fmt.Errorf("open --out: %w", err)
		}
		defer f.Close()
		out = f
	}

	n, err := triage.RunTriage(triage.RunOptions{
		Messages:        msgs,
		Lexicon:         lex,
		TopicPairs:      tp,
		Sources:         sources,
		Recurrence:      rec,
		TrustedContacts: nil,
		AuthorityStore:  authStore,
		RyanEmail:       triageRyanEmail,
		Threshold:       triageThreshold,
		MaxN:            triageMaxN,
		Out:             out,
	})
	if err != nil {
		return fmt.Errorf("run triage: %w", err)
	}
	logger.Info("triage complete", "candidates", n, "scanned", len(msgs))
	return nil
}
