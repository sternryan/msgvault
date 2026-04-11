// Package store provides database access for msgvault.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mutecomm/go-sqlcipher/v4"
)

//go:embed schema.sql schema_sqlite.sql
var schemaFS embed.FS

// Store provides database operations for msgvault.
type Store struct {
	db            *sql.DB
	dbPath        string
	fts5Available bool     // Whether FTS5 is available for full-text search
	lockFile      *os.File // Advisory lock file
	passphrase    string   // Stored for backup operations that need to reopen the DB
}

const defaultSQLiteParams = "?_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL&_foreign_keys=ON"

// OpenOption configures database opening behavior.
type OpenOption func(*openConfig)

type openConfig struct {
	passphrase string
}

// WithPassphrase sets the encryption passphrase for SQLCipher.
func WithPassphrase(passphrase string) OpenOption {
	return func(c *openConfig) {
		c.passphrase = passphrase
	}
}

// isSQLiteError checks if err is a sqlite3.Error with a message containing substr.
// This is more robust than strings.Contains on err.Error() because it first
// type-asserts to the specific driver error type using errors.As.
// Handles both value (sqlite3.Error) and pointer (*sqlite3.Error) forms.
func isSQLiteError(err error, substr string) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return strings.Contains(sqliteErr.Error(), substr)
	}
	var sqliteErrPtr *sqlite3.Error
	if errors.As(err, &sqliteErrPtr) && sqliteErrPtr != nil {
		return strings.Contains(sqliteErrPtr.Error(), substr)
	}
	return false
}

// Open opens or creates the database at the given path.
func Open(dbPath string, opts ...OpenOption) (*Store, error) {
	var cfg openConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// Build DSN with optional encryption passphrase
	params := "_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
	if cfg.passphrase != "" {
		params = "_pragma_key=" + url.QueryEscape(cfg.passphrase) + "&" + params
	}
	dsn := dbPath + "?" + params

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection (will fail if wrong passphrase)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open database (wrong passphrase?): %w", err)
	}

	// Additional verification for encrypted databases
	if cfg.passphrase != "" {
		if _, err := db.Exec("SELECT count(*) FROM sqlite_master"); err != nil {
			db.Close()
			return nil, fmt.Errorf("database passphrase verification failed: %w", err)
		}
	}

	// SQLite is single-writer; one connection eliminates
	// cross-connection visibility issues with FK checks.
	db.SetMaxOpenConns(1)

	store := &Store{
		db:         db,
		dbPath:     dbPath,
		passphrase: cfg.passphrase,
	}

	// Attempt advisory lock
	store.tryLock()

	return store, nil
}

// tryLock attempts an advisory file lock next to the database.
// Warns but does not fail if another process holds the lock.
func (s *Store) tryLock() {
	lockPath := s.dbPath + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return // Can't create lock file, skip
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "Warning: another msgvault process may be using %s\n", s.dbPath)
		return
	}

	s.lockFile = f
}

// Close checkpoints the WAL, closes the database connection, and releases
// the advisory lock.
func (s *Store) Close() error {
	// Checkpoint WAL before closing to fold it back into the main database.
	// This prevents WAL accumulation across sessions and reduces the risk of
	// corruption from stale WAL entries.
	_ = s.CheckpointWAL()
	if s.lockFile != nil {
		syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
		s.lockFile.Close()
		os.Remove(s.lockFile.Name())
		s.lockFile = nil
	}
	return s.db.Close()
}

// CheckpointWAL forces a WAL checkpoint, folding the WAL back into the main
// database file. Uses TRUNCATE mode which also resets the WAL file to zero
// bytes. Returns nil on success; callers may log but should not fail on error.
func (s *Store) CheckpointWAL() error {
	var busy, log, checkpointed int
	err := s.db.QueryRow(
		"PRAGMA wal_checkpoint(TRUNCATE)",
	).Scan(&busy, &log, &checkpointed)
	if err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf(
			"WAL checkpoint incomplete: database busy "+
				"(log=%d, checkpointed=%d)", log, checkpointed,
		)
	}
	return nil
}

// DB returns the underlying database connection for advanced queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

// DBPath returns the path to the SQLite database file.
func (s *Store) DBPath() string {
	return s.dbPath
}

// Passphrase returns the encryption passphrase used to open the database.
func (s *Store) Passphrase() string {
	return s.passphrase
}

// withTx executes fn within a database transaction. If fn returns an error,
// the transaction is rolled back; otherwise it is committed.
func (s *Store) withTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// queryInChunks executes a parameterized IN-query in chunks to stay within
// SQLite's parameter limit. queryTemplate must contain a single %s placeholder
// for the comma-separated "?" list. The prefix args are prepended before each
// chunk's args (e.g., a source_id filter).
func queryInChunks[T any](db *sql.DB, ids []T, prefixArgs []interface{}, queryTemplate string, fn func(*sql.Rows) error) error {
	const chunkSize = 500
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(prefixArgs)+len(chunk))
		args = append(args, prefixArgs...)
		for j, id := range chunk {
			placeholders[j] = "?"
			args = append(args, id)
		}

		query := fmt.Sprintf(queryTemplate, strings.Join(placeholders, ","))
		rows, err := db.Query(query, args...)
		if err != nil {
			return err
		}

		for rows.Next() {
			if err := fn(rows); err != nil {
				rows.Close()
				return err
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// insertInChunks executes a multi-value INSERT in chunks to stay within SQLite's
// parameter limit (999). The valuesPerRow specifies how many parameters are in
// each VALUES tuple (e.g., 4 for "(?, ?, ?, ?)"). The valueBuilder function
// generates the VALUES placeholders and args for each chunk of indices.
func insertInChunks(tx *sql.Tx, totalRows int, valuesPerRow int, queryPrefix string, valueBuilder func(start, end int) ([]string, []interface{})) error {
	// SQLite default SQLITE_MAX_VARIABLE_NUMBER is 999
	// Leave some margin for safety
	const maxParams = 900
	chunkSize := maxParams / valuesPerRow
	if chunkSize < 1 {
		chunkSize = 1
	}

	for i := 0; i < totalRows; i += chunkSize {
		end := i + chunkSize
		if end > totalRows {
			end = totalRows
		}

		values, args := valueBuilder(i, end)
		query := queryPrefix + strings.Join(values, ",")
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

// execInChunks executes a parameterized DELETE/UPDATE with an IN-clause in chunks
// to stay within SQLite's parameter limit. queryTemplate must contain a single %s
// placeholder for the comma-separated "?" list. The prefix args are prepended before
// each chunk's args (e.g., a message_id filter).
func execInChunks[T any](db *sql.DB, ids []T, prefixArgs []interface{}, queryTemplate string) error {
	const chunkSize = 500
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(prefixArgs)+len(chunk))
		args = append(args, prefixArgs...)
		for j, id := range chunk {
			placeholders[j] = "?"
			args = append(args, id)
		}

		query := fmt.Sprintf(queryTemplate, strings.Join(placeholders, ","))
		if _, err := db.Exec(query, args...); err != nil {
			return err
		}
	}
	return nil
}

// Rebind converts a query with ? placeholders to the appropriate format
// for the current database driver.
func (s *Store) Rebind(query string) string {
	return query
}

// FTS5Available returns whether FTS5 full-text search is available.
func (s *Store) FTS5Available() bool {
	return s.fts5Available
}

// migrateAddContentID adds the content_id column to attachments if it doesn't exist.
// This handles existing databases created before Plan 07-01.
func (s *Store) migrateAddContentID() error {
	rows, err := s.db.Query(`PRAGMA table_info(attachments)`)
	if err != nil {
		return fmt.Errorf("pragma table_info(attachments): %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan table_info: %w", err)
		}
		if name == "content_id" {
			return nil // already exists
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table_info: %w", err)
	}

	_, err = s.db.Exec(`ALTER TABLE attachments ADD COLUMN content_id TEXT`)
	if err != nil {
		return fmt.Errorf("alter table add content_id: %w", err)
	}
	return nil
}

// InitSchema initializes the database schema.
// This creates all tables if they don't exist.
func (s *Store) InitSchema() error {
	// Load and execute main schema
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema.sql: %w", err)
	}

	if _, err := s.db.Exec(string(schema)); err != nil {
		return fmt.Errorf("execute schema.sql: %w", err)
	}

	// Migrate existing databases to add content_id column if missing
	if err := s.migrateAddContentID(); err != nil {
		return fmt.Errorf("migrate content_id: %w", err)
	}

	// Migrations: add columns for databases created before these features.
	// SQLite returns "duplicate column name" if the column already exists,
	// which we treat as success.
	for _, m := range []struct {
		sql  string
		desc string
	}{
		{`ALTER TABLE sources ADD COLUMN sync_config JSON`, "sync_config"},
		{`ALTER TABLE messages ADD COLUMN rfc822_message_id TEXT`, "rfc822_message_id"},
		{`ALTER TABLE sources ADD COLUMN oauth_app TEXT`, "oauth_app"},
	} {
		if _, err := s.db.Exec(m.sql); err != nil {
			if !isSQLiteError(err, "duplicate column name") {
				return fmt.Errorf("migrate schema (%s): %w", m.desc, err)
			}
		}
	}

	// Initialize sqlite-vec virtual table (vec_messages).
	// Must happen after the core schema so the pipeline_runs table exists.
	if err := s.InitVectorTable(); err != nil {
		return fmt.Errorf("init vector table: %w", err)
	}

	// Try to load and execute SQLite-specific schema (FTS5)
	// This is optional - FTS5 may not be available in all builds
	sqliteSchema, err := schemaFS.ReadFile("schema_sqlite.sql")
	if err != nil {
		return fmt.Errorf("read schema_sqlite.sql: %w", err)
	}

	if _, err := s.db.Exec(string(sqliteSchema)); err != nil {
		if isSQLiteError(err, "no such module: fts5") {
			s.fts5Available = false
		} else {
			return fmt.Errorf("init fts5 schema: %w", err)
		}
	} else {
		s.fts5Available = true
	}

	return nil
}

// NeedsFTSBackfill reports whether the FTS table needs to be populated.
// Uses MAX(id) comparisons (instant B-tree lookups) instead of COUNT(*)
// to avoid full table scans on large databases.
func (s *Store) NeedsFTSBackfill() bool {
	if !s.fts5Available {
		return false
	}
	var msgMax int64
	if err := s.db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM messages").Scan(&msgMax); err != nil || msgMax == 0 {
		return false
	}
	var ftsMax int64
	if err := s.db.QueryRow("SELECT COALESCE(MAX(rowid), 0) FROM messages_fts").Scan(&ftsMax); err != nil {
		return false
	}
	// Backfill needed if FTS hasn't reached near the end of the messages table.
	// Using subtraction (msgMax - msgMax/10) instead of multiplication (msgMax*9/10)
	// ensures the threshold is at least msgMax for small values (e.g., msgMax=1).
	return ftsMax < msgMax-msgMax/10
}

// Stats holds database statistics.
type Stats struct {
	MessageCount    int64
	ThreadCount     int64
	AttachmentCount int64
	LabelCount      int64
	SourceCount     int64
	DatabaseSize    int64
}

// GetStats returns statistics about the database.
func (s *Store) GetStats() (*Stats, error) {
	stats := &Stats{}

	queries := []struct {
		query string
		dest  *int64
	}{
		{"SELECT COUNT(*) FROM messages", &stats.MessageCount},
		{"SELECT COUNT(*) FROM conversations", &stats.ThreadCount},
		{"SELECT COUNT(*) FROM attachments", &stats.AttachmentCount},
		{"SELECT COUNT(*) FROM labels", &stats.LabelCount},
		{"SELECT COUNT(*) FROM sources", &stats.SourceCount},
	}

	for _, q := range queries {
		if err := s.db.QueryRow(q.query).Scan(q.dest); err != nil {
			if isSQLiteError(err, "no such table") {
				continue
			}
			return nil, fmt.Errorf("get stats %q: %w", q.query, err)
		}
	}

	// Get database file size
	if info, err := os.Stat(s.dbPath); err == nil {
		stats.DatabaseSize = info.Size()
	}

	return stats, nil
}
