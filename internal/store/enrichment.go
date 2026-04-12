package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// LifeEventRow holds a life event record returned from the store.
type LifeEventRow struct {
	ID          int64
	MessageID   int64
	EventDate   string
	EventType   string
	Description string
}

// LifeEventExportRow holds a life event with source_message_id for export.
type LifeEventExportRow struct {
	Date            string
	Type            string
	Description     string
	SourceMessageID string
}

// EntityRow holds an entity record returned from the store.
type EntityRow struct {
	ID              int64
	MessageID       int64
	EntityType      string
	Value           string
	NormalizedValue string
	Context         string
}

// GetOrCreateAutoLabel gets or creates an auto label (label_type='auto', source_id IS NULL)
// with the given name. Returns the label ID. Idempotent.
func (s *Store) GetOrCreateAutoLabel(name string) (int64, error) {
	// Try to get existing auto label.
	var id int64
	err := s.db.QueryRow(`
		SELECT id FROM labels
		WHERE source_id IS NULL AND name = ? AND label_type = 'auto'
	`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("get auto label %q: %w", name, err)
	}

	// Create new auto label.
	result, err := s.db.Exec(`
		INSERT INTO labels (source_id, source_label_id, name, label_type)
		VALUES (NULL, NULL, ?, 'auto')
	`, name)
	if err != nil {
		// Handle race: another caller may have inserted concurrently.
		if isSQLiteError(err, "UNIQUE constraint failed") {
			// Re-query.
			err2 := s.db.QueryRow(`
				SELECT id FROM labels
				WHERE source_id IS NULL AND name = ? AND label_type = 'auto'
			`, name).Scan(&id)
			if err2 != nil {
				return 0, fmt.Errorf("re-query auto label %q after race: %w", name, err2)
			}
			return id, nil
		}
		return 0, fmt.Errorf("insert auto label %q: %w", name, err)
	}

	return result.LastInsertId()
}

// InsertLifeEvent inserts a life event associated with a message.
// eventDate may be empty string (stored as empty).
func (s *Store) InsertLifeEvent(messageID int64, eventDate, eventType, description string) error {
	_, err := s.db.Exec(`
		INSERT INTO life_events (message_id, event_date, event_type, description)
		VALUES (?, ?, ?, ?)
	`, messageID, eventDate, eventType, description)
	if err != nil {
		return fmt.Errorf("insert life event: %w", err)
	}
	return nil
}

// InsertEntity inserts an entity associated with a message.
// normalizedValue and context may be empty strings (stored as NULL if empty).
func (s *Store) InsertEntity(messageID int64, entityType, value, normalizedValue, context string) error {
	var normVal interface{}
	if normalizedValue != "" {
		normVal = normalizedValue
	}
	var ctx interface{}
	if context != "" {
		ctx = context
	}
	_, err := s.db.Exec(`
		INSERT INTO entities (message_id, entity_type, value, normalized_value, context)
		VALUES (?, ?, ?, ?, ?)
	`, messageID, entityType, value, normVal, ctx)
	if err != nil {
		return fmt.Errorf("insert entity: %w", err)
	}
	return nil
}

// GetAutoLabels returns a sorted list of distinct auto label names.
func (s *Store) GetAutoLabels() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT name FROM labels
		WHERE label_type = 'auto'
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("get auto labels: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan auto label: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// GetLifeEvents returns life events with optional eventType filter.
// Returns rows and total count.
func (s *Store) GetLifeEvents(eventType string, limit, offset int) ([]LifeEventRow, int64, error) {
	var args []interface{}
	var where strings.Builder

	if eventType != "" {
		where.WriteString(" WHERE event_type = ?")
		args = append(args, eventType)
	}

	// Count total.
	var total int64
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM life_events"+where.String(),
		args...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count life_events: %w", err)
	}

	// Fetch rows.
	queryArgs := append(args, limit, offset)
	rows, err := s.db.Query(
		"SELECT id, message_id, COALESCE(event_date, ''), event_type, description FROM life_events"+
			where.String()+
			" ORDER BY event_date ASC, id ASC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query life_events: %w", err)
	}
	defer rows.Close()

	var result []LifeEventRow
	for rows.Next() {
		var r LifeEventRow
		if err := rows.Scan(&r.ID, &r.MessageID, &r.EventDate, &r.EventType, &r.Description); err != nil {
			return nil, 0, fmt.Errorf("scan life_event: %w", err)
		}
		result = append(result, r)
	}
	return result, total, rows.Err()
}

// GetLifeEventsForExport returns all life events joined with messages to include
// source_message_id for LifeVault export. Optional eventType filter.
func (s *Store) GetLifeEventsForExport(eventType string) ([]LifeEventExportRow, error) {
	query := `
		SELECT le.event_date, le.event_type, le.description, m.source_message_id
		FROM life_events le
		JOIN messages m ON m.id = le.message_id
	`
	var args []interface{}
	if eventType != "" {
		query += " WHERE le.event_type = ?"
		args = append(args, eventType)
	}
	query += " ORDER BY le.event_date ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query life_events for export: %w", err)
	}
	defer rows.Close()

	var result []LifeEventExportRow
	for rows.Next() {
		var r LifeEventExportRow
		var date, srcMsgID sql.NullString
		if err := rows.Scan(&date, &r.Type, &r.Description, &srcMsgID); err != nil {
			return nil, fmt.Errorf("scan life_event export: %w", err)
		}
		if date.Valid {
			r.Date = date.String
		}
		if srcMsgID.Valid {
			r.SourceMessageID = srcMsgID.String
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetEntities returns entities with optional type filter and search query.
// Returns rows and total count.
func (s *Store) GetEntities(entityType, searchQuery string, limit, offset int) ([]EntityRow, int64, error) {
	var conditions []string
	var args []interface{}

	if entityType != "" {
		conditions = append(conditions, "entity_type = ?")
		args = append(args, entityType)
	}
	if searchQuery != "" {
		conditions = append(conditions, "(value LIKE ? ESCAPE '\\' OR normalized_value LIKE ? ESCAPE '\\')")
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(searchQuery)
		like := "%" + escaped + "%"
		args = append(args, like, like)
	}

	var where string
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total.
	var total int64
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM entities"+where,
		args...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count entities: %w", err)
	}

	// Fetch rows.
	queryArgs := append(args, limit, offset)
	rows, err := s.db.Query(
		"SELECT id, message_id, entity_type, value, COALESCE(normalized_value, ''), COALESCE(context, '') FROM entities"+
			where+
			" ORDER BY entity_type ASC, value ASC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query entities: %w", err)
	}
	defer rows.Close()

	var result []EntityRow
	for rows.Next() {
		var r EntityRow
		if err := rows.Scan(&r.ID, &r.MessageID, &r.EntityType, &r.Value, &r.NormalizedValue, &r.Context); err != nil {
			return nil, 0, fmt.Errorf("scan entity: %w", err)
		}
		result = append(result, r)
	}
	return result, total, rows.Err()
}

// GetEntityMessageIDs returns message IDs where entity value or normalized_value
// matches the given string. Used for drill-down from entities page to messages.
func (s *Store) GetEntityMessageIDs(entityValue string) ([]int64, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT message_id FROM entities
		WHERE value = ? OR normalized_value = ?
		ORDER BY message_id ASC
	`, entityValue, entityValue)
	if err != nil {
		return nil, fmt.Errorf("get entity message IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan message ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
