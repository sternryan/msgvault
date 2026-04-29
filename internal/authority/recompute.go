package authority

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// progressBatch is the number of senders processed between INFO progress logs
// during the UPSERT phase. Tunable via env MSGVAULT_AUTHORITY_PROGRESS_BATCH if
// the operator wants tighter or looser cadence; default is 1000.
const progressBatch = 1000

// RecomputeResult summarises one recompute run.
type RecomputeResult struct {
	SendersUpdated int
	NewWatermark   int64
	MaxVolume      int
	ReplyMode      string
}

// Recompute performs an incremental authority recompute (D-10 two-pass union).
// See plan 16-01 Task 3 for the orchestration spec.
//
//   - msgvaultDB: read+write target containing messages/participants/conversations
//     and the authority_* + url_hash_cache tables (created via InitSchema).
//   - sourcesDB:  read-only forge sources.db (Phase 14 seam, ?mode=ro).
//   - forgeSourcesDir: path to forge sources/ on disk for url_hash_cache build.
//   - userEmail:  primary user email; required only when is_from_me is unbacked
//     and we must fall back to LOWER(p.email_address) match (Pitfall 3 / A1).
func Recompute(ctx context.Context, msgvaultDB, sourcesDB *sql.DB, forgeSourcesDir, userEmail string) (RecomputeResult, error) {
	return recompute(ctx, msgvaultDB, sourcesDB, forgeSourcesDir, userEmail, false)
}

// RecomputeFull bypasses the touched-senders union and recomputes every
// inbound sender from scratch. D-11 parity: must match a from-zero
// incremental run within ±0.001 because max_v + per-sender SQL are
// identical; only the sender SET differs.
func RecomputeFull(ctx context.Context, msgvaultDB, sourcesDB *sql.DB, forgeSourcesDir, userEmail string) (RecomputeResult, error) {
	return recompute(ctx, msgvaultDB, sourcesDB, forgeSourcesDir, userEmail, true)
}

func recompute(ctx context.Context, msgvaultDB, sourcesDB *sql.DB, forgeSourcesDir, userEmail string, full bool) (RecomputeResult, error) {
	var res RecomputeResult
	runStart := time.Now()

	slog.Info("authority recompute starting", "full", full, "forge_sources_dir", forgeSourcesDir)

	stageStart := time.Now()
	if err := InitSchema(msgvaultDB); err != nil {
		return res, err
	}
	slog.Info("authority stage complete", "stage", "init_schema", "elapsed_s", time.Since(stageStart).Seconds())

	// Refresh URL hash cache before computing link_quality so the join is
	// against the current forge filesystem snapshot.
	if forgeSourcesDir != "" && sourcesDB != nil {
		stageStart = time.Now()
		slog.Info("authority stage starting", "stage", "build_url_hash_cache")
		if err := BuildURLHashCache(ctx, msgvaultDB, forgeSourcesDir, sourcesDB); err != nil {
			return res, fmt.Errorf("authority: build url cache: %w", err)
		}
		slog.Info("authority stage complete", "stage", "build_url_hash_cache", "elapsed_s", time.Since(stageStart).Seconds())
	}

	// BEGIN IMMEDIATE — fail fast if another writer holds the DB (Pitfall 7).
	// database/sql doesn't expose IMMEDIATE directly via BeginTx; emit raw.
	if _, err := msgvaultDB.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return res, fmt.Errorf("authority: begin immediate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = msgvaultDB.ExecContext(ctx, "ROLLBACK")
		}
	}()

	// Read prior watermark.
	var watermark int64
	if err := msgvaultDB.QueryRowContext(ctx,
		`SELECT last_msg_rowid FROM authority_state WHERE id = 1`,
	).Scan(&watermark); err != nil {
		return res, fmt.Errorf("authority: read watermark: %w", err)
	}

	// Determine reply detection mode (Pitfall 3 / A1).
	stageStart = time.Now()
	replyMode, err := decideReplyMode(ctx, msgvaultDB, userEmail)
	if err != nil {
		return res, err
	}
	res.ReplyMode = replyMode
	slog.Info("authority reply mode decided", "mode", replyMode, "elapsed_s", time.Since(stageStart).Seconds())
	if _, err := msgvaultDB.ExecContext(ctx,
		`UPDATE authority_state SET reply_detection_mode = ? WHERE id = 1`,
		replyMode,
	); err != nil {
		return res, fmt.Errorf("authority: persist reply mode: %w", err)
	}

	// Pitfall 2: max_v over the FULL corpus, not the touched subset.
	stageStart = time.Now()
	maxV, err := queryMaxVolume(ctx, msgvaultDB)
	if err != nil {
		return res, err
	}
	res.MaxVolume = maxV
	slog.Info("authority max_volume computed", "max_v", maxV, "elapsed_s", time.Since(stageStart).Seconds())

	// Determine sender set.
	stageStart = time.Now()
	var senders []string
	if full {
		senders, err = queryAllSenders(ctx, msgvaultDB)
	} else {
		senders, err = queryUnionSenders(ctx, msgvaultDB, watermark)
	}
	if err != nil {
		return res, err
	}
	slog.Info("authority senders enumerated", "sender_count", len(senders), "full", full, "elapsed_s", time.Since(stageStart).Seconds())

	// New watermark — always at least the prior watermark.
	var maxRowid int64
	if err := msgvaultDB.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(rowid), 0) FROM messages`,
	).Scan(&maxRowid); err != nil {
		return res, fmt.Errorf("authority: query max(rowid): %w", err)
	}
	if maxRowid < watermark {
		maxRowid = watermark
	}

	// No-op short-circuit (incremental only): no new rowid AND no senders to
	// recompute → leave authority_scores and watermark untouched, skip the
	// state update.
	if !full && len(senders) == 0 && maxRowid == watermark {
		if _, err := msgvaultDB.ExecContext(ctx, "COMMIT"); err != nil {
			return res, fmt.Errorf("authority: commit: %w", err)
		}
		committed = true
		res.NewWatermark = watermark
		slog.Info("authority recompute no-op",
			"watermark", watermark,
			"total_elapsed_s", time.Since(runStart).Seconds(),
		)
		return res, nil
	}

	// Per-sender computation, batched UPSERTs (chunk size 500 per Pitfall 7).
	const batchSize = 500
	var pending []scoreRow
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := upsertScores(ctx, msgvaultDB, pending); err != nil {
			return err
		}
		pending = pending[:0]
		return nil
	}

	stageStart = time.Now()
	slog.Info("authority scoring stage starting", "sender_count", len(senders), "reply_mode", replyMode, "max_v", maxV)
	processed := 0

	for _, email := range senders {
		volume, err := queryVolume(ctx, msgvaultDB, email)
		if err != nil {
			return res, err
		}
		volNorm := VolumeNorm(volume, maxV)

		inbound, replied, err := queryReplyCounts(ctx, msgvaultDB, email, replyMode, userEmail)
		if err != nil {
			return res, err
		}
		replyRate := ResponseRate7d(inbound, replied)

		matched, total, err := queryLinkCounts(ctx, msgvaultDB, email)
		if err != nil {
			return res, err
		}
		linkQ := LinkQuality(matched, total)

		composite := Composite(volNorm, replyRate, linkQ)
		pending = append(pending, scoreRow{
			email:    email,
			volume:   volume,
			replyRt:  replyRate,
			linkQ:    linkQ,
			score:    composite,
		})
		if len(pending) >= batchSize {
			if err := flush(); err != nil {
				return res, err
			}
		}
		processed++
		if processed%progressBatch == 0 {
			elapsed := time.Since(stageStart).Seconds()
			rate := float64(processed) / elapsed
			est := float64(len(senders)-processed) / rate
			slog.Info("authority scoring progress",
				"processed", processed,
				"total", len(senders),
				"elapsed_s", elapsed,
				"rate_per_s", rate,
				"est_remaining_s", est,
			)
		}
	}
	if err := flush(); err != nil {
		return res, err
	}
	slog.Info("authority scoring stage complete", "processed", processed, "elapsed_s", time.Since(stageStart).Seconds())

	if _, err := msgvaultDB.ExecContext(ctx,
		`UPDATE authority_state
		   SET last_msg_rowid    = ?,
		       last_recompute_at = CURRENT_TIMESTAMP,
		       last_max_volume   = ?
		 WHERE id = 1`,
		maxRowid, maxV,
	); err != nil {
		return res, fmt.Errorf("authority: update state: %w", err)
	}

	if _, err := msgvaultDB.ExecContext(ctx, "COMMIT"); err != nil {
		return res, fmt.Errorf("authority: commit: %w", err)
	}
	committed = true
	res.SendersUpdated = len(senders)
	res.NewWatermark = maxRowid
	slog.Info("authority recompute complete",
		"senders_updated", res.SendersUpdated,
		"new_watermark", res.NewWatermark,
		"max_volume", res.MaxVolume,
		"reply_mode", res.ReplyMode,
		"total_elapsed_s", time.Since(runStart).Seconds(),
	)
	return res, nil
}

type scoreRow struct {
	email    string
	volume   int
	replyRt  float64
	linkQ    float64
	score    float64
}

func upsertScores(ctx context.Context, db *sql.DB, rows []scoreRow) error {
	stmt, err := db.PrepareContext(ctx,
		`INSERT INTO authority_scores
		   (sender_email, volume, response_rate_7d, link_quality, authority_score, updated_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(sender_email) DO UPDATE SET
		   volume           = excluded.volume,
		   response_rate_7d = excluded.response_rate_7d,
		   link_quality     = excluded.link_quality,
		   authority_score  = excluded.authority_score,
		   updated_at       = CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return fmt.Errorf("authority: prepare upsert: %w", err)
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.email, r.volume, r.replyRt, r.linkQ, r.score); err != nil {
			return fmt.Errorf("authority: upsert %s: %w", r.email, err)
		}
	}
	return nil
}

// decideReplyMode chooses 'is_from_me' when the column has any positive
// rows (live signal); otherwise falls back to 'sender_email' which
// requires a non-empty userEmail (A1 mitigation).
func decideReplyMode(ctx context.Context, db *sql.DB, userEmail string) (string, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE is_from_me = 1`,
	).Scan(&n); err != nil {
		return "", fmt.Errorf("authority: probe is_from_me: %w", err)
	}
	if n > 0 {
		return "is_from_me", nil
	}
	if strings.TrimSpace(userEmail) == "" {
		return "", fmt.Errorf("authority: is_from_me has zero rows AND userEmail is empty — set MSGVAULT_USER_EMAIL")
	}
	return "sender_email", nil
}

// queryMaxVolume returns the inbound message count of the highest-volume
// sender across the FULL corpus (Pitfall 2).
func queryMaxVolume(ctx context.Context, db *sql.DB) (int, error) {
	var maxV sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(c), 0) FROM (
		   SELECT COUNT(*) AS c
		   FROM messages m
		   JOIN participants p ON p.id = m.sender_id
		   WHERE m.is_from_me = 0 AND p.email_address IS NOT NULL AND p.email_address <> ''
		   GROUP BY LOWER(TRIM(p.email_address))
		 )`,
	).Scan(&maxV)
	if err != nil {
		return 0, fmt.Errorf("authority: max_v: %w", err)
	}
	return int(maxV.Int64), nil
}

// queryAllSenders returns every distinct inbound sender (lowercased).
func queryAllSenders(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT LOWER(TRIM(p.email_address))
		   FROM messages m
		   JOIN participants p ON p.id = m.sender_id
		  WHERE m.is_from_me = 0
		    AND p.email_address IS NOT NULL AND p.email_address <> ''`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// queryUnionSenders returns affected ∪ drift senders (D-10).
func queryUnionSenders(ctx context.Context, db *sql.DB, watermark int64) ([]string, error) {
	q := `
		SELECT DISTINCT LOWER(TRIM(p.email_address))
		  FROM messages m
		  JOIN participants p ON p.id = m.sender_id
		 WHERE m.is_from_me = 0
		   AND p.email_address IS NOT NULL AND p.email_address <> ''
		   AND m.rowid > ?
		UNION
		SELECT LOWER(TRIM(p.email_address))
		  FROM messages m
		  JOIN participants p ON p.id = m.sender_id
		 WHERE m.is_from_me = 0
		   AND p.email_address IS NOT NULL AND p.email_address <> ''
		 GROUP BY LOWER(TRIM(p.email_address))
		HAVING MAX(m.received_at) BETWEEN datetime('now', '-8 days') AND datetime('now', '-6 days')
	`
	rows, err := db.QueryContext(ctx, q, watermark)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func queryVolume(ctx context.Context, db *sql.DB, email string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		   FROM messages m
		   JOIN participants p ON p.id = m.sender_id
		  WHERE LOWER(TRIM(p.email_address)) = ?
		    AND m.is_from_me = 0`,
		email,
	).Scan(&n)
	return n, err
}

// queryReplyCounts returns (inbound_total, replied_within_7d).
//
// Reply detection (Pitfall 3 / A1):
//   - mode 'is_from_me': reply row has r.is_from_me = 1
//   - mode 'sender_email': reply row's sender email matches userEmail
//
// Thread identity uses internal conversations.id via m.conversation_id
// (Pitfall 4: do NOT use thread_id; the column does not exist).
func queryReplyCounts(ctx context.Context, db *sql.DB, email, replyMode, userEmail string) (int, int, error) {
	var replyPredicate string
	args := []any{email}
	switch replyMode {
	case "is_from_me":
		replyPredicate = "r.is_from_me = 1"
	case "sender_email":
		replyPredicate = `LOWER(TRIM((SELECT email_address FROM participants WHERE id = r.sender_id))) = LOWER(?)`
		args = append(args, userEmail)
	default:
		return 0, 0, fmt.Errorf("authority: unknown reply mode %q", replyMode)
	}

	q := fmt.Sprintf(`
		SELECT
		  COUNT(*) AS inbound,
		  SUM(CASE WHEN EXISTS (
		    SELECT 1 FROM messages r
		     WHERE r.conversation_id = m.conversation_id
		       AND %s
		       AND r.received_at IS NOT NULL
		       AND m.received_at IS NOT NULL
		       AND r.received_at >= m.received_at
		       AND r.received_at <= datetime(m.received_at, '+7 days')
		  ) THEN 1 ELSE 0 END) AS replied
		FROM messages m
		JOIN participants p ON p.id = m.sender_id
		WHERE LOWER(TRIM(p.email_address)) = ?
		  AND m.is_from_me = 0
	`, replyPredicate)

	var inbound int
	var repliedNull sql.NullInt64
	if err := db.QueryRowContext(ctx, q, args...).Scan(&inbound, &repliedNull); err != nil {
		return 0, 0, fmt.Errorf("authority: reply counts for %s: %w", email, err)
	}
	return inbound, int(repliedNull.Int64), nil
}

// queryLinkCounts pulls all body_text from a sender's inbound messages,
// extracts URLs (mirroring triage.ExtractURLs), normalises each, and joins
// against url_hash_cache.compiled to derive (matched, total).
func queryLinkCounts(ctx context.Context, db *sql.DB, email string) (int, int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT COALESCE(b.body_text, '')
		   FROM messages m
		   JOIN participants p ON p.id = m.sender_id
		   LEFT JOIN message_bodies b ON b.message_id = m.id
		  WHERE LOWER(TRIM(p.email_address)) = ?
		    AND m.is_from_me = 0`,
		email,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("authority: link bodies for %s: %w", email, err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	var urls []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return 0, 0, err
		}
		for _, raw := range extractURLs(body) {
			n := NormalizeURL(raw)
			if seen[n] {
				continue
			}
			seen[n] = true
			urls = append(urls, n)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if len(urls) == 0 {
		return 0, 0, nil
	}

	matched := 0
	for _, u := range urls {
		var compiled int
		err := db.QueryRowContext(ctx,
			`SELECT compiled FROM url_hash_cache WHERE url_normalized = ?`, u,
		).Scan(&compiled)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return 0, 0, fmt.Errorf("authority: lookup url %s: %w", u, err)
		}
		if compiled == 1 {
			matched++
		}
	}
	return matched, len(urls), nil
}
