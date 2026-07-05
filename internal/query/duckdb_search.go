package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/wesm/msgvault/internal/search"
)

// Search performs a Gmail-style search query.
// Uses direct SQLite connection for FTS5 support when available,
// falls back to LIKE queries via sqlite_scan otherwise.
func (e *DuckDBEngine) Search(ctx context.Context, q *search.Query, limit, offset int) ([]MessageSummary, error) {
	// Prefer direct SQLite for FTS5 support
	if e.sqliteEngine != nil {
		return e.sqliteEngine.Search(ctx, q, limit, offset)
	}

	// Fall back to sqlite_scan with LIKE queries (no FTS)
	if !e.hasSQLite() {
		return nil, fmt.Errorf("Search requires SQLite: pass sqlitePath to NewDuckDBEngine")
	}

	var conditions []string
	var args []interface{}
	var joins []string

	// Include all messages (deleted messages shown with indicator in TUI)

	// From filter
	if len(q.FromAddrs) > 0 {
		joins = append(joins, `
			JOIN sqlite_db.message_recipients mr_from ON mr_from.message_id = m.id AND mr_from.recipient_type = 'from'
			JOIN sqlite_db.participants p_from ON p_from.id = mr_from.participant_id
		`)
		placeholders := make([]string, len(q.FromAddrs))
		for i, addr := range q.FromAddrs {
			placeholders[i] = "?"
			args = append(args, addr)
		}
		conditions = append(conditions, fmt.Sprintf("LOWER(p_from.email_address) IN (%s)", strings.Join(placeholders, ",")))
	}

	// To filter
	if len(q.ToAddrs) > 0 {
		joins = append(joins, `
			JOIN sqlite_db.message_recipients mr_to ON mr_to.message_id = m.id AND mr_to.recipient_type = 'to'
			JOIN sqlite_db.participants p_to ON p_to.id = mr_to.participant_id
		`)
		placeholders := make([]string, len(q.ToAddrs))
		for i, addr := range q.ToAddrs {
			placeholders[i] = "?"
			args = append(args, addr)
		}
		conditions = append(conditions, fmt.Sprintf("LOWER(p_to.email_address) IN (%s)", strings.Join(placeholders, ",")))
	}

	// Label filter
	if len(q.Labels) > 0 {
		joins = append(joins, `
			JOIN sqlite_db.message_labels ml ON ml.message_id = m.id
			JOIN sqlite_db.labels l ON l.id = ml.label_id
		`)
		placeholders := make([]string, len(q.Labels))
		for i, label := range q.Labels {
			placeholders[i] = "?"
			args = append(args, label)
		}
		conditions = append(conditions, fmt.Sprintf("l.name IN (%s)", strings.Join(placeholders, ",")))
	}

	// Subject filter (case-insensitive with ILIKE)
	if len(q.SubjectTerms) > 0 {
		for _, term := range q.SubjectTerms {
			conditions = append(conditions, "m.subject ILIKE ?")
			args = append(args, "%"+term+"%")
		}
	}

	// Has attachment filter
	if q.HasAttachment != nil && *q.HasAttachment {
		conditions = append(conditions, "m.has_attachments = 1")
	}

	// Date range filters
	if q.AfterDate != nil {
		conditions = append(conditions, "m.sent_at >= CAST(? AS TIMESTAMP)")
		args = append(args, q.AfterDate.Format("2006-01-02 15:04:05"))
	}
	if q.BeforeDate != nil {
		conditions = append(conditions, "m.sent_at < CAST(? AS TIMESTAMP)")
		args = append(args, q.BeforeDate.Format("2006-01-02 15:04:05"))
	}

	// Size filters
	if q.LargerThan != nil {
		conditions = append(conditions, "m.size_estimate > ?")
		args = append(args, *q.LargerThan)
	}
	if q.SmallerThan != nil {
		conditions = append(conditions, "m.size_estimate < ?")
		args = append(args, *q.SmallerThan)
	}

	// Full-text search: use ILIKE fallback (FTS5 not available via sqlite_scan)
	// Only search subject/snippet; body is in separate table, use FTS for body search
	if len(q.TextTerms) > 0 {
		for _, term := range q.TextTerms {
			likeTerm := "%" + term + "%"
			conditions = append(conditions, "(m.subject ILIKE ? OR m.snippet ILIKE ?)")
			args = append(args, likeTerm, likeTerm)
		}
	}

	// Account filter
	if q.AccountID != nil {
		conditions = append(conditions, "m.source_id = ?")
		args = append(args, *q.AccountID)
	}

	// Hide-deleted filter
	if q.HideDeleted {
		conditions = append(conditions, "m.deleted_from_source_at IS NULL")
	}

	if limit == 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT
			m.id,
			m.source_message_id,
			m.conversation_id,
			COALESCE(conv.source_conversation_id, ''),
			COALESCE(m.subject, ''),
			COALESCE(m.snippet, ''),
			COALESCE(p_sender.email_address, ''),
			COALESCE(p_sender.display_name, ''),
			m.sent_at,
			COALESCE(m.size_estimate, 0),
			m.has_attachments,
			m.attachment_count,
			m.deleted_from_source_at
		FROM sqlite_db.messages m
		LEFT JOIN sqlite_db.message_recipients mr_sender ON mr_sender.message_id = m.id AND mr_sender.recipient_type = 'from'
		LEFT JOIN sqlite_db.participants p_sender ON p_sender.id = mr_sender.participant_id
		LEFT JOIN sqlite_db.conversations conv ON conv.id = m.conversation_id
		%s
		WHERE %s
		ORDER BY m.sent_at DESC
		LIMIT ? OFFSET ?
	`, strings.Join(joins, "\n"), strings.Join(conditions, " AND "))

	args = append(args, limit, offset)

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
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
			&sentAt,
			&msg.SizeEstimate,
			&msg.HasAttachments,
			&msg.AttachmentCount,
			&deletedAt,
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

	// Fetch labels for results
	if len(results) > 0 {
		if err := e.fetchLabelsForMessages(ctx, results); err != nil {
			return nil, fmt.Errorf("fetch labels: %w", err)
		}
	}

	return results, nil
}

// SearchFast searches message metadata in Parquet files (no body text).
// This is much faster than FTS search for large archives.
// Searches: subject, sender email/name (case-insensitive).
func (e *DuckDBEngine) SearchFast(ctx context.Context, q *search.Query, filter MessageFilter, limit, offset int) ([]MessageSummary, error) {
	conditions, args := e.buildSearchConditions(q, filter)

	if limit == 0 {
		limit = 100
	}

	// Query with JOINs to reconstruct denormalized view
	query := fmt.Sprintf(`
		WITH %s,
		msg_labels AS (
			SELECT ml.message_id, LIST(lbl.name ORDER BY lbl.name) as labels
			FROM ml
			JOIN lbl ON lbl.id = ml.label_id
			GROUP BY ml.message_id
		),
		msg_sender AS (
			SELECT mr.message_id,
				   FIRST(p.email_address) as from_email,
				   FIRST(COALESCE(mr.display_name, p.display_name, '')) as from_name,
				   FIRST(COALESCE(p.phone_number, '')) as from_phone
			FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.recipient_type = 'from'
			GROUP BY mr.message_id
		),
		direct_sender AS (
			SELECT msg.id as message_id,
				   COALESCE(p.email_address, '') as from_email,
				   COALESCE(p.display_name, '') as from_name,
				   COALESCE(p.phone_number, '') as from_phone
			FROM msg
			JOIN p ON p.id = msg.sender_id
			WHERE msg.sender_id IS NOT NULL
			  AND msg.id NOT IN (SELECT message_id FROM msg_sender)
		)
		SELECT
			COALESCE(msg.id, 0) as id,
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
			COALESCE(att.attachment_count, 0) as attachment_count,
			CAST(COALESCE(to_json(mlbl.labels), '[]') AS VARCHAR) as labels,
			msg.deleted_from_source_at,
			COALESCE(msg.message_type, '') as message_type,
			COALESCE(c.title, '') as conv_title
		FROM msg
		LEFT JOIN msg_sender ms ON ms.message_id = msg.id
		LEFT JOIN direct_sender ds ON ds.message_id = msg.id
		LEFT JOIN att ON att.message_id = msg.id
		LEFT JOIN msg_labels mlbl ON mlbl.message_id = msg.id
		LEFT JOIN conv c ON c.id = msg.conversation_id
		WHERE %s
		ORDER BY msg.sent_at DESC
		LIMIT ? OFFSET ?
	`, e.parquetCTEs(), strings.Join(conditions, " AND "))

	args = append(args, limit, offset)

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search fast: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []MessageSummary
	for rows.Next() {
		var msg MessageSummary
		var sentAt sql.NullTime
		var deletedAt sql.NullTime
		var labelsJSON string
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
			&labelsJSON,
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
		// Parse labels from JSON array format
		msg.Labels = parseLabelsJSON(labelsJSON)
		results = append(results, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return results, nil
}

// SearchFastCount returns the total count of messages matching a search query.
// This is used for pagination UI to show "N of M results".
func (e *DuckDBEngine) SearchFastCount(ctx context.Context, q *search.Query, filter MessageFilter) (int64, error) {
	conditions, args := e.buildSearchConditions(q, filter)

	// Count with JOINs for filters that need them
	query := fmt.Sprintf(`
		WITH %s,
		msg_sender AS (
			SELECT mr.message_id,
				   FIRST(p.email_address) as from_email,
				   FIRST(COALESCE(mr.display_name, p.display_name, '')) as from_name,
				   FIRST(COALESCE(p.phone_number, '')) as from_phone
			FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.recipient_type = 'from'
			GROUP BY mr.message_id
		),
		direct_sender AS (
			SELECT msg.id as message_id,
				   COALESCE(p.email_address, '') as from_email,
				   COALESCE(p.display_name, '') as from_name,
				   COALESCE(p.phone_number, '') as from_phone
			FROM msg
			JOIN p ON p.id = msg.sender_id
			WHERE msg.sender_id IS NOT NULL
			  AND msg.id NOT IN (SELECT message_id FROM msg_sender)
		)
		SELECT COUNT(*) as cnt
		FROM msg
		LEFT JOIN msg_sender ms ON ms.message_id = msg.id
		LEFT JOIN direct_sender ds ON ds.message_id = msg.id
		WHERE %s
	`, e.parquetCTEs(), strings.Join(conditions, " AND "))

	var count int64
	if err := e.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("search fast count: %w", err)
	}
	return count, nil
}

// searchCacheKeyFor builds a deterministic cache key from search conditions and args.
// Same query+filter always produces the same key. Uses JSON encoding to avoid
// ambiguity from delimiter collisions (e.g. args containing commas or pipes).
func searchCacheKeyFor(conditions []string, args []interface{}) string {
	// JSON marshaling is unambiguous: each element is quoted/escaped independently.
	// Errors are impossible for string/int/float/bool args, but fall back to fmt.
	key := struct {
		C []string      `json:"c"`
		A []interface{} `json:"a"`
	}{conditions, args}
	b, err := json.Marshal(key)
	if err != nil {
		// Fallback: should never happen with the types buildSearchConditions produces.
		return fmt.Sprintf("%v#%v", conditions, args)
	}
	return string(b)
}

// dropSearchCache drops the cached temp table and clears all cache fields.
// Uses context.Background() so cleanup succeeds even if the caller's context
// is canceled (avoiding leaked temp tables on the single DuckDB connection).
// Caller must hold e.searchCacheMu.
func (e *DuckDBEngine) dropSearchCache() {
	if e.searchCacheTable != "" {
		_, _ = e.db.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", e.searchCacheTable))
	}
	e.searchCacheKey = ""
	e.searchCacheTable = ""
	e.searchCacheCount = 0
	e.searchCacheStats = nil
}

// searchPageFromCache executes Phase 3 (paginated results) from the cached temp table.
// Returns a SearchFastResult with cached count and stats.
func (e *DuckDBEngine) searchPageFromCache(ctx context.Context, limit, offset int) (*SearchFastResult, error) {
	pageQuery := fmt.Sprintf(`
		WITH %s,
		page AS (
			SELECT sm.id FROM %s sm
			ORDER BY sm.sent_at DESC
			LIMIT ? OFFSET ?
		),
		msg_labels AS (
			SELECT ml.message_id, LIST(lbl.name ORDER BY lbl.name) as labels
			FROM ml
			JOIN lbl ON lbl.id = ml.label_id
			WHERE ml.message_id IN (SELECT id FROM page)
			GROUP BY ml.message_id
		)
		SELECT
			sm.id,
			sm.source_message_id,
			sm.conversation_id,
			COALESCE(c.source_conversation_id, '') as source_conversation_id,
			sm.subject,
			sm.snippet,
			sm.from_email,
			sm.from_name,
			COALESCE(sm.from_phone, '') as from_phone,
			sm.sent_at,
			sm.size_estimate,
			sm.has_attachments,
			COALESCE(att.attachment_count, 0) as attachment_count,
			CAST(COALESCE(to_json(mlbl.labels), '[]') AS VARCHAR) as labels,
			sm.deleted_from_source_at,
			COALESCE(sm.message_type, '') as message_type,
			COALESCE(c.title, '') as conv_title
		FROM %s sm
		JOIN page p ON p.id = sm.id
		LEFT JOIN att ON att.message_id = sm.id
		LEFT JOIN msg_labels mlbl ON mlbl.message_id = sm.id
		LEFT JOIN conv c ON c.id = sm.conversation_id
		ORDER BY sm.sent_at DESC
	`, e.parquetCTEs(), e.searchCacheTable, e.searchCacheTable)

	rows, err := e.db.QueryContext(ctx, pageQuery, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search fast page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Return a copy of cached stats to prevent callers from mutating the cache
	var statsCopy *TotalStats
	if e.searchCacheStats != nil {
		tmp := *e.searchCacheStats
		statsCopy = &tmp
	}

	result := &SearchFastResult{
		TotalCount: e.searchCacheCount,
		Stats:      statsCopy,
	}

	for rows.Next() {
		var msg MessageSummary
		var sentAt sql.NullTime
		var deletedAt sql.NullTime
		var labelsJSON string
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
			&labelsJSON,
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
		msg.Labels = parseLabelsJSON(labelsJSON)
		result.Messages = append(result.Messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return result, nil
}

// computeSearchStats computes stats (Phase 4) from the cached temp table.
// Returns nil stats on failure (best-effort).
func (e *DuckDBEngine) computeSearchStats(ctx context.Context) *TotalStats {
	msgStatsQuery := fmt.Sprintf(`
		WITH %s
		SELECT
			COUNT(*) as message_count,
			COALESCE(SUM(sm.size_estimate), 0) as total_size,
			CAST(COALESCE(SUM(att.attachment_count), 0) AS BIGINT) as attachment_count,
			CAST(COALESCE(SUM(att.attachment_size), 0) AS BIGINT) as attachment_size,
			COUNT(DISTINCT sm.source_id) as account_count
		FROM %s sm
		LEFT JOIN att ON att.message_id = sm.id
	`, e.parquetCTEs(), e.searchCacheTable)

	stats := &TotalStats{}
	var attachmentSize sql.NullFloat64
	if err := e.db.QueryRowContext(ctx, msgStatsQuery).Scan(
		&stats.MessageCount,
		&stats.TotalSize,
		&stats.AttachmentCount,
		&attachmentSize,
		&stats.AccountCount,
	); err != nil {
		log.Printf("warning: search stats query failed (stats will be nil): %v", err)
		return nil
	}
	if attachmentSize.Valid {
		stats.AttachmentSize = int64(attachmentSize.Float64)
	}

	// Label count — only ml/lbl Parquet tables needed.
	labelStatsQuery := fmt.Sprintf(`
		WITH %s
		SELECT COUNT(DISTINCT lbl.name)
		FROM %s sm
		JOIN ml ON ml.message_id = sm.id
		JOIN lbl ON lbl.id = ml.label_id
	`, e.parquetCTEs(), e.searchCacheTable)

	if err := e.db.QueryRowContext(ctx, labelStatsQuery).Scan(&stats.LabelCount); err != nil {
		log.Printf("warning: search label count query failed (using 0): %v", err)
		stats.LabelCount = 0
	}

	return stats
}

// SearchFastWithStats performs a single-scan fast search with temp table materialization.
// It denormalizes matching messages (with sender info) into a temp table using one
// Parquet scan, then reuses the in-memory temp table for count, pagination, and stats
// — eliminating all subsequent msg Parquet reads. Only small page-scoped lookups
// into label/attachment Parquet tables remain.
//
// The temp table is cached internally: if the same search conditions+args are
// requested again (e.g. pagination), the Parquet scan is skipped and the page
// is served directly from the cached temp table. A new search invalidates the
// old cache.
func (e *DuckDBEngine) SearchFastWithStats(ctx context.Context, q *search.Query, queryStr string,
	filter MessageFilter, statsGroupBy ViewType, limit, offset int) (*SearchFastResult, error) {

	conditions, args := e.buildSearchConditions(q, filter)

	if limit == 0 {
		limit = 100
	}

	e.searchCacheMu.Lock()
	defer e.searchCacheMu.Unlock()

	// Check cache: same conditions+args means same search, serve from cached table.
	cacheKey := searchCacheKeyFor(conditions, args)
	if cacheKey == e.searchCacheKey && e.searchCacheTable != "" {
		// Retry stats if a previous attempt failed (transient error).
		if e.searchCacheStats == nil {
			e.searchCacheStats = e.computeSearchStats(ctx)
		}
		return e.searchPageFromCache(ctx, limit, offset)
	}

	// Cache miss — drop old cache and materialize fresh.
	e.dropSearchCache()

	// Unique temp table name to avoid concurrent collisions.
	seq := e.tempTableSeq.Add(1)
	tempTable := fmt.Sprintf("_search_matches_%d", seq)

	// Phase 1: Materialize matching messages into temp table (single Parquet scan).
	// Stores all columns needed by later phases so they never re-read msg Parquet.
	// The msg_sender CTE is required because buildSearchConditions references ms.from_email.
	materializeQuery := fmt.Sprintf(`
		CREATE TEMP TABLE %s AS
		WITH %s,
		msg_sender AS (
			SELECT mr.message_id,
				   FIRST(p.email_address) as from_email,
				   FIRST(COALESCE(mr.display_name, p.display_name, '')) as from_name,
				   FIRST(COALESCE(p.phone_number, '')) as from_phone
			FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.recipient_type = 'from'
			GROUP BY mr.message_id
		),
		direct_sender AS (
			SELECT msg.id as message_id,
				   COALESCE(p.email_address, '') as from_email,
				   COALESCE(p.display_name, '') as from_name,
				   COALESCE(p.phone_number, '') as from_phone
			FROM msg
			JOIN p ON p.id = msg.sender_id
			WHERE msg.sender_id IS NOT NULL
			  AND msg.id NOT IN (SELECT message_id FROM msg_sender)
		)
		SELECT
			msg.id,
			COALESCE(msg.source_message_id, '') as source_message_id,
			COALESCE(msg.conversation_id, 0) as conversation_id,
			COALESCE(msg.subject, '') as subject,
			COALESCE(msg.snippet, '') as snippet,
			COALESCE(ms.from_email, ds.from_email, '') as from_email,
			COALESCE(ms.from_name, ds.from_name, '') as from_name,
			COALESCE(ms.from_phone, ds.from_phone, '') as from_phone,
			msg.sent_at,
			COALESCE(CAST(msg.size_estimate AS BIGINT), 0) as size_estimate,
			COALESCE(msg.has_attachments, false) as has_attachments,
			msg.deleted_from_source_at,
			CAST(msg.source_id AS BIGINT) as source_id,
			COALESCE(msg.message_type, '') as message_type
		FROM msg
		LEFT JOIN msg_sender ms ON ms.message_id = msg.id
		LEFT JOIN direct_sender ds ON ds.message_id = msg.id
		WHERE %s
	`, tempTable, e.parquetCTEs(), strings.Join(conditions, " AND "))

	if _, err := e.db.ExecContext(ctx, materializeQuery, args...); err != nil {
		return nil, fmt.Errorf("materialize search matches: %w", err)
	}

	// Store temp table name so we can clean up on error.
	e.searchCacheTable = tempTable

	// Phase 2: Count (trivial — reads in-memory temp table only).
	// Best-effort: if count fails, use -1 (unknown total) and continue.
	var count int64
	if err := e.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tempTable)).Scan(&count); err != nil {
		log.Printf("warning: search count query failed (using -1): %v", err)
		count = -1
	}
	e.searchCacheCount = count

	// Phase 4: Stats from temp table (compute before page so cache is fully populated).
	e.searchCacheStats = e.computeSearchStats(ctx)

	// Store cache key — cache is now valid.
	e.searchCacheKey = cacheKey

	// Phase 3: Paginated results from cached temp table.
	return e.searchPageFromCache(ctx, limit, offset)
}
