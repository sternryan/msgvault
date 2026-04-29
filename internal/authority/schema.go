// Package authority implements the per-sender [0,1] authority score that
// feeds Phase 14 triage criterion #7 (replacing the static contact-domain
// allowlist retired in Phase 16). See .planning/phases/16-* for the
// SPEC, CONTEXT, and RESEARCH that govern this package.
package authority

import (
	"database/sql"
	"embed"
	"fmt"
)

//go:embed schema.sql
var schemaFS embed.FS

// InitSchema creates the authority_scores, authority_state, and
// url_hash_cache tables (with seed row in authority_state). Idempotent —
// re-running against an already-initialized DB is a no-op. Mirrors the
// pattern at internal/store/store.go:527 (embedded SQL + db.Exec).
func InitSchema(db *sql.DB) error {
	sqlBytes, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("authority: read schema.sql: %w", err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("authority: init schema: %w", err)
	}
	return nil
}
