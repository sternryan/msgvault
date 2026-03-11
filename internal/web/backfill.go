package web

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"io"
	"log/slog"
	"strings"

	internalmime "github.com/wesm/msgvault/internal/mime"
)

// BackfillContentIDs re-parses raw MIME for attachments missing content_id.
// Runs in a background goroutine, non-blocking on server start.
// It queries all attachments with NULL or empty content_id, groups them by
// message_id, re-parses the raw MIME, and updates matching attachments via
// content_hash lookup.
func BackfillContentIDs(db *sql.DB, logger *slog.Logger) {
	rows, err := db.Query(`
		SELECT DISTINCT a.message_id
		FROM attachments a
		WHERE (a.content_id IS NULL OR a.content_id = '')
	`)
	if err != nil {
		logger.Error("backfill: failed to query messages needing content_id", "error", err)
		return
	}
	defer rows.Close()

	var messageIDs []int64
	for rows.Next() {
		var msgID int64
		if err := rows.Scan(&msgID); err != nil {
			logger.Error("backfill: failed to scan message_id", "error", err)
			continue
		}
		messageIDs = append(messageIDs, msgID)
	}
	if err := rows.Err(); err != nil {
		logger.Error("backfill: row iteration error", "error", err)
		return
	}

	if len(messageIDs) == 0 {
		return
	}

	logger.Info("backfill: starting content_id backfill", "message_count", len(messageIDs))

	processed := 0
	updated := 0

	for _, msgID := range messageIDs {
		n, err := backfillMessage(db, msgID)
		if err != nil {
			logger.Error("backfill: error processing message", "message_id", msgID, "error", err)
			continue
		}
		updated += n
		processed++
		if processed%100 == 0 {
			logger.Info("backfill: progress", "processed", processed, "total", len(messageIDs), "updated", updated)
		}
	}

	logger.Info("backfill: complete", "processed", processed, "updated", updated)
}

// backfillMessage fetches the raw MIME for a single message, parses it,
// and updates attachments that have a content_id in the MIME but not in the DB.
// Returns the number of rows updated.
func backfillMessage(db *sql.DB, messageID int64) (int, error) {
	var rawData []byte
	var compression string
	err := db.QueryRow(`
		SELECT raw_data, compression FROM message_raw WHERE message_id = ?
	`, messageID).Scan(&rawData, &compression)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	// Decompress if needed
	decoded, err := decompressRaw(rawData, compression)
	if err != nil {
		return 0, err
	}

	// Parse MIME
	msg, err := internalmime.Parse(decoded)
	if err != nil {
		return 0, err
	}

	// Build map: content_hash -> content_id for attachments with non-empty content_id
	cidByHash := make(map[string]string)
	for _, att := range msg.Attachments {
		if att.ContentID == "" || att.ContentHash == "" {
			continue
		}
		cidByHash[att.ContentHash] = att.ContentID
	}

	if len(cidByHash) == 0 {
		return 0, nil
	}

	// Update attachments in the DB that match by content_hash
	updated := 0
	for hash, cid := range cidByHash {
		// Normalize: strip angle brackets for storage consistency
		bare := strings.Trim(cid, "<>")
		result, err := db.Exec(`
			UPDATE attachments
			SET content_id = ?
			WHERE message_id = ? AND content_hash = ? AND (content_id IS NULL OR content_id = '')
		`, bare, messageID, hash)
		if err != nil {
			continue
		}
		n, _ := result.RowsAffected()
		updated += int(n)
	}

	return updated, nil
}

// decompressRaw decompresses raw MIME data based on the stored compression type.
func decompressRaw(data []byte, compression string) ([]byte, error) {
	if compression != "zlib" {
		return data, nil
	}
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
