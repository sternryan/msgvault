package cmd

import (
	"database/sql"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/store"
)

var buildFTSCmd = &cobra.Command{
	Use:   "build-fts",
	Short: "Build the full-text search index",
	Long: `Populate the messages_fts FTS5 table from messages, message_bodies, and participants.
This enables bare-word search to match message subject and body text.

NOTE: This requires the system 'sqlite3' CLI to be available (FTS5 support).
The Go SQLite driver used by msgvault does not include FTS5.

Run this once after initial sync, and again after large syncs to keep the
index up to date. The operation is safe to run multiple times.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()

		s, err := store.Open(dbPath, store.WithPassphrase(passphrase))
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}

		totalMessages, err := countMessages(s.DB())
		s.Close()
		if err != nil {
			return err
		}

		return buildFTSViaCLI(dbPath, totalMessages)
	},
}

func countMessages(db *sql.DB) (int64, error) {
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return count, nil
}

func buildFTSViaCLI(dbPath string, totalMessages int64) error {
	// Check sqlite3 is available
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return fmt.Errorf("sqlite3 CLI not found: %w\nInstall via: brew install sqlite3", err)
	}

	fmt.Printf("Rebuilding FTS index for %d messages using sqlite3 CLI...\n", totalMessages)

	// Note: dbPath may be a DSN like "file:/path?params" — extract the actual path
	actualPath := dbPath
	if strings.HasPrefix(dbPath, "file:") {
		// Strip file: prefix and any query params
		path := strings.TrimPrefix(dbPath, "file:")
		if idx := strings.Index(path, "?"); idx >= 0 {
			path = path[:idx]
		}
		actualPath = path
	}

	// Build the SQL to populate FTS — run via sqlite3 CLI which has FTS5 support
	populateSQL := `
DELETE FROM messages_fts;
INSERT INTO messages_fts (rowid, message_id, subject, body, from_addr, to_addr, cc_addr)
SELECT
    m.id,
    m.id,
    COALESCE(m.subject, ''),
    COALESCE(mb.body_text, ''),
    COALESCE(p_from.email_address, ''),
    COALESCE(to_r.addrs, ''),
    COALESCE(cc_r.addrs, '')
FROM messages m
LEFT JOIN message_bodies mb ON mb.message_id = m.id
LEFT JOIN message_recipients mr_from ON mr_from.message_id = m.id AND mr_from.recipient_type = 'from'
LEFT JOIN participants p_from ON p_from.id = mr_from.participant_id
LEFT JOIN (
    SELECT mr.message_id, GROUP_CONCAT(p.email_address, ' ') as addrs
    FROM message_recipients mr
    JOIN participants p ON p.id = mr.participant_id
    WHERE mr.recipient_type = 'to'
    GROUP BY mr.message_id
) to_r ON to_r.message_id = m.id
LEFT JOIN (
    SELECT mr.message_id, GROUP_CONCAT(p.email_address, ' ') as addrs
    FROM message_recipients mr
    JOIN participants p ON p.id = mr.participant_id
    WHERE mr.recipient_type = 'cc'
    GROUP BY mr.message_id
) cc_r ON cc_r.message_id = m.id;
SELECT 'Indexed ' || COUNT(*) || ' messages.' FROM messages_fts;
`

	cmd := exec.Command("sqlite3", actualPath)
	cmd.Stdin = strings.NewReader(populateSQL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite3 failed: %w\nOutput: %s", err, string(out))
	}

	output := strings.TrimSpace(string(out))
	if output != "" {
		fmt.Println(output)
	}
	fmt.Println("FTS index built. Restart msgvault web to use full-text search.")
	return nil
}

func init() {
	rootCmd.AddCommand(buildFTSCmd)
}
