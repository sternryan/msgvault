package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wesm/msgvault/internal/query"
)

// DuckDBBulkFetcher implements BulkDataFetcher using DuckDB queries over Parquet files.
// This provides 10-100x faster aggregations compared to SQLite by leveraging
// columnar storage and vectorized execution.
type DuckDBBulkFetcher struct {
	engine *query.DuckDBEngine
}

// NewDuckDBBulkFetcher creates a new DuckDB-based bulk data fetcher.
func NewDuckDBBulkFetcher(engine *query.DuckDBEngine) *DuckDBBulkFetcher {
	return &DuckDBBulkFetcher{engine: engine}
}

// FetchAllTopLabelsByPerson fetches top labels for all people using DuckDB's vectorized engine.
func (f *DuckDBBulkFetcher) FetchAllTopLabelsByPerson(ctx context.Context, opts ExportOptions) (map[string][]LabelStat, error) {
	q := `
		WITH person_labels AS (
			SELECT
				p.email_address,
				l.name as label_name,
				COUNT(DISTINCT m.id) as msg_count
			FROM sqlite_db.participants p
			JOIN sqlite_db.message_recipients mr ON mr.participant_id = p.id
			JOIN sqlite_db.messages m ON m.id = mr.message_id
			JOIN sqlite_db.message_labels ml ON ml.message_id = m.id
			JOIN sqlite_db.labels l ON l.id = ml.label_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		q += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		q += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	q += `
			GROUP BY p.email_address, l.id
		),
		ranked AS (
			SELECT email_address, label_name, msg_count,
				ROW_NUMBER() OVER (PARTITION BY email_address ORDER BY msg_count DESC) as rn
			FROM person_labels
		)
		SELECT email_address, label_name, msg_count
		FROM ranked WHERE rn <= 10
		ORDER BY email_address, rn
	`
	rows, err := f.engine.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb fetch top labels by person: %w", err)
	}
	defer rows.Close()

	return scanLabelsByKey(rows)
}

// FetchAllRelatedPeople fetches related people for all contacts using DuckDB.
func (f *DuckDBBulkFetcher) FetchAllRelatedPeople(ctx context.Context, opts ExportOptions) (map[string][]RelatedPerson, error) {
	q := `
		WITH person_relationships AS (
			SELECT
				p1.email_address as person_email,
				p2.email_address as related_email,
				COUNT(DISTINCT m.conversation_id) as shared_threads
			FROM sqlite_db.messages m
			JOIN sqlite_db.message_recipients mr1 ON mr1.message_id = m.id
			JOIN sqlite_db.participants p1 ON p1.id = mr1.participant_id
			JOIN sqlite_db.message_recipients mr2 ON mr2.message_id = m.id
			JOIN sqlite_db.participants p2 ON p2.id = mr2.participant_id
			WHERE p2.email_address != p1.email_address
				AND m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		q += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		q += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	q += `
			GROUP BY p1.id, p2.id
			HAVING shared_threads > 5
		),
		ranked AS (
			SELECT person_email, related_email, shared_threads,
				ROW_NUMBER() OVER (PARTITION BY person_email ORDER BY shared_threads DESC) as rn
			FROM person_relationships
		)
		SELECT person_email, related_email, shared_threads
		FROM ranked WHERE rn <= 10
		ORDER BY person_email, rn
	`
	rows, err := f.engine.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb fetch related people: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]RelatedPerson)
	for rows.Next() {
		var personEmail, relatedEmail string
		var sharedThreads int
		if err := rows.Scan(&personEmail, &relatedEmail, &sharedThreads); err != nil {
			return nil, fmt.Errorf("scan related person: %w", err)
		}
		result[personEmail] = append(result[personEmail], RelatedPerson{
			Email:         relatedEmail,
			SharedThreads: sharedThreads,
		})
	}
	return result, rows.Err()
}

// FetchAllRecentMonthsByPerson fetches recent activity months for all people using DuckDB.
func (f *DuckDBBulkFetcher) FetchAllRecentMonthsByPerson(ctx context.Context, opts ExportOptions) (map[string][]string, error) {
	q := `
		WITH person_months AS (
			SELECT
				p.email_address,
				strftime(m.sent_at, '%Y-%m') as period
			FROM sqlite_db.participants p
			JOIN sqlite_db.message_recipients mr ON mr.participant_id = p.id
			JOIN sqlite_db.messages m ON m.id = mr.message_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		q += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		q += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	q += `
			GROUP BY p.email_address, period
		),
		ranked AS (
			SELECT email_address, period,
				ROW_NUMBER() OVER (PARTITION BY email_address ORDER BY period DESC) as rn
			FROM person_months
		)
		SELECT email_address, period
		FROM ranked WHERE rn <= 12
		ORDER BY email_address, period DESC
	`
	rows, err := f.engine.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb fetch recent months by person: %w", err)
	}
	defer rows.Close()

	return scanStringsByKey(rows)
}

// FetchAllTopPeopleByProject fetches top people for all projects using DuckDB.
func (f *DuckDBBulkFetcher) FetchAllTopPeopleByProject(ctx context.Context, opts ExportOptions) (map[string][]PersonStat, error) {
	q := `
		WITH project_people AS (
			SELECT
				l.name as label_name,
				p.email_address,
				COUNT(DISTINCT m.id) as msg_count
			FROM sqlite_db.labels l
			JOIN sqlite_db.message_labels ml ON ml.label_id = l.id
			JOIN sqlite_db.messages m ON m.id = ml.message_id
			JOIN sqlite_db.message_recipients mr ON mr.message_id = m.id
			JOIN sqlite_db.participants p ON p.id = mr.participant_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		q += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		q += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	q += `
			GROUP BY l.id, p.id
		),
		ranked AS (
			SELECT label_name, email_address, msg_count,
				ROW_NUMBER() OVER (PARTITION BY label_name ORDER BY msg_count DESC) as rn
			FROM project_people
		)
		SELECT label_name, email_address, msg_count
		FROM ranked WHERE rn <= 10
		ORDER BY label_name, rn
	`
	rows, err := f.engine.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb fetch top people by project: %w", err)
	}
	defer rows.Close()

	return scanPeopleByKey(rows)
}

// FetchAllRecentMonthsByProject fetches recent activity months for all projects using DuckDB.
func (f *DuckDBBulkFetcher) FetchAllRecentMonthsByProject(ctx context.Context, opts ExportOptions) (map[string][]string, error) {
	q := `
		WITH project_months AS (
			SELECT
				l.name as label_name,
				strftime(m.sent_at, '%Y-%m') as period
			FROM sqlite_db.labels l
			JOIN sqlite_db.message_labels ml ON ml.label_id = l.id
			JOIN sqlite_db.messages m ON m.id = ml.message_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		q += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		q += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	q += `
			GROUP BY l.name, period
		),
		ranked AS (
			SELECT label_name, period,
				ROW_NUMBER() OVER (PARTITION BY label_name ORDER BY period DESC) as rn
			FROM project_months
		)
		SELECT label_name, period
		FROM ranked WHERE rn <= 12
		ORDER BY label_name, period DESC
	`
	rows, err := f.engine.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb fetch recent months by project: %w", err)
	}
	defer rows.Close()

	return scanStringsByKey(rows)
}

// FetchAllTopPeopleByMonth fetches top people for all time periods using DuckDB.
func (f *DuckDBBulkFetcher) FetchAllTopPeopleByMonth(ctx context.Context, opts ExportOptions) (map[string][]PersonStat, error) {
	q := `
		WITH period_people AS (
			SELECT
				strftime(m.sent_at, '%Y-%m') as period,
				p.email_address,
				COUNT(DISTINCT m.id) as msg_count
			FROM sqlite_db.messages m
			JOIN sqlite_db.message_recipients mr ON mr.message_id = m.id
			JOIN sqlite_db.participants p ON p.id = mr.participant_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		q += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		q += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	q += `
			GROUP BY period, p.id
		),
		ranked AS (
			SELECT period, email_address, msg_count,
				ROW_NUMBER() OVER (PARTITION BY period ORDER BY msg_count DESC) as rn
			FROM period_people
		)
		SELECT period, email_address, msg_count
		FROM ranked WHERE rn <= 10
		ORDER BY period, rn
	`
	rows, err := f.engine.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb fetch top people by month: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]PersonStat)
	for rows.Next() {
		var period sql.NullString
		var email string
		var count int
		if err := rows.Scan(&period, &email, &count); err != nil {
			return nil, fmt.Errorf("scan top person: %w", err)
		}
		if period.Valid {
			result[period.String] = append(result[period.String], PersonStat{Email: email, Count: count})
		}
	}
	return result, rows.Err()
}

// FetchAllTopLabelsByMonth fetches top labels for all time periods using DuckDB.
func (f *DuckDBBulkFetcher) FetchAllTopLabelsByMonth(ctx context.Context, opts ExportOptions) (map[string][]LabelStat, error) {
	q := `
		WITH period_labels AS (
			SELECT
				strftime(m.sent_at, '%Y-%m') as period,
				l.name as label_name,
				COUNT(DISTINCT m.id) as msg_count
			FROM sqlite_db.messages m
			JOIN sqlite_db.message_labels ml ON ml.message_id = m.id
			JOIN sqlite_db.labels l ON l.id = ml.label_id
			WHERE m.deleted_at IS NULL
	`
	args := []interface{}{}
	if !opts.After.IsZero() {
		q += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		q += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}
	q += `
			GROUP BY period, l.id
		),
		ranked AS (
			SELECT period, label_name, msg_count,
				ROW_NUMBER() OVER (PARTITION BY period ORDER BY msg_count DESC) as rn
			FROM period_labels
		)
		SELECT period, label_name, msg_count
		FROM ranked WHERE rn <= 10
		ORDER BY period, rn
	`
	rows, err := f.engine.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb fetch top labels by month: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]LabelStat)
	for rows.Next() {
		var period sql.NullString
		var labelName string
		var count int
		if err := rows.Scan(&period, &labelName, &count); err != nil {
			return nil, fmt.Errorf("scan top label: %w", err)
		}
		if period.Valid {
			result[period.String] = append(result[period.String], LabelStat{Name: labelName, Count: count})
		}
	}
	return result, rows.Err()
}

// scanLabelsByKey scans rows of (key, label_name, count) into a map.
func scanLabelsByKey(rows *sql.Rows) (map[string][]LabelStat, error) {
	result := make(map[string][]LabelStat)
	for rows.Next() {
		var key, labelName string
		var count int
		if err := rows.Scan(&key, &labelName, &count); err != nil {
			return nil, fmt.Errorf("scan label stat: %w", err)
		}
		result[key] = append(result[key], LabelStat{Name: labelName, Count: count})
	}
	return result, rows.Err()
}

// scanPeopleByKey scans rows of (key, email, count) into a map.
func scanPeopleByKey(rows *sql.Rows) (map[string][]PersonStat, error) {
	result := make(map[string][]PersonStat)
	for rows.Next() {
		var key, email string
		var count int
		if err := rows.Scan(&key, &email, &count); err != nil {
			return nil, fmt.Errorf("scan person stat: %w", err)
		}
		result[key] = append(result[key], PersonStat{Email: email, Count: count})
	}
	return result, rows.Err()
}

// scanStringsByKey scans rows of (key, value) into a map.
func scanStringsByKey(rows *sql.Rows) (map[string][]string, error) {
	result := make(map[string][]string)
	for rows.Next() {
		var key string
		var value sql.NullString
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan string value: %w", err)
		}
		if value.Valid {
			result[key] = append(result[key], value.String)
		}
	}
	return result, rows.Err()
}
