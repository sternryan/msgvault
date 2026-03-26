package cmd

import (
	"fmt"
	"os"

	"github.com/wesm/msgvault/internal/query"
	"github.com/wesm/msgvault/internal/store"
)

// initQueryEngine creates a query engine with appropriate backend (DuckDB or SQLite).
// It returns the engine, a cleanup function that must be deferred, and any error.
// If forceSQL is true, SQLite is used directly. Otherwise, DuckDB over Parquet is
// preferred when available. If skipCacheBuild is false, the cache is automatically
// built/updated when needed.
func initQueryEngine(dbPath, analyticsDir string, forceSQL, skipCacheBuild bool) (*store.Store, query.Engine, func(), error) {
	s, err := openStore(dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open database: %w", err)
	}

	cleanup := func() { s.Close() }

	// Check if cache needs to be built/updated
	if !forceSQL && !skipCacheBuild {
		staleness := cacheNeedsBuild(dbPath, analyticsDir)
		if staleness.NeedsBuild {
			fmt.Printf("Building analytics cache (%s)...\n", staleness.Reason)
			result, err := buildCache(dbPath, analyticsDir, staleness.FullRebuild)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to build cache: %v\n", err)
				fmt.Fprintf(os.Stderr, "Falling back to SQLite (may be slow for large archives)\n")
			} else if !result.Skipped {
				fmt.Printf("Cached %d messages for fast queries.\n", result.ExportedCount)
			}
		}
	}

	var engine query.Engine

	if !forceSQL && query.HasParquetData(analyticsDir) {
		duckEngine, err := query.NewDuckDBEngine(analyticsDir, dbPath, s.DB())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to open Parquet engine: %v\n", err)
			fmt.Fprintf(os.Stderr, "Falling back to SQLite (may be slow for large archives)\n")
			engine = query.NewSQLiteEngine(s.DB())
		} else {
			engine = duckEngine
			origCleanup := cleanup
			cleanup = func() {
				duckEngine.Close()
				origCleanup()
			}
		}
	} else {
		if !forceSQL {
			fmt.Fprintf(os.Stderr, "Note: No cache data available, using SQLite (slow for large archives)\n")
			fmt.Fprintf(os.Stderr, "Run 'msgvault build-cache' to enable fast queries.\n")
		}
		engine = query.NewSQLiteEngine(s.DB())
	}

	return s, engine, cleanup, nil
}
