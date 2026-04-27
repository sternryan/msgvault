-- internal/authority/schema.sql
-- Phase 16 — Trusted-contact authority graph (msgvault-side).
-- Idempotent: every statement uses IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS authority_scores (
    sender_email      TEXT PRIMARY KEY,
    volume            INTEGER NOT NULL DEFAULT 0,
    response_rate_7d  REAL NOT NULL DEFAULT 0.0,
    link_quality      REAL NOT NULL DEFAULT 0.0,
    authority_score   REAL NOT NULL DEFAULT 0.0,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS authority_state (
    id                       INTEGER PRIMARY KEY CHECK (id = 1),
    last_msg_rowid           INTEGER NOT NULL DEFAULT 0,
    last_recompute_at        DATETIME,
    last_max_volume          INTEGER NOT NULL DEFAULT 0,
    reply_detection_mode     TEXT NOT NULL DEFAULT 'is_from_me'
);

INSERT OR IGNORE INTO authority_state (id) VALUES (1);

CREATE TABLE IF NOT EXISTS url_hash_cache (
    url_normalized   TEXT PRIMARY KEY,
    source_hash      TEXT NOT NULL,
    compiled         INTEGER NOT NULL DEFAULT 0,
    refreshed_at     DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_authority_scores_score ON authority_scores(authority_score DESC);
CREATE INDEX IF NOT EXISTS idx_url_hash_cache_hash ON url_hash_cache(source_hash);
