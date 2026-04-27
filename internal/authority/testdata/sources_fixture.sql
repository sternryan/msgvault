-- forge sources.db fixture (read-only target for url_hash_cache build).
-- Schema mirrors confirmed real shape per RESEARCH.md §6.
CREATE TABLE IF NOT EXISTS sources (
    id            INTEGER PRIMARY KEY,
    source_hash   TEXT NOT NULL UNIQUE,
    slug          TEXT,
    status        TEXT,
    source_dir    TEXT,
    ingested_at   DATETIME,
    compiled_at   DATETIME
);

INSERT INTO sources (source_hash, slug, status, source_dir, ingested_at, compiled_at) VALUES
    ('hash1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'example-1', 'compiled',  'sources/example-1', '2026-04-01T00:00:00Z', '2026-04-02T00:00:00Z'),
    ('hash2bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'example-2', 'compiled',  'sources/example-2', '2026-04-01T00:00:00Z', '2026-04-02T00:00:00Z'),
    ('hash3cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 'example-3', 'ingested',  'sources/example-3', '2026-04-01T00:00:00Z', NULL);
