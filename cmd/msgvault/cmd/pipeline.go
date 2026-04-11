package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/ai"
)

var pipelineCmd = &cobra.Command{
	Use:    "pipeline",
	Short:  "AI pipeline commands",
	Hidden: true, // Infrastructure — user-facing commands come in Phase 13/14
}

var pipelineTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Validate pipeline infrastructure (config, client, checkpoint, progress)",
	RunE:  runPipelineTest,
}

func init() {
	rootCmd.AddCommand(pipelineCmd)
	pipelineCmd.AddCommand(pipelineTestCmd)
	pipelineTestCmd.Flags().Int("messages", 50, "Number of fake messages to process")
	pipelineTestCmd.Flags().Int("batch-size", 10, "Batch size")
}

func runPipelineTest(cmd *cobra.Command, args []string) error {
	// Validate Azure OpenAI config
	if cfg.AzureOpenAI.Endpoint == "" {
		return fmt.Errorf("azure_openai.endpoint not configured in config.toml\n\n" +
			"Add to config.toml:\n" +
			"  [azure_openai]\n" +
			"  endpoint = \"https://YOUR-INSTANCE.openai.azure.com\"\n" +
			"  # api_key_env = \"AZURE_OPENAI_API_KEY\"  # default\n" +
			"  # tpm_limit = 120000\n" +
			"  # rpm_limit = 720\n" +
			"  [azure_openai.deployments]\n" +
			"  embedding = \"text-embedding-3-small\"\n" +
			"  chat = \"gpt-4o-mini\"")
	}

	// Test API key resolution
	apiKey, err := cfg.AzureOpenAI.ResolveAPIKey()
	if err != nil {
		return fmt.Errorf("API key resolution failed: %w", err)
	}
	// Show first/last 4 chars only — never print full API key
	keyDisplay := apiKey
	if len(apiKey) > 8 {
		keyDisplay = apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
	}
	fmt.Printf("Config OK: endpoint=%s, api_key=%s\n",
		cfg.AzureOpenAI.Endpoint, keyDisplay)

	// Open store and verify pipeline tables are present
	st, err := openStore(cfg.DatabaseDSN())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	if err := st.InitSchema(); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	// Run a dry-run pipeline with fake messages
	msgCount, _ := cmd.Flags().GetInt("messages")
	batchSize, _ := cmd.Flags().GetInt("batch-size")

	if msgCount <= 0 {
		return fmt.Errorf("--messages must be positive")
	}
	if batchSize <= 0 {
		return fmt.Errorf("--batch-size must be positive")
	}

	// Generate fake message IDs
	fakeMessages := make([]ai.MessageRow, msgCount)
	for i := range fakeMessages {
		fakeMessages[i] = ai.MessageRow{
			ID:      int64(i + 1),
			Subject: fmt.Sprintf("Test message %d", i+1),
			Snippet: "This is a test message for pipeline validation.",
		}
	}

	queryFn := func(afterID int64, limit int) ([]ai.MessageRow, error) {
		var result []ai.MessageRow
		for _, m := range fakeMessages {
			if m.ID > afterID {
				result = append(result, m)
				if len(result) >= limit {
					break
				}
			}
		}
		return result, nil
	}

	// Dry-run process function (no API calls)
	processFn := func(ctx context.Context, messages []ai.MessageRow) (*ai.BatchResult, error) {
		if len(messages) == 0 {
			return &ai.BatchResult{}, nil
		}
		return &ai.BatchResult{
			Processed:     int64(len(messages)),
			TokensInput:   int64(len(messages) * 100), // fake token counts
			TokensOutput:  0,
			CostUSD:       float64(len(messages)) * 0.000002, // fake cost
			LastMessageID: messages[len(messages)-1].ID,
		}, nil
	}

	runner := ai.NewBatchRunner(ai.RunConfig{
		PipelineType:    "test",
		BatchSize:       batchSize,
		CheckpointEvery: 5,
		Store:           st,
		QueryMessages:   queryFn,
		Process:         processFn,
	})

	fmt.Printf("Running dry-run pipeline: %d messages, batch size %d\n\n", msgCount, batchSize)

	if err := runner.Run(cmd.Context()); err != nil {
		return fmt.Errorf("pipeline test failed: %w", err)
	}

	fmt.Println("\nPipeline infrastructure validated successfully.")
	return nil
}
