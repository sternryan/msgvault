package cmd

import (
	"context"

	"github.com/spf13/cobra"
	mcpserver "github.com/wesm/msgvault/internal/mcp"
)

var mcpForceSQL bool

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run MCP server for Claude Desktop integration",
	Long: `Start an MCP (Model Context Protocol) server over stdio.

This allows Claude Desktop (or any MCP client) to query your email archive
using tools like search_messages, get_message, list_messages, get_stats,
and aggregate.

Add to Claude Desktop config:
  {
    "mcpServers": {
      "msgvault": {
        "command": "msgvault",
        "args": ["mcp"]
      }
    }
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()
		analyticsDir := cfg.AnalyticsDir()

		_, engine, cleanup, err := initQueryEngine(dbPath, analyticsDir, mcpForceSQL, true)
		if err != nil {
			return err
		}
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		return mcpserver.Serve(ctx, engine, cfg.AttachmentsDir())
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().BoolVar(&mcpForceSQL, "force-sql", false, "Force SQLite queries instead of Parquet")
}
