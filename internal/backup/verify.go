package backup

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wesm/msgvault/internal/mime"
	"github.com/wesm/msgvault/internal/store"
)

// VerificationResult categorizes issues found during verification.
type VerificationResult struct {
	Verified    int      // Messages that passed all checks
	MissingMeta []string // Gmail IDs with no message record in DB
	MissingRaw  []string // Gmail IDs with message record but no raw MIME
	CorruptRaw  []string // Gmail IDs where raw MIME fails to decompress or parse
}

// HasIssues returns true if any problems were found.
func (r *VerificationResult) HasIssues() bool {
	return len(r.MissingMeta) > 0 || len(r.MissingRaw) > 0 || len(r.CorruptRaw) > 0
}

// Summary returns a human-readable summary of verification issues.
func (r *VerificationResult) Summary() string {
	var sb strings.Builder
	sb.WriteString("Verification issues:\n")
	if len(r.MissingMeta) > 0 {
		sb.WriteString(fmt.Sprintf("  Missing from DB:       %d messages\n", len(r.MissingMeta)))
	}
	if len(r.MissingRaw) > 0 {
		sb.WriteString(fmt.Sprintf("  Missing raw MIME:      %d messages\n", len(r.MissingRaw)))
	}
	if len(r.CorruptRaw) > 0 {
		sb.WriteString(fmt.Sprintf("  Corrupt raw MIME:      %d messages\n", len(r.CorruptRaw)))
	}
	total := len(r.MissingMeta) + len(r.MissingRaw) + len(r.CorruptRaw)
	sb.WriteString(fmt.Sprintf("  Total issues:          %d\n", total))
	sb.WriteString(fmt.Sprintf("  Verified OK:           %d\n", r.Verified))
	return sb.String()
}

// VerifyMessagesForDeletion checks that messages exist and are intact in the local archive.
// For each Gmail ID: confirms message exists in DB, raw MIME decompresses, MIME parses.
// Processes in batches of 500 to avoid SQLite parameter limits.
func VerifyMessagesForDeletion(ctx context.Context, s *store.Store, gmailIDs []string) (*VerificationResult, error) {
	result := &VerificationResult{}

	const batchSize = 500
	for i := 0; i < len(gmailIDs); i += batchSize {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		end := i + batchSize
		if end > len(gmailIDs) {
			end = len(gmailIDs)
		}
		batch := gmailIDs[i:end]

		// Step 1: Find which gmail IDs have message records in the DB.
		// Query messages table by source_message_id (gmail IDs are stored there).
		idMap, err := lookupGmailIDs(s.DB(), batch)
		if err != nil {
			return result, fmt.Errorf("lookup gmail IDs: %w", err)
		}

		// Identify missing messages
		for _, gmailID := range batch {
			if _, ok := idMap[gmailID]; !ok {
				result.MissingMeta = append(result.MissingMeta, gmailID)
			}
		}

		// Step 2 & 3: For existing messages, verify raw MIME
		for gmailID, internalID := range idMap {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}

			rawData, err := s.GetMessageRaw(internalID)
			if err != nil {
				if err == sql.ErrNoRows {
					result.MissingRaw = append(result.MissingRaw, gmailID)
				} else {
					result.CorruptRaw = append(result.CorruptRaw, gmailID)
				}
				continue
			}

			// Try to parse MIME
			_, err = mime.Parse(rawData)
			if err != nil {
				result.CorruptRaw = append(result.CorruptRaw, gmailID)
				continue
			}

			result.Verified++
		}
	}

	return result, nil
}

// lookupGmailIDs queries the messages table for source_message_id values,
// returning a map of source_message_id -> internal message ID.
func lookupGmailIDs(db *sql.DB, gmailIDs []string) (map[string]int64, error) {
	if len(gmailIDs) == 0 {
		return make(map[string]int64), nil
	}

	result := make(map[string]int64, len(gmailIDs))

	placeholders := make([]string, len(gmailIDs))
	args := make([]interface{}, len(gmailIDs))
	for i, id := range gmailIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		"SELECT source_message_id, id FROM messages WHERE source_message_id IN (%s)",
		strings.Join(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var srcID string
		var id int64
		if err := rows.Scan(&srcID, &id); err != nil {
			return nil, err
		}
		result[srcID] = id
	}
	return result, rows.Err()
}
