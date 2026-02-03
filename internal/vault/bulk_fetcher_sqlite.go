package vault

import (
	"context"
	"database/sql"
	"fmt"
)

// SQLiteBulkFetcher implements BulkDataFetcher using SQLite queries.
// Uses optimized queries with EXISTS subqueries (semi-joins) instead of
// DISTINCT + JOINs to avoid duplicates and improve performance.
type SQLiteBulkFetcher struct {
	db *sql.DB
}

// NewSQLiteBulkFetcher creates a new SQLite-based bulk data fetcher.
func NewSQLiteBulkFetcher(db *sql.DB) *SQLiteBulkFetcher {
	return &SQLiteBulkFetcher{db: db}
}

// FetchAllTopLabelsByPerson fetches top labels for all people in a single query.
func (f *SQLiteBulkFetcher) FetchAllTopLabelsByPerson(ctx context.Context, opts ExportOptions) (map[string][]LabelStat, error) {
	query := `
		WITH person_labels AS (
			SELECT
				p.email_address,
				l.name as label_name,
				COUNT(DISTINCT m.id) as msg_count
			FROM participants p
			JOIN message_recipients mr ON mr.participant_id = p.id
			JOIN messages m ON m.id = mr.message_id
			JOIN message_labels ml ON ml.message_id = m.id
			JOIN labels l ON l.id = ml.label_id
			WHERE m.deleted_at IS NULL
	`

	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
			GROUP BY p.email_address, l.id
		),
		ranked AS (
			SELECT
				email_address,
				label_name,
				msg_count,
				ROW_NUMBER() OVER (PARTITION BY email_address ORDER BY msg_count DESC) as rn
			FROM person_labels
		)
		SELECT email_address, label_name, msg_count
		FROM ranked
		WHERE rn <= 10
		ORDER BY email_address, rn
	`

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch top labels by person: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]LabelStat)
	for rows.Next() {
		var email, labelName string
		var count int
		if err := rows.Scan(&email, &labelName, &count); err != nil {
			return nil, fmt.Errorf("scan top label: %w", err)
		}
		result[email] = append(result[email], LabelStat{
			Name:  labelName,
			Count: count,
		})
	}

	return result, rows.Err()
}

// FetchAllRelatedPeople fetches related people for all contacts in a single query.
func (f *SQLiteBulkFetcher) FetchAllRelatedPeople(ctx context.Context, opts ExportOptions) (map[string][]RelatedPerson, error) {
	query := `
		WITH person_relationships AS (
			SELECT
				p1.email_address as person_email,
				p2.email_address as related_email,
				COUNT(DISTINCT m.conversation_id) as shared_threads
			FROM messages m
			JOIN message_recipients mr1 ON mr1.message_id = m.id
			JOIN participants p1 ON p1.id = mr1.participant_id
			JOIN message_recipients mr2 ON mr2.message_id = m.id
			JOIN participants p2 ON p2.id = mr2.participant_id
			WHERE p2.email_address != p1.email_address
				AND m.deleted_at IS NULL
	`

	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
			GROUP BY p1.id, p2.id
			HAVING shared_threads > 5
		),
		ranked AS (
			SELECT
				person_email,
				related_email,
				shared_threads,
				ROW_NUMBER() OVER (PARTITION BY person_email ORDER BY shared_threads DESC) as rn
			FROM person_relationships
		)
		SELECT person_email, related_email, shared_threads
		FROM ranked
		WHERE rn <= 10
		ORDER BY person_email, rn
	`

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch related people: %w", err)
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

// FetchAllRecentMonthsByPerson fetches recent activity months for all people in a single query.
func (f *SQLiteBulkFetcher) FetchAllRecentMonthsByPerson(ctx context.Context, opts ExportOptions) (map[string][]string, error) {
	query := `
		WITH person_months AS (
			SELECT
				p.email_address,
				strftime('%Y-%m', m.sent_at) as period
			FROM participants p
			JOIN message_recipients mr ON mr.participant_id = p.id
			JOIN messages m ON m.id = mr.message_id
			WHERE m.deleted_at IS NULL
	`

	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
			GROUP BY p.email_address, period
		),
		ranked AS (
			SELECT
				email_address,
				period,
				ROW_NUMBER() OVER (PARTITION BY email_address ORDER BY period DESC) as rn
			FROM person_months
		)
		SELECT email_address, period
		FROM ranked
		WHERE rn <= 12
		ORDER BY email_address, period DESC
	`

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch recent months by person: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var email string
		var period sql.NullString
		if err := rows.Scan(&email, &period); err != nil {
			return nil, fmt.Errorf("scan recent month: %w", err)
		}
		if period.Valid {
			result[email] = append(result[email], period.String)
		}
	}

	return result, rows.Err()
}

// FetchAllTopPeopleByProject fetches top people for all projects in a single query.
func (f *SQLiteBulkFetcher) FetchAllTopPeopleByProject(ctx context.Context, opts ExportOptions) (map[string][]PersonStat, error) {
	query := `
		WITH project_people AS (
			SELECT
				l.name as label_name,
				p.email_address,
				COUNT(DISTINCT m.id) as msg_count
			FROM labels l
			JOIN message_labels ml ON ml.label_id = l.id
			JOIN messages m ON m.id = ml.message_id
			JOIN message_recipients mr ON mr.message_id = m.id
			JOIN participants p ON p.id = mr.participant_id
			WHERE m.deleted_at IS NULL
	`

	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
			GROUP BY l.id, p.id
		),
		ranked AS (
			SELECT
				label_name,
				email_address,
				msg_count,
				ROW_NUMBER() OVER (PARTITION BY label_name ORDER BY msg_count DESC) as rn
			FROM project_people
		)
		SELECT label_name, email_address, msg_count
		FROM ranked
		WHERE rn <= 10
		ORDER BY label_name, rn
	`

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch top people by project: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]PersonStat)
	for rows.Next() {
		var labelName, email string
		var count int
		if err := rows.Scan(&labelName, &email, &count); err != nil {
			return nil, fmt.Errorf("scan top person: %w", err)
		}
		result[labelName] = append(result[labelName], PersonStat{
			Email: email,
			Count: count,
		})
	}

	return result, rows.Err()
}

// FetchAllRecentMonthsByProject fetches recent activity months for all projects in a single query.
func (f *SQLiteBulkFetcher) FetchAllRecentMonthsByProject(ctx context.Context, opts ExportOptions) (map[string][]string, error) {
	query := `
		WITH project_months AS (
			SELECT
				l.name as label_name,
				strftime('%Y-%m', m.sent_at) as period
			FROM labels l
			JOIN message_labels ml ON ml.label_id = l.id
			JOIN messages m ON m.id = ml.message_id
			WHERE m.deleted_at IS NULL
	`

	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
			GROUP BY l.name, period
		),
		ranked AS (
			SELECT
				label_name,
				period,
				ROW_NUMBER() OVER (PARTITION BY label_name ORDER BY period DESC) as rn
			FROM project_months
		)
		SELECT label_name, period
		FROM ranked
		WHERE rn <= 12
		ORDER BY label_name, period DESC
	`

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch recent months by project: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var labelName string
		var period sql.NullString
		if err := rows.Scan(&labelName, &period); err != nil {
			return nil, fmt.Errorf("scan recent month: %w", err)
		}
		if period.Valid {
			result[labelName] = append(result[labelName], period.String)
		}
	}

	return result, rows.Err()
}

// FetchAllTopPeopleByMonth fetches top people for all time periods in a single query.
func (f *SQLiteBulkFetcher) FetchAllTopPeopleByMonth(ctx context.Context, opts ExportOptions) (map[string][]PersonStat, error) {
	query := `
		WITH period_people AS (
			SELECT
				strftime('%Y-%m', m.sent_at) as period,
				p.email_address,
				COUNT(DISTINCT m.id) as msg_count
			FROM messages m
			JOIN message_recipients mr ON mr.message_id = m.id
			JOIN participants p ON p.id = mr.participant_id
			WHERE m.deleted_at IS NULL
	`

	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
			GROUP BY period, p.id
		),
		ranked AS (
			SELECT
				period,
				email_address,
				msg_count,
				ROW_NUMBER() OVER (PARTITION BY period ORDER BY msg_count DESC) as rn
			FROM period_people
		)
		SELECT period, email_address, msg_count
		FROM ranked
		WHERE rn <= 10
		ORDER BY period, rn
	`

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch top people by month: %w", err)
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
			result[period.String] = append(result[period.String], PersonStat{
				Email: email,
				Count: count,
			})
		}
	}

	return result, rows.Err()
}

// FetchAllTopLabelsByMonth fetches top labels for all time periods in a single query.
func (f *SQLiteBulkFetcher) FetchAllTopLabelsByMonth(ctx context.Context, opts ExportOptions) (map[string][]LabelStat, error) {
	query := `
		WITH period_labels AS (
			SELECT
				strftime('%Y-%m', m.sent_at) as period,
				l.name as label_name,
				COUNT(DISTINCT m.id) as msg_count
			FROM messages m
			JOIN message_labels ml ON ml.message_id = m.id
			JOIN labels l ON l.id = ml.label_id
			WHERE m.deleted_at IS NULL
	`

	args := []interface{}{}
	if !opts.After.IsZero() {
		query += ` AND m.sent_at >= ?`
		args = append(args, opts.After)
	}
	if !opts.Before.IsZero() {
		query += ` AND m.sent_at < ?`
		args = append(args, opts.Before)
	}

	query += `
			GROUP BY period, l.id
		),
		ranked AS (
			SELECT
				period,
				label_name,
				msg_count,
				ROW_NUMBER() OVER (PARTITION BY period ORDER BY msg_count DESC) as rn
			FROM period_labels
		)
		SELECT period, label_name, msg_count
		FROM ranked
		WHERE rn <= 10
		ORDER BY period, rn
	`

	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch top labels by month: %w", err)
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
			result[period.String] = append(result[period.String], LabelStat{
				Name:  labelName,
				Count: count,
			})
		}
	}

	return result, rows.Err()
}
