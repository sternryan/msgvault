package authority

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// BuildURLHashCache walks forge's sources/<id>/manifest.yaml filesystem
// and UPSERTs (url_normalized, source_hash, compiled, refreshed_at) rows
// into msgvault.db's url_hash_cache table. Resolves the §6 BLOCKER:
// forge sources.db has no `url` column, so the URL→source_hash mapping
// must be derived from the per-source manifest YAML on disk.
//
// Read-only against the forge filesystem (and sourcesDB). Tolerates
// per-file YAML/read errors with slog.Warn (A2 mitigation): a single
// malformed manifest never fails the whole recompute.
//
// `compiled` is set to 1 only when BOTH:
//   1. forge sources.db row for source_hash has status='compiled', AND
//   2. the manifest YAML's `status` field equals "compiled".
func BuildURLHashCache(ctx context.Context, db *sql.DB, forgeSourcesDir string, sourcesDB *sql.DB) error {
	approved, err := loadCompiledHashes(ctx, sourcesDB)
	if err != nil {
		return fmt.Errorf("authority: load compiled source_hashes: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("authority: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entries, err := os.ReadDir(forgeSourcesDir)
	if err != nil {
		return fmt.Errorf("authority: read forge sources dir %q: %w", forgeSourcesDir, err)
	}

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		manifestPath := filepath.Join(forgeSourcesDir, ent.Name(), "manifest.yaml")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			// Missing manifest is normal for partial sources; warn at debug-ish
			// level and continue. Other read errors (permissions etc.) likewise
			// degrade gracefully per A2.
			if !os.IsNotExist(err) {
				slog.Warn("authority: skipping unreadable manifest", "path", manifestPath, "err", err)
			}
			continue
		}
		var m struct {
			OriginURL  string `yaml:"origin_url"`
			SourceHash string `yaml:"source_hash"`
			Status     string `yaml:"status"`
		}
		if err := yaml.Unmarshal(data, &m); err != nil {
			slog.Warn("authority: skipping malformed manifest", "path", manifestPath, "err", err)
			continue
		}
		if m.OriginURL == "" || m.SourceHash == "" {
			continue
		}
		normalized := NormalizeURL(m.OriginURL)
		compiled := 0
		if approved[m.SourceHash] && m.Status == "compiled" {
			compiled = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO url_hash_cache (url_normalized, source_hash, compiled, refreshed_at)
			 VALUES (?, ?, ?, datetime('now'))
			 ON CONFLICT(url_normalized) DO UPDATE SET
			   source_hash  = excluded.source_hash,
			   compiled     = excluded.compiled,
			   refreshed_at = excluded.refreshed_at`,
			normalized, m.SourceHash, compiled,
		); err != nil {
			return fmt.Errorf("authority: upsert url_hash_cache for %q: %w", normalized, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("authority: commit url_hash_cache tx: %w", err)
	}
	return nil
}

func loadCompiledHashes(ctx context.Context, sourcesDB *sql.DB) (map[string]bool, error) {
	out := map[string]bool{}
	if sourcesDB == nil {
		return out, nil
	}
	rows, err := sourcesDB.QueryContext(ctx,
		`SELECT source_hash FROM sources WHERE status = 'compiled'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out[h] = true
	}
	return out, rows.Err()
}

// NormalizeURL canonicalises a raw URL for cache key equality:
//   - lowercases scheme + host
//   - strips fragment
//   - trims a single trailing slash from the path
//   - parse failures fall back to a lowercased copy of the trimmed input
//
// Query string is preserved verbatim except for fragment removal (we don't
// strip utm_* etc. in v1; manifests publish the canonical origin_url).
func NormalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return strings.ToLower(trimmed)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	s := u.String()
	if strings.HasSuffix(s, "/") && len(s) > len(u.Scheme)+3 {
		s = strings.TrimRight(s, "/")
	}
	return s
}
