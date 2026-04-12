package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/store"
)

// TimelineEntry is the LifeVault-compatible JSON structure for a life event.
type TimelineEntry struct {
	Date            string `json:"date"`
	Type            string `json:"type"`
	Description     string `json:"description"`
	SourceMessageID string `json:"source_message_id"`
}

var (
	timelineOutput string
	timelineType   string
	timelinePretty bool
)

var exportTimelineCmd = &cobra.Command{
	Use:   "export-timeline",
	Short: "Export life events as LifeVault-compatible JSON",
	Long: `Export all extracted life events as a JSON array in LifeVault-compatible format.

Each entry contains:
  - date:              ISO 8601 date string (may be empty)
  - type:              event type (job/move/purchase/travel/milestone)
  - description:       human-readable description of the event
  - source_message_id: Gmail message ID for traceability

Use --type to filter by event type. Output defaults to stdout; use --output to write to a file.

Example:
  msgvault export-timeline --type job --output jobs.json`,
	RunE: runExportTimeline,
}

func init() {
	rootCmd.AddCommand(exportTimelineCmd)
	exportTimelineCmd.Flags().StringVarP(&timelineOutput, "output", "o", "", "Output file path (default: stdout)")
	exportTimelineCmd.Flags().StringVar(&timelineType, "type", "", "Filter by event type (job/move/purchase/travel/milestone)")
	exportTimelineCmd.Flags().BoolVar(&timelinePretty, "pretty", true, "Pretty-print JSON with indentation")
}

func runExportTimeline(_ *cobra.Command, _ []string) error {
	// Open database.
	s, err := openStore(cfg.DatabaseDSN())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	return runExportTimelineWithStore(s, timelineOutput, timelineType, timelinePretty)
}

// runExportTimelineWithStore is extracted for testability.
func runExportTimelineWithStore(s *store.Store, outputPath, eventType string, pretty bool) error {
	rows, err := s.GetLifeEventsForExport(eventType)
	if err != nil {
		return fmt.Errorf("get life events: %w", err)
	}

	// Map to output struct.
	entries := make([]TimelineEntry, len(rows))
	for i, r := range rows {
		entries[i] = TimelineEntry{
			Date:            r.Date,
			Type:            r.Type,
			Description:     r.Description,
			SourceMessageID: r.SourceMessageID,
		}
	}

	// Marshal to JSON.
	var data []byte
	if pretty {
		data, err = json.MarshalIndent(entries, "", "  ")
	} else {
		data, err = json.Marshal(entries)
	}
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	data = append(data, '\n')

	// Write to output.
	destination := "stdout"
	if outputPath != "" {
		destination = outputPath
		if err := os.WriteFile(outputPath, data, 0o644); err != nil {
			return fmt.Errorf("write output file %s: %w", outputPath, err)
		}
	} else {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("write to stdout: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Exported %d life events to %s\n", len(entries), destination)
	return nil
}
