package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/wesm/msgvault/internal/digest"
	"github.com/wesm/msgvault/internal/gmail"
	"github.com/wesm/msgvault/internal/oauth"
	"github.com/wesm/msgvault/internal/triage"
)

var (
	digestIn       string
	digestTo       string
	digestFrom     string
	digestAccount  string
	digestSubject  string
	digestDryRun   bool
	digestOAuthApp string
)

var digestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Build and send weekly triage digest emails",
}

var digestSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Format triage JSONL into a markdown digest and send via Gmail",
	Long: `Reads a JSONL produced by 'msgvault triage' and renders the locked
D-05 markdown body. Sends via Gmail OAuth if --dry-run is not set.

Requires the gmail.send scope on the active token. The startup check
fails fast with a re-auth instruction (msgvault add-account --reauth)
if the scope is absent.`,
	RunE: runDigestSend,
}

func init() {
	digestSendCmd.Flags().StringVar(&digestIn, "in", "", "JSONL path from 'msgvault triage' (required)")
	digestSendCmd.Flags().StringVar(&digestTo, "to", "", "Recipient email (required)")
	digestSendCmd.Flags().StringVar(&digestFrom, "from", "", "Sender email; defaults to --account when omitted")
	digestSendCmd.Flags().StringVar(&digestAccount, "account", "", "msgvault account whose OAuth token sends the email (required unless --dry-run)")
	digestSendCmd.Flags().StringVar(&digestSubject, "subject", "Weekly triage digest", "Email subject line")
	digestSendCmd.Flags().BoolVar(&digestDryRun, "dry-run", false, "Print rendered body to stdout instead of sending")
	digestSendCmd.Flags().StringVar(&digestOAuthApp, "oauth-app", "", "Named OAuth app (matches config.toml [oauth.apps.*])")

	digestCmd.AddCommand(digestSendCmd)
	rootCmd.AddCommand(digestCmd)
}

func runDigestSend(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if digestIn == "" {
		return fmt.Errorf("--in is required")
	}
	candidates, err := readJSONL(digestIn)
	if err != nil {
		return fmt.Errorf("read jsonl: %w", err)
	}
	body := digest.Format(candidates)

	if digestDryRun {
		fmt.Print(body)
		return nil
	}

	// Sending requires --to and --account.
	if digestTo == "" {
		return fmt.Errorf("--to is required (unless --dry-run)")
	}
	if digestAccount == "" {
		return fmt.Errorf("--account is required (unless --dry-run)")
	}
	from := digestFrom
	if from == "" {
		from = digestAccount
	}

	// Acquire OAuth manager + verify gmail.send scope (T-14-11).
	mgrFor := oauthManagerCache()
	mgr, err := mgrFor(digestOAuthApp)
	if err != nil {
		return fmt.Errorf("oauth manager: %w", err)
	}
	// Re-instantiate with ScopesTriage so future re-auth requests will
	// include gmail.send.
	mgrTriage, err := oauth.NewManagerWithScopes(
		mustClientSecretsPath(digestOAuthApp),
		cfg.TokensDir(),
		logger,
		oauth.ScopesTriage,
	)
	if err != nil {
		return fmt.Errorf("oauth manager (triage scopes): %w", err)
	}
	_ = mgr // keep regular mgr alive for token cache; mgrTriage is used for HasScope/TokenSource.

	if !mgrTriage.HasScope(digestAccount, "https://www.googleapis.com/auth/gmail.send") {
		return fmt.Errorf(
			"account %s does not have gmail.send scope. Re-authorize with:\n"+
				"  msgvault add-account %s --reauth",
			digestAccount, digestAccount,
		)
	}
	tokenSource, err := mgrTriage.TokenSource(ctx, digestAccount)
	if err != nil {
		return fmt.Errorf("token source: %w", err)
	}

	client := gmail.NewClient(tokenSource, gmail.WithLogger(logger))
	defer func() { _ = client.Close() }()

	rfc := digest.BuildRFC822(from, digestTo, digestSubject, body)
	id, err := client.Send(ctx, rfc)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	logger.Info("digest sent", "id", id, "to", digestTo, "candidates", len(candidates))
	fmt.Printf("Sent (id=%s) to %s — %d candidates.\n", id, digestTo, len(candidates))
	return nil
}

// readJSONL streams a triage JSONL file into []triage.Candidate.
func readJSONL(path string) ([]triage.Candidate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []triage.Candidate
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var c triage.Candidate
		if err := dec.Decode(&c); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode jsonl: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}

// mustClientSecretsPath returns the resolved path or panics — only used
// after oauthManagerCache has already validated config.
func mustClientSecretsPath(appName string) string {
	p, err := cfg.OAuth.ClientSecretsFor(appName)
	if err != nil {
		// This shouldn't happen because oauthManagerCache succeeded above.
		panic(err)
	}
	return p
}
