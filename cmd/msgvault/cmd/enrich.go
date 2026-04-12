package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/enrichment"
	"github.com/wesm/msgvault/internal/store"
)

var (
	enrichBatchSize  int
	enrichDeployment string
	enrichDryRun     bool
)

var enrichCmd = &cobra.Command{
	Use:   "enrich",
	Short: "Categorize messages and extract life events and entities using Azure OpenAI",
	Long: `Enrich all messages via Azure OpenAI GPT-4o-mini by:
  - Assigning exactly one AI category label (finance/travel/legal/health/shopping/newsletters/personal/work)
  - Extracting life events (jobs, moves, purchases, travel, milestones)
  - Extracting entities (people, companies, dates, amounts)

The enrichment run is resumable — interrupted runs resume from the last checkpoint.
Only uncategorized messages are processed (idempotent).

Requires [azure_openai] configuration in config.toml:
  [azure_openai]
  endpoint = "https://YOUR-INSTANCE.openai.azure.com"
  # api_key_env = "AZURE_OPENAI_API_KEY"  (default env var)
  [azure_openai.deployments]
  chat = "gpt-4o-mini"`,
	RunE: runEnrich,
}

func init() {
	rootCmd.AddCommand(enrichCmd)
	enrichCmd.Flags().IntVar(&enrichBatchSize, "batch-size", 20, "Number of messages per batch (each message makes one API call)")
	enrichCmd.Flags().StringVar(&enrichDeployment, "deployment", "chat", "Azure OpenAI deployment name for chat completions")
	enrichCmd.Flags().BoolVar(&enrichDryRun, "dry-run", false, "Count unenriched messages and print estimate without processing")
}

func runEnrich(cmd *cobra.Command, args []string) error {
	if cfg.AzureOpenAI.Endpoint == "" {
		return fmt.Errorf("azure_openai.endpoint not configured\n\n" +
			"Add to config.toml:\n" +
			"  [azure_openai]\n" +
			"  endpoint = \"https://YOUR-INSTANCE.openai.azure.com\"\n" +
			"  [azure_openai.deployments]\n" +
			"  chat = \"gpt-4o-mini\"")
	}

	// Open database.
	s, err := openStore(cfg.DatabaseDSN())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	// Dry-run: show stats without calling the API.
	if enrichDryRun {
		return runEnrichDryRun(s)
	}

	// Create AI client.
	aiClient, err := ai.NewClient(cfg.AzureOpenAI, ai.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("create AI client: %w", err)
	}

	slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	if verbose {
		slogLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	fmt.Fprintf(os.Stderr, "Starting enrichment pipeline (batch-size=%d, deployment=%s)...\n",
		enrichBatchSize, enrichDeployment)

	if err := enrichment.RunEnrichPipeline(cmd.Context(), aiClient, s, slogLogger, enrichBatchSize, enrichDeployment); err != nil {
		return fmt.Errorf("enrichment pipeline: %w", err)
	}

	fmt.Println("Enrichment complete.")
	return nil
}

// runEnrichDryRun prints the count of unenriched messages and an estimated cost.
// s is the already-opened and schema-initialized store.
func runEnrichDryRun(s *store.Store) error {
	count, err := enrichment.CountUnenriched(s)
	if err != nil {
		return fmt.Errorf("count unenriched: %w", err)
	}

	// GPT-4o-mini pricing: ~300 input tokens per message (subject+snippet+prompt), ~100 output
	estimatedInputTokens := count * 300
	estimatedOutputTokens := count * 100
	estimatedCost := float64(estimatedInputTokens)*(0.15/1_000_000) +
		float64(estimatedOutputTokens)*(0.60/1_000_000)

	fmt.Printf("Unenriched messages:    %d\n", count)
	fmt.Printf("Estimated input tokens: ~%d\n", estimatedInputTokens)
	fmt.Printf("Estimated output tokens:~%d\n", estimatedOutputTokens)
	fmt.Printf("Estimated cost:         ~$%.4f\n", estimatedCost)
	return nil
}
