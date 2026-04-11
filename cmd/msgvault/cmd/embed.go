package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/ai"
	"github.com/wesm/msgvault/internal/embedding"
	"github.com/wesm/msgvault/internal/store"
)

var (
	embedBatchSize int
	embedDryRun    bool
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Embed messages for semantic search using Azure OpenAI",
	Long: `Embed all messages via Azure OpenAI text-embedding-3-small and store
vectors in sqlite-vec for semantic search.

The embedding run is resumable — interrupted runs resume from the last checkpoint.
Only messages not yet embedded are processed (idempotent).

Requires [azure_openai] configuration in config.toml:
  [azure_openai]
  endpoint = "https://YOUR-INSTANCE.openai.azure.com"
  # api_key_env = "AZURE_OPENAI_API_KEY"  (default env var)
  [azure_openai.deployments]
  text-embedding = "text-embedding-3-small"`,
	RunE: runEmbed,
}

func init() {
	rootCmd.AddCommand(embedCmd)
	embedCmd.Flags().IntVar(&embedBatchSize, "batch-size", 100, "Number of messages per API batch")
	embedCmd.Flags().BoolVar(&embedDryRun, "dry-run", false, "Print unembedded count and estimated cost without calling the API")
}

func runEmbed(cmd *cobra.Command, args []string) error {
	if cfg.AzureOpenAI.Endpoint == "" {
		return fmt.Errorf("azure_openai.endpoint not configured\n\n" +
			"Add to config.toml:\n" +
			"  [azure_openai]\n" +
			"  endpoint = \"https://YOUR-INSTANCE.openai.azure.com\"\n" +
			"  [azure_openai.deployments]\n" +
			"  text-embedding = \"text-embedding-3-small\"")
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
	if embedDryRun {
		return runEmbedDryRunWithStore(s)
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

	fmt.Fprintf(os.Stderr, "Starting embedding pipeline (batch-size=%d)...\n", embedBatchSize)

	if err := embedding.RunEmbedPipeline(cmd.Context(), aiClient, s, slogLogger); err != nil {
		return fmt.Errorf("embedding pipeline: %w", err)
	}

	// Show final count.
	count, err := s.EmbeddingCount()
	if err != nil {
		return fmt.Errorf("get embedding count: %w", err)
	}
	fmt.Printf("Done. %d total embeddings stored.\n", count)
	return nil
}

// runEmbedDryRunWithStore prints the count of unembedded messages and estimated cost.
func runEmbedDryRunWithStore(s *store.Store) error {
	count, err := embedding.CountUnembedded(s)
	if err != nil {
		return fmt.Errorf("count unembedded: %w", err)
	}

	// text-embedding-3-small: ~200 tokens per subject+snippet on average.
	estimatedTokens := count * 200
	estimatedCost := float64(estimatedTokens) * (0.02 / 1_000_000)

	fmt.Printf("Unembedded messages: %d\n", count)
	fmt.Printf("Estimated tokens:    ~%d\n", estimatedTokens)
	fmt.Printf("Estimated cost:      ~$%.4f\n", estimatedCost)
	return nil
}
