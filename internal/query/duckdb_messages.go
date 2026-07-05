package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ListMessages retrieves messages from Parquet files for fast filtered queries.
// Joins normalized Parquet tables to reconstruct denormalized view.
func (e *DuckDBEngine) ListMessages(ctx context.Context, filter MessageFilter) ([]MessageSummary, error) {
	where, args := e.buildFilterConditions(filter)

	// Build ORDER BY
	var orderBy string
	switch filter.Sorting.Field {
	case MessageSortByDate:
		orderBy = "msg.sent_at"
	case MessageSortBySize:
		orderBy = "msg.size_estimate"
	case MessageSortBySubject:
		orderBy = "msg.subject"
	default:
		orderBy = "msg.sent_at"
	}
	if filter.Sorting.Direction == SortDesc {
		orderBy += " DESC"
	} else {
		orderBy += " ASC"
	}

	limit := filter.Pagination.Limit
	if limit == 0 {
		limit = 500
	}

	// Optimized query structure:
	// 1. filtered_msgs: filter and paginate message IDs first (EXISTS becomes semi-join)
	// 2. msg_sender: only compute sender info for the filtered messages
	// 3. Final SELECT: join filtered messages with sender info
	query := fmt.Sprintf(`
		WITH %s,
		filtered_msgs AS (
			SELECT msg.id
			FROM msg
			WHERE %s
			ORDER BY %s
			LIMIT ? OFFSET ?
		),
		msg_sender AS (
			SELECT mr.message_id,
				   FIRST(p.email_address) as from_email,
				   FIRST(COALESCE(mr.display_name, p.display_name, '')) as from_name,
				   FIRST(COALESCE(p.phone_number, '')) as from_phone
			FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.recipient_type = 'from'
			  AND mr.message_id IN (SELECT id FROM filtered_msgs)
			GROUP BY mr.message_id
		),
		direct_sender AS (
			SELECT msg.id as message_id,
				   COALESCE(p.email_address, '') as from_email,
				   COALESCE(p.display_name, '') as from_name,
				   COALESCE(p.phone_number, '') as from_phone
			FROM msg
			JOIN filtered_msgs fm ON fm.id = msg.id
			JOIN p ON p.id = msg.sender_id
			WHERE msg.sender_id IS NOT NULL
			  AND msg.id NOT IN (SELECT message_id FROM msg_sender)
		)
		SELECT
			msg.id,
			COALESCE(msg.source_message_id, '') as source_message_id,
			COALESCE(msg.conversation_id, 0) as conversation_id,
			COALESCE(c.source_conversation_id, '') as source_conversation_id,
			COALESCE(msg.subject, '') as subject,
			COALESCE(msg.snippet, '') as snippet,
			COALESCE(ms.from_email, ds.from_email, '') as from_email,
			COALESCE(ms.from_name, ds.from_name, '') as from_name,
			COALESCE(ms.from_phone, ds.from_phone, '') as from_phone,
			msg.sent_at,
			COALESCE(msg.size_estimate, 0) as size_estimate,
			COALESCE(msg.has_attachments, false) as has_attachments,
			COALESCE(msg.attachment_count, 0) as attachment_count,
			msg.deleted_from_source_at,
			COALESCE(msg.message_type, '') as message_type,
			COALESCE(c.title, '') as conv_title
		FROM msg
		JOIN filtered_msgs fm ON fm.id = msg.id
		LEFT JOIN msg_sender ms ON ms.message_id = msg.id
		LEFT JOIN direct_sender ds ON ds.message_id = msg.id
		LEFT JOIN conv c ON c.id = msg.conversation_id
		ORDER BY %s
	`, e.parquetCTEs(), where, orderBy, orderBy)

	args = append(args, limit, filter.Pagination.Offset)

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []MessageSummary
	for rows.Next() {
		var msg MessageSummary
		var sentAt sql.NullTime
		var deletedAt sql.NullTime
		if err := rows.Scan(
			&msg.ID,
			&msg.SourceMessageID,
			&msg.ConversationID,
			&msg.SourceConversationID,
			&msg.Subject,
			&msg.Snippet,
			&msg.FromEmail,
			&msg.FromName,
			&msg.FromPhone,
			&sentAt,
			&msg.SizeEstimate,
			&msg.HasAttachments,
			&msg.AttachmentCount,
			&deletedAt,
			&msg.MessageType,
			&msg.ConversationTitle,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		if sentAt.Valid {
			msg.SentAt = sentAt.Time
		}
		if deletedAt.Valid {
			msg.DeletedAt = &deletedAt.Time
		}
		results = append(results, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return results, nil
}

// parseLabelsJSON parses JSON array format into string slice.
// We use to_json(labels) in the SQL query to get proper JSON encoding,
// which handles commas, quotes, and special characters in label names.
func parseLabelsJSON(s string) []string {
	if s == "" || s == "[]" || s == "null" {
		return nil
	}
	var labels []string
	if err := json.Unmarshal([]byte(s), &labels); err != nil {
		// Fallback: if JSON parsing fails, return empty
		return nil
	}
	return labels
}

// fetchLabelsForMessages adds labels to message summaries.
// Uses DuckDB's sqlite_scanner when available, otherwise direct SQLite.
func (e *DuckDBEngine) fetchLabelsForMessages(ctx context.Context, messages []MessageSummary) error {
	if len(messages) == 0 {
		return nil
	}

	// Prefer direct SQLite (works on all platforms including Windows)
	if e.sqliteEngine != nil {
		return e.sqliteEngine.fetchLabelsForMessages(ctx, messages)
	}

	if !e.hasSQLite() {
		log.Printf("[warn] fetchLabelsForMessages: no label source available (sqliteEngine=nil, hasSQLiteScanner=false); labels will be empty")
		return nil
	}

	return fetchLabelsForMessageList(ctx, e.db, "sqlite_db.", messages)
}

// GetMessageSummariesByIDs delegates to the SQLite engine — the
// summary lookup is a small handful of indexed queries that gain
// nothing from going through Parquet, and SQLite's plan keeps the
// caller-supplied id order intact.
func (e *DuckDBEngine) GetMessageSummariesByIDs(ctx context.Context, ids []int64) ([]MessageSummary, error) {
	if e.sqliteEngine == nil {
		return nil, fmt.Errorf("GetMessageSummariesByIDs requires SQLite: pass sqlitePath to NewDuckDBEngine")
	}
	return e.sqliteEngine.GetMessageSummariesByIDs(ctx, ids)
}

// GetMessage retrieves a full message from SQLite.
// Uses direct SQLite connection when available for better BLOB handling.
func (e *DuckDBEngine) GetMessage(ctx context.Context, id int64) (*MessageDetail, error) {
	// Prefer direct SQLite for body/BLOB retrieval
	if e.sqliteEngine != nil {
		return e.sqliteEngine.GetMessage(ctx, id)
	}

	// Fall back to sqlite_scan
	if !e.hasSQLite() {
		return nil, fmt.Errorf("GetMessage requires SQLite: pass sqlitePath to NewDuckDBEngine")
	}

	return e.getMessageByQuery(ctx, "m.id = ?", id)
}

// GetMessageBySourceID retrieves a message by source ID from SQLite.
// Uses direct SQLite connection when available for better BLOB handling.
func (e *DuckDBEngine) GetMessageBySourceID(ctx context.Context, sourceMessageID string) (*MessageDetail, error) {
	// Prefer direct SQLite for body/BLOB retrieval
	if e.sqliteEngine != nil {
		return e.sqliteEngine.GetMessageBySourceID(ctx, sourceMessageID)
	}

	// Fall back to sqlite_scan
	if !e.hasSQLite() {
		return nil, fmt.Errorf("GetMessageBySourceID requires SQLite: pass sqlitePath to NewDuckDBEngine")
	}

	return e.getMessageByQuery(ctx, "m.source_message_id = ?", sourceMessageID)
}

// GetAttachment retrieves attachment metadata by ID.
// Attachments live in SQLite, so delegate to the SQLite engine.
func (e *DuckDBEngine) GetAttachment(ctx context.Context, id int64) (*AttachmentInfo, error) {
	if e.sqliteEngine != nil {
		return e.sqliteEngine.GetAttachment(ctx, id)
	}
	return nil, fmt.Errorf("GetAttachment requires SQLite: pass sqliteDB to NewDuckDBEngine")
}

func (e *DuckDBEngine) getMessageByQuery(ctx context.Context, whereClause string, args ...interface{}) (*MessageDetail, error) {
	return getMessageByQueryShared(ctx, e.db, "sqlite_db.", whereClause, args...)
}

// GetGmailIDsByFilter returns Gmail IDs matching a filter.
// This method delegates to SQLite for authoritative deletion status.
// The Parquet cache may be stale if messages were deleted after the last cache build,
// so we use SQLite directly to ensure deleted messages are properly excluded.
func (e *DuckDBEngine) GetGmailIDsByFilter(ctx context.Context, filter MessageFilter) ([]string, error) {
	// Delegate to SQLite for authoritative deletion status.
	// Parquet cache may be stale if deletions occurred after the last build.
	if e.sqliteEngine != nil {
		return e.sqliteEngine.GetGmailIDsByFilter(ctx, filter)
	}

	// Fall back to Parquet if no SQLite engine available (shouldn't happen in practice)
	if e.analyticsDir == "" {
		return nil, fmt.Errorf("GetGmailIDsByFilter requires SQLite or Parquet data")
	}

	var conditions []string
	var args []interface{}

	// Always exclude deleted messages
	conditions = append(conditions, "msg.deleted_from_source_at IS NULL")

	// Gmail scoping is handled by JOIN src in the query below — this function
	// is used for Gmail-specific deletion/staging workflows and must not
	// return WhatsApp or other source IDs.

	if filter.SourceID != nil {
		conditions = append(conditions, "msg.source_id = ?")
		args = append(args, *filter.SourceID)
	}

	// Use EXISTS subqueries for filtering (becomes semi-joins, no duplicates)
	if filter.Sender != "" {
		conditions = append(conditions, `(EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND (p.email_address = ? OR p.phone_number = ?)
		) OR EXISTS (
			SELECT 1 FROM p
			WHERE p.id = msg.sender_id
			  AND (p.email_address = ? OR p.phone_number = ?)
		))`)
		args = append(args, filter.Sender, filter.Sender, filter.Sender, filter.Sender)
	}

	if filter.SenderName != "" {
		conditions = append(conditions, `(EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) = ?
		) OR EXISTS (
			SELECT 1 FROM p
			WHERE p.id = msg.sender_id
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) = ?
		))`)
		args = append(args, filter.SenderName, filter.SenderName)
	}

	if filter.Recipient != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type IN ('to', 'cc', 'bcc')
			  AND p.email_address = ?
		)`)
		args = append(args, filter.Recipient)
	}

	if filter.RecipientName != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type IN ('to', 'cc', 'bcc')
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) = ?
		)`)
		args = append(args, filter.RecipientName)
	}

	if filter.Domain != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND p.domain = ?
		)`)
		args = append(args, filter.Domain)
	}

	if filter.Label != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM ml
			JOIN lbl ON lbl.id = ml.label_id
			WHERE ml.message_id = msg.id
			  AND lbl.name ILIKE ? ESCAPE '\'
		)`)
		args = append(args, escapeILIKE(filter.Label))
	}

	if filter.TimeRange.Period != "" {
		granularity := inferTimeGranularity(filter.TimeRange.Granularity, filter.TimeRange.Period)
		// GetGmailIDsByFilter uses strftime for time filtering (no year/month columns)
		var te string
		switch granularity {
		case TimeYear:
			te = "strftime(msg.sent_at, '%Y')"
		case TimeDay:
			te = "strftime(msg.sent_at, '%Y-%m-%d')"
		default:
			te = "strftime(msg.sent_at, '%Y-%m')"
		}
		conditions = append(conditions, fmt.Sprintf("%s = ?", te))
		args = append(args, filter.TimeRange.Period)
	}

	// Build query — JOIN src to scope to Gmail sources authoritatively.
	query := fmt.Sprintf(`
		WITH %s
		SELECT msg.source_message_id
		FROM msg
		JOIN src ON src.id = msg.source_id AND COALESCE(src.source_type, 'gmail') = 'gmail'
		WHERE %s
		ORDER BY msg.sent_at DESC, msg.id DESC
	`, e.parquetCTEs(), strings.Join(conditions, " AND "))

	// Only add LIMIT if explicitly set (0 means no limit)
	if filter.Pagination.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Pagination.Limit)
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get gmail ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return collectGmailIDs(rows)
}
