-- Fixture forge graph.db schema mirroring forge-cli/forge/db/graph.py
-- Loaded by tests via init() into testdata/forge_graph.db (or :memory:).

CREATE TABLE IF NOT EXISTS entities (
    id INTEGER PRIMARY KEY,
    canonical_name TEXT NOT NULL,
    aliases TEXT DEFAULT '',
    source_count INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS entity_edges (
    id INTEGER PRIMARY KEY,
    source_entity_id INTEGER NOT NULL REFERENCES entities(id),
    target_entity_id INTEGER REFERENCES entities(id),
    topic_slug TEXT
);

CREATE TABLE IF NOT EXISTS topic_pairs (
    topic_a TEXT NOT NULL,
    topic_b TEXT NOT NULL,
    shared_entities INTEGER NOT NULL DEFAULT 0,
    cosine_similarity REAL NOT NULL DEFAULT 0.0,
    edge_count INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT '2026-04-25T00:00:00Z',
    PRIMARY KEY (topic_a, topic_b)
);

-- 5 entities: 3 rare + 2 dense
INSERT INTO entities(id, canonical_name, aliases, source_count) VALUES
    (1, 'Vanguard',         'Vanguard Group,VG',     120),
    (2, 'Section 351',      '§351,351 exchange',       3),
    (3, 'arXiv',             '',                     250),
    (4, 'Diffraction',       'diffraction grating',     5),
    (5, 'Phase-locked loop', 'PLL',                     2);

-- entity_edges to topics
INSERT INTO entity_edges(id, source_entity_id, target_entity_id, topic_slug) VALUES
    (1, 1, 2, 'tax-strategies'),
    (2, 2, 1, 'tax-strategies'),
    (3, 4, 5, 'optics'),
    (4, 5, 4, 'electronics');

-- topic_pairs: high-bridge, low-bridge, neutral
-- high bridge: low cosine + low shared_entities
INSERT INTO topic_pairs(topic_a, topic_b, shared_entities, cosine_similarity, edge_count) VALUES
    ('optics',          'tax-strategies', 1,  0.10, 2),  -- high bridge
    ('optics',          'electronics',    2,  0.15, 4),  -- high bridge
    ('tax-strategies',  'electronics',    8,  0.65, 12); -- not bridge
