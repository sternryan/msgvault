package vault

import (
	"context"

	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/store"
)

// BulkDataFetcher provides optimized bulk data fetching for vault export.
// Instead of N+1 queries (one per entity), bulk fetchers execute a single
// query per data type and return maps keyed by entity (email, label, period).
type BulkDataFetcher interface {
	// FetchAllTopLabelsByPerson returns top labels for each person email.
	// Map key: email address
	// Map value: slice of label statistics (limited to top 10)
	FetchAllTopLabelsByPerson(ctx context.Context, opts ExportOptions) (map[string][]LabelStat, error)

	// FetchAllRelatedPeople returns related people for each person email.
	// Map key: email address
	// Map value: slice of related person records (limited to top 10)
	FetchAllRelatedPeople(ctx context.Context, opts ExportOptions) (map[string][]RelatedPerson, error)

	// FetchAllRecentMonthsByPerson returns recent activity months for each person.
	// Map key: email address
	// Map value: slice of period strings "YYYY-MM" (limited to 12 months)
	FetchAllRecentMonthsByPerson(ctx context.Context, opts ExportOptions) (map[string][]string, error)

	// FetchAllTopPeopleByProject returns top people for each project/label.
	// Map key: label name
	// Map value: slice of person statistics (limited to top 10)
	FetchAllTopPeopleByProject(ctx context.Context, opts ExportOptions) (map[string][]PersonStat, error)

	// FetchAllRecentMonthsByProject returns recent activity months for each project.
	// Map key: label name
	// Map value: slice of period strings "YYYY-MM" (limited to 12 months)
	FetchAllRecentMonthsByProject(ctx context.Context, opts ExportOptions) (map[string][]string, error)

	// FetchAllTopPeopleByMonth returns top people for each time period.
	// Map key: period string "YYYY-MM"
	// Map value: slice of person statistics (limited to top 10)
	FetchAllTopPeopleByMonth(ctx context.Context, opts ExportOptions) (map[string][]PersonStat, error)

	// FetchAllTopLabelsByMonth returns top labels for each time period.
	// Map key: period string "YYYY-MM"
	// Map value: slice of label statistics (limited to top 10)
	FetchAllTopLabelsByMonth(ctx context.Context, opts ExportOptions) (map[string][]LabelStat, error)
}

// NewBulkDataFetcher creates the appropriate bulk fetcher based on available query engine.
// Uses DuckDB when available for vectorized execution over SQLite tables;
// falls back to direct SQLite queries otherwise.
func NewBulkDataFetcher(engine query.Engine, st *store.Store) BulkDataFetcher {
	if duckdb, ok := engine.(*query.DuckDBEngine); ok {
		return NewDuckDBBulkFetcher(duckdb)
	}
	return NewSQLiteBulkFetcher(st.DB())
}
