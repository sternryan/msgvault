-- msgvault fixture (in-memory build for authority tests).
-- Subset of internal/store/schema.sql tables required for authority recompute:
--   participants, conversations, messages, message_bodies.
-- Three senders, six inbound messages, three reply messages from "me".
-- All received_at timestamps are set via the test harness (datetime('now', '-N days'))
-- so the 7d window stays valid. The .sql intentionally omits received_at literals;
-- the test sets them via UPDATE before invoking Recompute.

CREATE TABLE IF NOT EXISTS participants (
    id            INTEGER PRIMARY KEY,
    email_address TEXT,
    display_name  TEXT
);

CREATE TABLE IF NOT EXISTS conversations (
    id                     INTEGER PRIMARY KEY,
    source_conversation_id TEXT,
    title                  TEXT
);

CREATE TABLE IF NOT EXISTS messages (
    id              INTEGER PRIMARY KEY,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id),
    sender_id       INTEGER REFERENCES participants(id),
    is_from_me      BOOLEAN NOT NULL DEFAULT 0,
    received_at     DATETIME,
    sent_at         DATETIME,
    subject         TEXT
);

CREATE TABLE IF NOT EXISTS message_bodies (
    message_id INTEGER PRIMARY KEY REFERENCES messages(id),
    body_text  TEXT,
    body_html  TEXT
);

-- Participants:
--   1 = me (ryan@example.com)
--   2 = alice@example.com           — 2 inbound, 1 reply  → reply_rate 0.5
--   3 = bob@stanford.edu            — 2 inbound, 2 replies → reply_rate 1.0
--   4 = carol@newsletter.com        — 2 inbound, 0 replies → reply_rate 0.0
INSERT INTO participants (id, email_address, display_name) VALUES
    (1, 'ryan@example.com',        'Me'),
    (2, 'alice@example.com',       'Alice'),
    (3, 'bob@stanford.edu',        'Bob'),
    (4, 'carol@newsletter.com',    'Carol');

INSERT INTO conversations (id, source_conversation_id, title) VALUES
    (10, 'thread-alice-A', 'Alice thread'),
    (11, 'thread-bob-B',   'Bob thread'),
    (12, 'thread-carol-C', 'Carol newsletter');

-- Inbound + reply pairs. received_at filled by the test harness.
INSERT INTO messages (id, conversation_id, sender_id, is_from_me, subject) VALUES
    -- Alice: 2 inbound, 1 reply
    (100, 10, 2, 0, 'Re: §351'),
    (101, 10, 1, 1, 'Re: §351'),
    (102, 10, 2, 0, 'Re: §351 follow-up'),
    -- Bob: 2 inbound, 2 replies
    (110, 11, 3, 0, 'arXiv preprint'),
    (111, 11, 1, 1, 'Re: arXiv preprint'),
    (112, 11, 3, 0, 'follow up'),
    (113, 11, 1, 1, 'Re: follow up'),
    -- Carol: 2 inbound, 0 replies
    (120, 12, 4, 0, 'Newsletter #3'),
    (121, 12, 4, 0, 'Newsletter #4');

INSERT INTO message_bodies (message_id, body_text, body_html) VALUES
    (100, 'See https://example.com/post/alpha for context.',         NULL),
    (101, 'Got it, thanks.',                                         NULL),
    (102, 'Anything else on https://example.com/post/alpha?',        NULL),
    (110, 'New paper: https://stanford.edu/research/beta is great.', NULL),
    (111, 'Will read.',                                              NULL),
    (112, 'Also check https://stanford.edu/research/beta references.', NULL),
    (113, 'Done, thanks.',                                           NULL),
    (120, 'This week at https://newsletter.com/issue/3',             NULL),
    (121, 'Latest: https://newsletter.com/issue/3',                  NULL);
