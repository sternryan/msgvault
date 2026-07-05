package query

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

// DuckDBEngine implements Engine using DuckDB for fast Parquet queries.
// It uses a hybrid approach:
//   - DuckDB with Parquet for fast aggregate queries
//   - DuckDB's sqlite_scan for list queries (ListMessages, ListAccounts) — non-Windows only
//   - Direct SQLite for FTS search and message body retrieval (sqlite_scan can't use FTS5)
//
// On Windows, the sqlite_scanner extension is not available (DuckDB's extension
// repository does not publish MinGW builds). All SQLite queries route through
// sqliteEngine instead.
//
// Deletion handling: The Python ETL excludes deleted messages (deleted_from_source_at IS NOT NULL)
// when building Parquet files. However, messages deleted AFTER the Parquet build will still
// appear in aggregates until the next `build-parquet --full-rebuild`. For the full deletion
// index solution, see beads issue msgvault-ozj.
type DuckDBEngine struct {
	db               *sql.DB
	analyticsDir     string
	sqlitePath       string        // Path to SQLite database for sqlite_scan queries
	sqliteDB         *sql.DB       // Direct SQLite connection for FTS and body retrieval
	sqliteEngine     *SQLiteEngine // Reusable engine for FTS cache, created once if sqliteDB is set
	hasSQLiteScanner bool          // true if DuckDB's sqlite extension is loaded
	tempTableSeq     atomic.Uint64 // Unique suffix for temp tables to avoid concurrent collisions

	// optionalCols tracks which columns exist in each Parquet table's schema.
	// Used to gracefully handle stale cache files that lack newer columns
	// (e.g. phone_number, attachment_count, sender_id, message_type added in PR #160).
	// Map: table_name -> column_name -> exists_in_parquet
	optionalCols map[string]map[string]bool

	// Search result cache: keeps the materialized temp table alive across
	// pagination calls for the same search query, avoiding repeated Parquet scans.
	searchCacheMu    sync.Mutex  // protects cache fields from concurrent goroutines
	searchCacheKey   string      // deterministic key from conditions+args
	searchCacheTable string      // temp table name (e.g. "_search_matches_42")
	searchCacheCount int64       // cached COUNT(*) from materialization
	searchCacheStats *TotalStats // cached stats from Phase 4
}

// DuckDBOptions configures optional DuckDB engine behavior.
type DuckDBOptions struct {
	// DisableSQLiteScanner prevents loading the sqlite_scanner extension even
	// on platforms where it would normally be available. This forces all SQLite
	// queries to route through sqliteEngine, matching the Windows code path.
	// Useful for testing the non-scanner code path on Linux/macOS.
	DisableSQLiteScanner bool
}

// NewDuckDBEngine creates a new DuckDB-backed query engine.
// analyticsDir should point to ~/.msgvault/analytics/
// sqlitePath should point to ~/.msgvault/msgvault.db
// sqliteDB is a direct SQLite connection for FTS search and body retrieval
//
// The engine uses a hybrid approach:
//   - DuckDB's sqlite_scan for list queries (ListMessages, ListAccounts, etc.)
//   - Direct SQLite (sqliteDB) for FTS search and message body retrieval
//
// If sqlitePath is empty, only aggregate queries and GetTotalStats will work.
// If sqliteDB is nil, Search will fall back to LIKE queries and body extraction
// from raw MIME may be slower.
func NewDuckDBEngine(analyticsDir string, sqlitePath string, sqliteDB *sql.DB, opts ...DuckDBOptions) (*DuckDBEngine, error) {
	var opt DuckDBOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	// Open in-memory DuckDB
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}

	// Constrain to single connection to ensure session settings (SET threads, ATTACH)
	// are applied consistently. DuckDB session settings don't propagate across
	// pooled connections, so limiting to one connection avoids inconsistent behavior.
	db.SetMaxOpenConns(1)

	// Enable multithreading for better query performance.
	// Use GOMAXPROCS(0) instead of NumCPU() to respect container CPU limits.
	threads := runtime.GOMAXPROCS(0)
	if _, err := db.Exec(fmt.Sprintf("SET threads = %d", threads)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set threads: %w", err)
	}

	// Install and load SQLite extension if we have a SQLite path.
	// On Windows, the sqlite_scanner extension is not available for MinGW
	// builds — all detail queries route through sqliteEngine instead.
	// DisableSQLiteScanner forces the same fallback on any platform (for testing).
	// On other platforms, try to load but fall back gracefully (e.g. no internet).
	var hasSQLiteScanner bool
	if sqlitePath != "" && runtime.GOOS != "windows" && !opt.DisableSQLiteScanner {
		if _, err := db.Exec("INSTALL sqlite; LOAD sqlite;"); err != nil {
			log.Printf("[warn] sqlite_scanner extension unavailable, falling back to direct SQLite: %v", err)
		} else {
			// Attach SQLite database as read-only
			escapedPath := strings.ReplaceAll(sqlitePath, "'", "''")
			attachSQL := fmt.Sprintf("ATTACH '%s' AS sqlite_db (TYPE sqlite, READ_ONLY)", escapedPath)
			if _, err := db.Exec(attachSQL); err != nil {
				log.Printf("[warn] failed to attach SQLite via sqlite_scanner, falling back to direct SQLite: %v", err)
			} else {
				hasSQLiteScanner = true
			}
		}
	}

	// Create reusable SQLiteEngine if we have a direct connection
	// This preserves FTS cache across calls
	var sqliteEngine *SQLiteEngine
	if sqliteDB != nil {
		sqliteEngine = NewSQLiteEngine(sqliteDB)
	}

	engine := &DuckDBEngine{
		db:               db,
		analyticsDir:     analyticsDir,
		sqlitePath:       sqlitePath,
		sqliteDB:         sqliteDB,
		sqliteEngine:     sqliteEngine,
		hasSQLiteScanner: hasSQLiteScanner,
	}

	// Probe Parquet schemas for optional columns added in PR #160 (WhatsApp import).
	// Old cache files may lack these columns; we'll supply defaults in parquetCTEs().
	engine.optionalCols = map[string]map[string]bool{
		"participants":  engine.probeParquetColumns(engine.parquetPath("participants"), false),
		"messages":      engine.probeParquetColumns(engine.parquetGlob(), true),
		"conversations": engine.probeParquetColumns(engine.parquetPath("conversations"), false),
		"sources":       engine.probeParquetColumns(engine.parquetPath("sources"), false),
	}
	var missing []string
	for _, col := range []struct{ table, col string }{
		{"participants", "phone_number"},
		{"messages", "attachment_count"},
		{"messages", "sender_id"},
		{"messages", "message_type"},
		{"conversations", "title"},
		{"conversations", "conversation_type"},
		{"sources", "source_type"},
	} {
		if !engine.optionalCols[col.table][col.col] {
			missing = append(missing, col.table+"."+col.col)
		}
	}
	if len(missing) > 0 {
		log.Printf("[warn] Parquet cache missing columns %v — run 'msgvault build-cache --full-rebuild' to update", missing)
	}

	// Register SQL views over Parquet files for raw SQL access.
	// Pass the already-probed optionalCols to avoid a redundant schema probe.
	if err := RegisterViewsWithColumns(db, analyticsDir, engine.optionalCols); err != nil {
		log.Printf("[warn] failed to register SQL views: %v", err)
		// Non-fatal: existing CTE-based queries still work.
	}

	return engine, nil
}

// QueryContext executes a query on the DuckDB connection.
// This enables callers (like DuckDBBulkFetcher) to run queries against
// the attached SQLite database via DuckDB's vectorized engine.
func (e *DuckDBEngine) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return e.db.QueryContext(ctx, query, args...)
}

// Close releases DuckDB resources, including any cached search temp table.
func (e *DuckDBEngine) Close() error {
	e.searchCacheMu.Lock()
	e.dropSearchCache()
	e.searchCacheMu.Unlock()
	return e.db.Close()
}

// QuerySQL executes an arbitrary SQL query against the DuckDB engine
// and returns the results in a columnar format. Views registered by
// RegisterViews (base + convenience) are available.
func (e *DuckDBEngine) QuerySQL(
	ctx context.Context, sqlStr string,
) (*QueryResult, error) {
	rows, err := e.db.QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	result := &QueryResult{Columns: cols}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		result.Rows = append(result.Rows, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

// hasSQLite returns true if DuckDB's sqlite_scanner extension is loaded,
// allowing sqlite_db.* queries. On Windows this is always false.
func (e *DuckDBEngine) hasSQLite() bool {
	return e.hasSQLiteScanner
}

// parquetGlob returns the glob pattern for reading message Parquet files.
func (e *DuckDBEngine) parquetGlob() string {
	return filepath.Join(e.analyticsDir, "messages", "**", "*.parquet")
}

// parquetPath returns the path pattern for a specific Parquet table.
func (e *DuckDBEngine) parquetPath(table string) string {
	return filepath.Join(e.analyticsDir, table, "*.parquet")
}

// probeParquetColumns checks which columns exist in a Parquet table's files.
// Delegates to the standalone probeColumns in views.go.
func (e *DuckDBEngine) probeParquetColumns(
	pathPattern string, hivePartitioning bool,
) map[string]bool {
	return probeColumns(e.db, pathPattern, hivePartitioning)
}

// hasCol returns true if the named column exists in the Parquet schema for the given table.
func (e *DuckDBEngine) hasCol(table, col string) bool {
	if e.optionalCols == nil {
		return true // no probe data — assume present (backwards compatible)
	}
	tbl, ok := e.optionalCols[table]
	if !ok {
		return true // table not probed — assume present
	}
	return tbl[col]
}

// parquetCTEs returns common CTEs for reading all Parquet tables.
// This is used by aggregate queries that need to join across tables.
// parquetCTEs returns the WITH clause body that defines CTEs for all Parquet
// tables. Columns are explicitly cast to their expected types using DuckDB's
// REPLACE syntax, because Parquet schema inference from SQLite can store
// integer/boolean columns as VARCHAR, causing type mismatch errors in JOINs
// and COALESCE expressions.
//
// Optional columns (phone_number, attachment_count, sender_id, message_type)
// are handled gracefully: if the Parquet file predates their addition, they
// are synthesised with sensible defaults instead of causing a binder error.
func (e *DuckDBEngine) parquetCTEs() string {
	// --- messages CTE ---
	msgReplace := []string{
		"CAST(id AS BIGINT) AS id",
		"CAST(source_id AS BIGINT) AS source_id",
		"CAST(source_message_id AS VARCHAR) AS source_message_id",
		"CAST(conversation_id AS BIGINT) AS conversation_id",
		"CAST(subject AS VARCHAR) AS subject",
		"CAST(snippet AS VARCHAR) AS snippet",
		"CAST(size_estimate AS BIGINT) AS size_estimate",
		"COALESCE(TRY_CAST(has_attachments AS BOOLEAN), false) AS has_attachments",
	}
	var msgExtra []string
	if e.hasCol("messages", "attachment_count") {
		msgReplace = append(msgReplace, "COALESCE(TRY_CAST(attachment_count AS INTEGER), 0) AS attachment_count")
	} else {
		msgExtra = append(msgExtra, "0 AS attachment_count")
	}
	if e.hasCol("messages", "sender_id") {
		msgReplace = append(msgReplace, "TRY_CAST(sender_id AS BIGINT) AS sender_id")
	} else {
		msgExtra = append(msgExtra, "NULL::BIGINT AS sender_id")
	}
	if e.hasCol("messages", "message_type") {
		msgReplace = append(msgReplace, "COALESCE(CAST(message_type AS VARCHAR), '') AS message_type")
	} else {
		msgExtra = append(msgExtra, "'' AS message_type")
	}
	msgCTE := fmt.Sprintf("SELECT * REPLACE (\n\t\t\t\t%s\n\t\t\t)", strings.Join(msgReplace, ",\n\t\t\t\t"))
	if len(msgExtra) > 0 {
		msgCTE += ", " + strings.Join(msgExtra, ", ")
	}
	msgCTE += fmt.Sprintf(" FROM read_parquet('%s', hive_partitioning=true, union_by_name=true)", e.parquetGlob())

	// --- participants CTE ---
	pReplace := []string{
		"CAST(id AS BIGINT) AS id",
		"CAST(email_address AS VARCHAR) AS email_address",
		"CAST(domain AS VARCHAR) AS domain",
		"CAST(display_name AS VARCHAR) AS display_name",
	}
	var pExtra []string
	if e.hasCol("participants", "phone_number") {
		pReplace = append(pReplace, "COALESCE(CAST(phone_number AS VARCHAR), '') AS phone_number")
	} else {
		pExtra = append(pExtra, "'' AS phone_number")
	}
	pCTE := fmt.Sprintf("SELECT * REPLACE (\n\t\t\t\t%s\n\t\t\t)", strings.Join(pReplace, ",\n\t\t\t\t"))
	if len(pExtra) > 0 {
		pCTE += ", " + strings.Join(pExtra, ", ")
	}
	pCTE += fmt.Sprintf(" FROM read_parquet('%s')", e.parquetPath("participants"))

	// --- conversations CTE ---
	convReplace := []string{
		"CAST(id AS BIGINT) AS id",
		"CAST(source_conversation_id AS VARCHAR) AS source_conversation_id",
	}
	var convExtra []string
	if e.hasCol("conversations", "title") {
		convReplace = append(convReplace, "COALESCE(CAST(title AS VARCHAR), '') AS title")
	} else {
		convExtra = append(convExtra, "'' AS title")
	}
	if e.hasCol("conversations", "conversation_type") {
		convReplace = append(convReplace, "COALESCE(CAST(conversation_type AS VARCHAR), 'email') AS conversation_type")
	} else {
		convExtra = append(convExtra, "'email' AS conversation_type")
	}
	convCTE := fmt.Sprintf("SELECT * REPLACE (\n\t\t\t\t%s\n\t\t\t)", strings.Join(convReplace, ",\n\t\t\t\t"))
	if len(convExtra) > 0 {
		convCTE += ", " + strings.Join(convExtra, ", ")
	}
	convCTE += fmt.Sprintf(" FROM read_parquet('%s')", e.parquetPath("conversations"))

	// --- sources CTE ---
	srcReplace := []string{
		"CAST(id AS BIGINT) AS id",
	}
	var srcExtra []string
	if e.hasCol("sources", "source_type") {
		srcReplace = append(srcReplace, "COALESCE(CAST(source_type AS VARCHAR), 'gmail') AS source_type")
	} else {
		srcExtra = append(srcExtra, "'gmail' AS source_type")
	}
	srcCTE := fmt.Sprintf("SELECT * REPLACE (\n\t\t\t\t%s\n\t\t\t)", strings.Join(srcReplace, ",\n\t\t\t\t"))
	if len(srcExtra) > 0 {
		srcCTE += ", " + strings.Join(srcExtra, ", ")
	}
	srcCTE += fmt.Sprintf(" FROM read_parquet('%s')", e.parquetPath("sources"))

	return fmt.Sprintf(`
		msg AS (
			%s
		),
		mr AS (
			SELECT * REPLACE (
				CAST(message_id AS BIGINT) AS message_id,
				CAST(participant_id AS BIGINT) AS participant_id,
				CAST(recipient_type AS VARCHAR) AS recipient_type,
				CAST(display_name AS VARCHAR) AS display_name
			) FROM read_parquet('%s')
		),
		p AS (
			%s
		),
		lbl AS (
			SELECT * REPLACE (
				CAST(id AS BIGINT) AS id,
				CAST(name AS VARCHAR) AS name
			) FROM read_parquet('%s')
		),
		ml AS (
			SELECT * REPLACE (
				CAST(message_id AS BIGINT) AS message_id,
				CAST(label_id AS BIGINT) AS label_id
			) FROM read_parquet('%s')
		),
		att AS (
			SELECT CAST(message_id AS BIGINT) AS message_id,
				SUM(COALESCE(TRY_CAST(size AS BIGINT), 0)) as attachment_size,
				COUNT(*) as attachment_count
			FROM read_parquet('%s')
			GROUP BY 1
		),
		src AS (
			%s
		),
		conv AS (
			%s
		)
	`, msgCTE,
		e.parquetPath("message_recipients"),
		pCTE,
		e.parquetPath("labels"),
		e.parquetPath("message_labels"),
		e.parquetPath("attachments"),
		srcCTE,
		convCTE)
}

// HasParquetData checks if Parquet files exist and are usable.
func HasParquetData(analyticsDir string) bool {
	pattern := filepath.Join(analyticsDir, "messages", "**", "*.parquet")
	matches, err := filepath.Glob(filepath.Join(analyticsDir, "messages", "*", "*.parquet"))
	if err != nil {
		return false
	}
	_ = pattern // Used in queries, not glob
	return len(matches) > 0
}

// RequiredParquetDirs lists the analytics subdirectories that must each
// contain at least one .parquet file for the cache to be considered complete.
// Shared between the cache builder, TUI, and MCP startup paths.
var RequiredParquetDirs = []string{
	"messages",
	"sources",
	"participants",
	"message_recipients",
	"labels",
	"message_labels",
	"attachments",
	"conversations",
}

// HasCompleteParquetData checks that all required parquet tables exist.
// Use this instead of HasParquetData when enabling DuckDB, since DuckDB
// unconditionally reads all tables (including conversations) and will fail
// at runtime if any are missing.
func HasCompleteParquetData(analyticsDir string) bool {
	for _, dir := range RequiredParquetDirs {
		pattern := filepath.Join(analyticsDir, dir, "*.parquet")
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			continue
		}
		// For messages, also check hive-partitioned layout (messages/year=*/*.parquet)
		if dir == "messages" {
			deepMatches, _ := filepath.Glob(filepath.Join(analyticsDir, dir, "*", "*.parquet"))
			if len(deepMatches) > 0 {
				continue
			}
		}
		return false
	}
	return true
}

// ParquetSyncState represents the sync state from _last_sync.json.
type ParquetSyncState struct {
	LastMessageID int64     `json:"last_message_id"`
	LastSyncAt    time.Time `json:"last_sync_at,omitempty"`
}
