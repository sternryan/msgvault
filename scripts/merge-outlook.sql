-- Merge Outlook data from /tmp/outlook-import.db into main msgvault.db
-- The import DB has source_id=1 for the Outlook account.
-- Main DB has sources 1-5 already. We'll insert as source_id=6.

ATTACH DATABASE '/tmp/outlook-import.db' AS src;

BEGIN TRANSACTION;

-- 1. Insert the source (account)
INSERT INTO main.sources (source_type, identifier, display_name, last_sync_at, sync_cursor, sync_config, created_at, updated_at, oauth_app)
SELECT source_type, identifier, display_name, last_sync_at, sync_cursor, sync_config, created_at, updated_at, oauth_app
FROM src.sources WHERE id = 1;

-- Capture the new source ID
-- SQLite: last_insert_rowid() gives us the new source id

-- 2. Create temp mapping tables
CREATE TEMP TABLE source_map (old_id INTEGER PRIMARY KEY, new_id INTEGER);
INSERT INTO source_map VALUES (1, last_insert_rowid());

-- 3. Merge participants (dedup by email_address)
CREATE TEMP TABLE participant_map (old_id INTEGER PRIMARY KEY, new_id INTEGER);

-- Insert participants that don't exist yet (by email)
INSERT OR IGNORE INTO main.participants (email_address, phone_number, display_name, domain, canonical_id, created_at, updated_at)
SELECT email_address, phone_number, display_name, domain, canonical_id, created_at, updated_at
FROM src.participants;

-- Build mapping: match src participants to main participants by email
INSERT INTO participant_map (old_id, new_id)
SELECT s.id, m.id
FROM src.participants s
JOIN main.participants m ON m.email_address = s.email_address
WHERE s.email_address IS NOT NULL;

-- For participants without email, match by phone
INSERT OR IGNORE INTO participant_map (old_id, new_id)
SELECT s.id, m.id
FROM src.participants s
JOIN main.participants m ON m.phone_number = s.phone_number
WHERE s.email_address IS NULL AND s.phone_number IS NOT NULL;

-- Any remaining participants (no email, no phone) — insert them
INSERT INTO main.participants (phone_number, display_name, domain, canonical_id, created_at, updated_at)
SELECT s.phone_number, s.display_name, s.domain, s.canonical_id, s.created_at, s.updated_at
FROM src.participants s
WHERE s.id NOT IN (SELECT old_id FROM participant_map);

-- Map the newly inserted ones by rowid range
INSERT INTO participant_map (old_id, new_id)
SELECT s.id, m.id
FROM src.participants s
JOIN main.participants m ON
    (s.email_address IS NOT NULL AND m.email_address = s.email_address) OR
    (s.email_address IS NULL AND s.phone_number IS NOT NULL AND m.phone_number = s.phone_number) OR
    (s.email_address IS NULL AND s.phone_number IS NULL AND m.display_name = s.display_name AND m.domain = s.domain)
WHERE s.id NOT IN (SELECT old_id FROM participant_map);

-- 4. Merge conversations
CREATE TEMP TABLE conversation_map (old_id INTEGER PRIMARY KEY, new_id INTEGER);

INSERT INTO main.conversations (source_id, source_conversation_id, conversation_type, title, participant_count, message_count, unread_count, last_message_at, last_message_preview, metadata, created_at, updated_at)
SELECT (SELECT new_id FROM source_map WHERE old_id = c.source_id), source_conversation_id, conversation_type, title, participant_count, message_count, unread_count, last_message_at, last_message_preview, metadata, created_at, updated_at
FROM src.conversations c;

-- Map conversations by source_conversation_id
INSERT INTO conversation_map (old_id, new_id)
SELECT s.id, m.id
FROM src.conversations s
JOIN main.conversations m ON m.source_id = (SELECT new_id FROM source_map WHERE old_id = 1) AND m.source_conversation_id = s.source_conversation_id;

-- 5. Merge messages
CREATE TEMP TABLE message_map (old_id INTEGER PRIMARY KEY, new_id INTEGER);

INSERT INTO main.messages (conversation_id, source_id, source_message_id, rfc822_message_id, message_type, sent_at, received_at, read_at, delivered_at, internal_date, sender_id, is_from_me, subject, snippet, thread_position, is_read, is_delivered, is_sent, is_edited, is_forwarded, size_estimate, has_attachments, attachment_count, deleted_at, deleted_from_source_at, delete_batch_id, archived_at, indexing_version, metadata)
SELECT
    (SELECT new_id FROM conversation_map WHERE old_id = m.conversation_id),
    (SELECT new_id FROM source_map WHERE old_id = m.source_id),
    m.source_message_id, m.rfc822_message_id, m.message_type, m.sent_at, m.received_at, m.read_at, m.delivered_at, m.internal_date,
    (SELECT new_id FROM participant_map WHERE old_id = m.sender_id),
    m.is_from_me, m.subject, m.snippet, m.thread_position, m.is_read, m.is_delivered, m.is_sent, m.is_edited, m.is_forwarded, m.size_estimate, m.has_attachments, m.attachment_count, m.deleted_at, m.deleted_from_source_at, m.delete_batch_id, m.archived_at, m.indexing_version, m.metadata
FROM src.messages m;

-- Build message mapping
INSERT INTO message_map (old_id, new_id)
SELECT s.id, m.id
FROM src.messages s
JOIN main.messages m ON m.source_id = (SELECT new_id FROM source_map WHERE old_id = 1) AND m.source_message_id = s.source_message_id;

-- 6. Merge message_bodies
INSERT INTO main.message_bodies (message_id, body_text, body_html)
SELECT (SELECT new_id FROM message_map WHERE old_id = b.message_id), b.body_text, b.body_html
FROM src.message_bodies b;

-- 7. Merge message_raw
INSERT INTO main.message_raw (message_id, raw_data, raw_format, compression, encryption_version)
SELECT (SELECT new_id FROM message_map WHERE old_id = r.message_id), r.raw_data, r.raw_format, r.compression, r.encryption_version
FROM src.message_raw r;

-- 8. Merge labels
CREATE TEMP TABLE label_map (old_id INTEGER PRIMARY KEY, new_id INTEGER);

INSERT OR IGNORE INTO main.labels (source_id, source_label_id, name, label_type, color)
SELECT (SELECT new_id FROM source_map WHERE old_id = l.source_id), l.source_label_id, l.name, l.label_type, l.color
FROM src.labels l;

INSERT INTO label_map (old_id, new_id)
SELECT s.id, m.id
FROM src.labels s
JOIN main.labels m ON m.source_id = (SELECT new_id FROM source_map WHERE old_id = 1) AND m.name = s.name;

-- 9. Merge message_labels
INSERT OR IGNORE INTO main.message_labels (message_id, label_id)
SELECT (SELECT new_id FROM message_map WHERE old_id = ml.message_id),
       (SELECT new_id FROM label_map WHERE old_id = ml.label_id)
FROM src.message_labels ml;

-- 10. Merge message_recipients
INSERT OR IGNORE INTO main.message_recipients (message_id, participant_id, recipient_type, display_name)
SELECT (SELECT new_id FROM message_map WHERE old_id = mr.message_id),
       (SELECT new_id FROM participant_map WHERE old_id = mr.participant_id),
       mr.recipient_type, mr.display_name
FROM src.message_recipients mr
WHERE (SELECT new_id FROM participant_map WHERE old_id = mr.participant_id) IS NOT NULL;

-- 11. Merge attachments
INSERT INTO main.attachments (message_id, filename, mime_type, size, content_hash, content_id, storage_path, media_type, width, height, duration_ms, thumbnail_hash, thumbnail_path, source_attachment_id, attachment_metadata, encryption_version, created_at)
SELECT (SELECT new_id FROM message_map WHERE old_id = a.message_id),
       a.filename, a.mime_type, a.size, a.content_hash, a.content_id, a.storage_path, a.media_type, a.width, a.height, a.duration_ms, a.thumbnail_hash, a.thumbnail_path, a.source_attachment_id, a.attachment_metadata, a.encryption_version, a.created_at
FROM src.attachments a;

-- 12. Merge participant_identifiers
INSERT OR IGNORE INTO main.participant_identifiers (participant_id, identifier_type, identifier_value, display_value, is_primary)
SELECT (SELECT new_id FROM participant_map WHERE old_id = pi.participant_id),
       pi.identifier_type, pi.identifier_value, pi.display_value, pi.is_primary
FROM src.participant_identifiers pi
WHERE (SELECT new_id FROM participant_map WHERE old_id = pi.participant_id) IS NOT NULL;

-- 13. Merge conversation_participants
INSERT OR IGNORE INTO main.conversation_participants (conversation_id, participant_id, role, joined_at, left_at)
SELECT (SELECT new_id FROM conversation_map WHERE old_id = cp.conversation_id),
       (SELECT new_id FROM participant_map WHERE old_id = cp.participant_id),
       cp.role, cp.joined_at, cp.left_at
FROM src.conversation_participants cp
WHERE (SELECT new_id FROM participant_map WHERE old_id = cp.participant_id) IS NOT NULL;

-- 14. Merge sync_runs
INSERT INTO main.sync_runs (source_id, started_at, completed_at, status, messages_processed, messages_added, messages_updated, errors_count, error_message, cursor_before, cursor_after)
SELECT (SELECT new_id FROM source_map WHERE old_id = sr.source_id),
       sr.started_at, sr.completed_at, sr.status, sr.messages_processed, sr.messages_added, sr.messages_updated, sr.errors_count, sr.error_message, sr.cursor_before, sr.cursor_after
FROM src.sync_runs sr;

-- 15. Merge sync_checkpoints
INSERT OR IGNORE INTO main.sync_checkpoints (source_id, checkpoint_type, checkpoint_value, updated_at)
SELECT (SELECT new_id FROM source_map WHERE old_id = sc.source_id),
       sc.checkpoint_type, sc.checkpoint_value, sc.updated_at
FROM src.sync_checkpoints sc;

COMMIT;

-- Verify
SELECT 'Merge complete. New account:';
SELECT id, identifier, display_name, source_type FROM main.sources ORDER BY id DESC LIMIT 1;
SELECT 'Total messages:', COUNT(*) FROM main.messages;

DETACH DATABASE src;
