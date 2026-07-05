package query

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// timeExpr returns the SQL expression for time grouping based on granularity.
func timeExpr(g TimeGranularity) string {
	switch g {
	case TimeYear:
		return "CAST(msg.year AS VARCHAR)"
	case TimeDay:
		return "strftime(msg.sent_at, '%Y-%m-%d')"
	default: // TimeMonth
		return "CAST(msg.year AS VARCHAR) || '-' || LPAD(CAST(msg.month AS VARCHAR), 2, '0')"
	}
}

// aggViewDef defines the varying parts of an aggregate query for each view type.
type aggViewDef struct {
	keyExpr    string // SQL expression for the grouping key (e.g. "p.email_address")
	joinClause string // JOIN clause specific to this view
	nullGuard  string // WHERE condition to exclude NULL keys
	// keyColumns for buildWhereClause search filtering (passed through to buildAggregateSearchConditions)
	keyColumns []string
}

// getViewDef returns the aggregate query definition for a given view type.
// The tablePrefix is used to alias tables in SubAggregate to avoid conflicts
// with CTE names used in filter conditions. Pass "" for top-level aggregates.
func getViewDef(view ViewType, granularity TimeGranularity, tablePrefix string) (aggViewDef, error) {
	// Use prefix for table aliases in SubAggregate (e.g. "mr_agg", "p_agg")
	// to avoid ambiguity with CTE names used in WHERE clause EXISTS subqueries.
	mrAlias := "mr"
	pAlias := "p"
	mlAlias := "ml"
	lblAlias := "lbl"
	if tablePrefix != "" {
		mrAlias = "mr_" + tablePrefix
		pAlias = "p_" + tablePrefix
		mlAlias = "ml_" + tablePrefix
		lblAlias = "lbl_" + tablePrefix
	}

	switch view {
	case ViewSenders:
		return aggViewDef{
			keyExpr:    pAlias + ".email_address",
			joinClause: fmt.Sprintf("JOIN mr %s ON %s.message_id = msg.id AND %s.recipient_type = 'from'\n\t\t\t\tJOIN p %s ON %s.id = %s.participant_id", mrAlias, mrAlias, mrAlias, pAlias, pAlias, mrAlias),
			nullGuard:  pAlias + ".email_address IS NOT NULL",
		}, nil
	case ViewSenderNames:
		nameExpr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%s.display_name), ''), %s.email_address)", pAlias, pAlias)
		return aggViewDef{
			keyExpr:    nameExpr,
			joinClause: fmt.Sprintf("JOIN mr %s ON %s.message_id = msg.id AND %s.recipient_type = 'from'\n\t\t\t\tJOIN p %s ON %s.id = %s.participant_id", mrAlias, mrAlias, mrAlias, pAlias, pAlias, mrAlias),
			nullGuard:  nameExpr + " IS NOT NULL",
		}, nil
	case ViewRecipients:
		return aggViewDef{
			keyExpr:    pAlias + ".email_address",
			joinClause: fmt.Sprintf("JOIN mr %s ON %s.message_id = msg.id AND %s.recipient_type IN ('to', 'cc', 'bcc')\n\t\t\t\tJOIN p %s ON %s.id = %s.participant_id", mrAlias, mrAlias, mrAlias, pAlias, pAlias, mrAlias),
			nullGuard:  pAlias + ".email_address IS NOT NULL",
			keyColumns: []string{pAlias + ".email_address", pAlias + ".display_name"},
		}, nil
	case ViewRecipientNames:
		nameExpr := fmt.Sprintf("COALESCE(NULLIF(TRIM(%s.display_name), ''), %s.email_address)", pAlias, pAlias)
		return aggViewDef{
			keyExpr:    nameExpr,
			joinClause: fmt.Sprintf("JOIN mr %s ON %s.message_id = msg.id AND %s.recipient_type IN ('to', 'cc', 'bcc')\n\t\t\t\tJOIN p %s ON %s.id = %s.participant_id", mrAlias, mrAlias, mrAlias, pAlias, pAlias, mrAlias),
			nullGuard:  nameExpr + " IS NOT NULL",
			keyColumns: []string{pAlias + ".email_address", pAlias + ".display_name"},
		}, nil
	case ViewDomains:
		return aggViewDef{
			keyExpr:    pAlias + ".domain",
			joinClause: fmt.Sprintf("JOIN mr %s ON %s.message_id = msg.id AND %s.recipient_type = 'from'\n\t\t\t\tJOIN p %s ON %s.id = %s.participant_id", mrAlias, mrAlias, mrAlias, pAlias, pAlias, mrAlias),
			nullGuard:  pAlias + ".domain IS NOT NULL AND " + pAlias + ".domain != ''",
		}, nil
	case ViewLabels:
		return aggViewDef{
			keyExpr:    lblAlias + ".name",
			joinClause: fmt.Sprintf("JOIN ml %s ON %s.message_id = msg.id\n\t\t\t\tJOIN lbl %s ON %s.id = %s.label_id", mlAlias, mlAlias, lblAlias, lblAlias, mlAlias),
			nullGuard:  lblAlias + ".name IS NOT NULL",
			keyColumns: []string{lblAlias + ".name"},
		}, nil
	case ViewTime:
		return aggViewDef{
			keyExpr:   timeExpr(granularity),
			nullGuard: "msg.sent_at IS NOT NULL",
		}, nil
	default:
		return aggViewDef{}, fmt.Errorf("unsupported view type: %v", view)
	}
}

// runAggregation executes a generic aggregation query using the view definition.
func (e *DuckDBEngine) runAggregation(ctx context.Context, def aggViewDef, whereClause string, args []interface{}, opts AggregateOptions) ([]AggregateRow, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}

	fullWhere := whereClause
	if def.nullGuard != "" {
		fullWhere += " AND " + def.nullGuard
	}

	query := fmt.Sprintf(`
		WITH %s
		SELECT key, count, total_size, attachment_size, attachment_count, total_unique
		FROM (
			SELECT
				%s as key,
				COUNT(*) as count,
				COALESCE(SUM(CAST(msg.size_estimate AS BIGINT)), 0) as total_size,
				CAST(COALESCE(SUM(att.attachment_size), 0) AS BIGINT) as attachment_size,
				CAST(COALESCE(SUM(att.attachment_count), 0) AS BIGINT) as attachment_count,
				COUNT(*) OVER() as total_unique
			FROM msg
			%s
			LEFT JOIN att ON att.message_id = msg.id
			WHERE %s
			GROUP BY %s
		)
		%s
		LIMIT ?
	`, e.parquetCTEs(), def.keyExpr, def.joinClause, fullWhere, def.keyExpr, e.sortClause(opts))

	args = append(args, limit)
	return e.executeAggregateQuery(ctx, query, args)
}

// sortClause returns ORDER BY clause for aggregates.
func (e *DuckDBEngine) sortClause(opts AggregateOptions) string {
	field := "count"
	switch opts.SortField {
	case SortBySize:
		field = "total_size"
	case SortByAttachmentSize:
		field = "attachment_size"
	case SortByName:
		field = "key"
	}

	dir := "DESC"
	if opts.SortDirection == SortAsc {
		dir = "ASC"
	}

	return fmt.Sprintf("ORDER BY %s %s", field, dir)
}

// aggregateByView is the generic implementation for all AggregateBy* methods.
func (e *DuckDBEngine) aggregateByView(ctx context.Context, view ViewType, opts AggregateOptions) ([]AggregateRow, error) {
	def, err := getViewDef(view, opts.TimeGranularity, "")
	if err != nil {
		return nil, err
	}
	where, args := e.buildWhereClause(opts, def.keyColumns...)
	return e.runAggregation(ctx, def, where, args, opts)
}

// Aggregate performs grouping based on the provided ViewType.
// ViewAICategories falls back to the SQLite engine because the Parquet labels
// table does not include label_type, so filtering by label_type = 'auto'
// requires the SQLite store. AI category counts are always small (≤8 rows),
// so SQLite performance is acceptable.
func (e *DuckDBEngine) Aggregate(ctx context.Context, groupBy ViewType, opts AggregateOptions) ([]AggregateRow, error) {
	if groupBy == ViewAICategories && e.sqliteEngine != nil {
		return e.sqliteEngine.Aggregate(ctx, groupBy, opts)
	}
	return e.aggregateByView(ctx, groupBy, opts)
}

// inferTimeGranularity adjusts the granularity based on the time period string length.
func inferTimeGranularity(base TimeGranularity, period string) TimeGranularity {
	if base == TimeYear && len(period) > 4 {
		switch len(period) {
		case 7:
			return TimeMonth
		case 10:
			return TimeDay
		}
	}
	return base
}

// SubAggregate performs aggregation on a filtered subset of messages.
// This is used for sub-grouping after drill-down.
// ViewAICategories falls back to the SQLite engine (same reason as Aggregate).
func (e *DuckDBEngine) SubAggregate(ctx context.Context, filter MessageFilter, groupBy ViewType, opts AggregateOptions) ([]AggregateRow, error) {
	if groupBy == ViewAICategories && e.sqliteEngine != nil {
		return e.sqliteEngine.SubAggregate(ctx, filter, groupBy, opts)
	}
	def, err := getViewDef(groupBy, opts.TimeGranularity, "agg")
	if err != nil {
		return nil, err
	}

	where, args := e.buildFilterConditions(filter)

	// Add opts-based conditions (source_id, date range, attachment filter)
	if opts.SourceID != nil {
		where += " AND msg.source_id = ?"
		args = append(args, *opts.SourceID)
	}
	if opts.After != nil {
		where += " AND msg.sent_at >= CAST(? AS TIMESTAMP)"
		args = append(args, opts.After.Format("2006-01-02 15:04:05"))
	}
	if opts.Before != nil {
		where += " AND msg.sent_at < CAST(? AS TIMESTAMP)"
		args = append(args, opts.Before.Format("2006-01-02 15:04:05"))
	}
	if opts.WithAttachmentsOnly {
		where += " AND msg.has_attachments = true"
	}
	if opts.HideDeletedFromSource {
		where += " AND msg.deleted_from_source_at IS NULL"
	}

	// Add search query conditions using the view's key columns
	searchConds, searchArgs := e.buildAggregateSearchConditions(opts.SearchQuery, def.keyColumns...)
	for _, cond := range searchConds {
		where += " AND " + cond
	}
	args = append(args, searchArgs...)

	return e.runAggregation(ctx, def, where, args, opts)
}

// executeAggregateQuery runs an aggregate query and returns the results.
// Expects 6 columns: key, count, total_size, attachment_size, attachment_count, total_unique
func (e *DuckDBEngine) executeAggregateQuery(ctx context.Context, query string, args []interface{}) ([]AggregateRow, error) {
	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("aggregate query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []AggregateRow
	for rows.Next() {
		var row AggregateRow
		// SQL uses CAST(... AS BIGINT) so we can scan directly into int64
		var attachmentSize sql.NullInt64
		var attachmentCount sql.NullInt64
		if err := rows.Scan(&row.Key, &row.Count, &row.TotalSize, &attachmentSize, &attachmentCount, &row.TotalUnique); err != nil {
			return nil, fmt.Errorf("scan aggregate row: %w", err)
		}
		if attachmentSize.Valid {
			row.AttachmentSize = attachmentSize.Int64
		}
		if attachmentCount.Valid {
			row.AttachmentCount = attachmentCount.Int64
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aggregate rows: %w", err)
	}

	return results, nil
}

// GetTotalStats returns overall statistics from Parquet.
func (e *DuckDBEngine) GetTotalStats(ctx context.Context, opts StatsOptions) (*TotalStats, error) {
	stats := &TotalStats{}

	var conditions []string
	var args []interface{}

	// Restrict to email messages only; NULL and '' handle pre-message_type data.
	conditions = append(conditions, emailOnlyFilterMsg)

	if opts.SourceID != nil {
		conditions = append(conditions, "msg.source_id = ?")
		args = append(args, *opts.SourceID)
	}

	if opts.WithAttachmentsOnly {
		conditions = append(conditions, "msg.has_attachments = 1")
	}
	if opts.HideDeletedFromSource {
		conditions = append(conditions, "msg.deleted_from_source_at IS NULL")
	}

	// Search filter — uses EXISTS subqueries so no row multiplication.
	// For 1:N views (Recipients, RecipientNames, Labels), filter on the
	// grouping key columns so stats match the visible aggregate rows.
	if opts.SearchQuery != "" {
		searchConds, searchArgs := e.buildStatsSearchConditions(opts.SearchQuery, opts.GroupBy)
		conditions = append(conditions, searchConds...)
		args = append(args, searchArgs...)
	}

	whereClause := "1=1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}

	// Message stats - join with attachment aggregates
	msgQuery := fmt.Sprintf(`
		WITH %s
		SELECT
			COUNT(*) as message_count,
			COALESCE(SUM(CAST(msg.size_estimate AS BIGINT)), 0) as total_size,
			CAST(COALESCE(SUM(att.attachment_count), 0) AS BIGINT) as attachment_count,
			CAST(COALESCE(SUM(att.attachment_size), 0) AS BIGINT) as attachment_size,
			COUNT(DISTINCT msg.source_id) as account_count
		FROM msg
		LEFT JOIN att ON att.message_id = msg.id
		WHERE %s
	`, e.parquetCTEs(), whereClause)

	var attachmentSize sql.NullFloat64
	err := e.db.QueryRowContext(ctx, msgQuery, args...).Scan(
		&stats.MessageCount,
		&stats.TotalSize,
		&stats.AttachmentCount,
		&attachmentSize,
		&stats.AccountCount,
	)
	if err != nil {
		return nil, fmt.Errorf("stats query: %w", err)
	}

	if attachmentSize.Valid {
		stats.AttachmentSize = int64(attachmentSize.Float64)
	}

	// Label count from joined tables
	labelQuery := fmt.Sprintf(`
		WITH %s
		SELECT COUNT(DISTINCT lbl.name)
		FROM msg
		JOIN ml ON ml.message_id = msg.id
		JOIN lbl ON lbl.id = ml.label_id
		WHERE %s
	`, e.parquetCTEs(), whereClause)

	if err := e.db.QueryRowContext(ctx, labelQuery, args...).Scan(&stats.LabelCount); err != nil {
		// Non-fatal: label count is informational, but log for debugging
		log.Printf("warning: label count query failed (using 0): %v", err)
		stats.LabelCount = 0
	}

	return stats, nil
}

// ListAccounts returns accounts from SQLite via DuckDB's sqlite_scan,
// or via direct SQLite connection on platforms without sqlite_scanner.
func (e *DuckDBEngine) ListAccounts(ctx context.Context) ([]AccountInfo, error) {
	if e.sqliteEngine != nil {
		return e.sqliteEngine.ListAccounts(ctx)
	}
	if !e.hasSQLite() {
		return nil, fmt.Errorf("ListAccounts requires SQLite: pass sqlitePath to NewDuckDBEngine")
	}

	rows, err := e.db.QueryContext(ctx, `
		SELECT id, source_type, identifier, COALESCE(display_name, '')
		FROM sqlite_db.sources
		ORDER BY identifier
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var accounts []AccountInfo
	for rows.Next() {
		var acc AccountInfo
		if err := rows.Scan(&acc.ID, &acc.SourceType, &acc.Identifier, &acc.DisplayName); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}
