package store_test

import (
	"testing"

	"github.com/wesm/msgvault/internal/store"
	"github.com/wesm/msgvault/internal/testutil"
	"github.com/wesm/msgvault/internal/testutil/storetest"
)

// TestGetOrCreateAutoLabel verifies idempotency and label_type='auto'.
func TestGetOrCreateAutoLabel(t *testing.T) {
	f := storetest.New(t)

	// First call creates the label.
	id1, err := f.Store.GetOrCreateAutoLabel("finance")
	testutil.MustNoErr(t, err, "GetOrCreateAutoLabel (create)")
	if id1 == 0 {
		t.Error("GetOrCreateAutoLabel returned 0, want non-zero ID")
	}

	// Second call returns the same ID (idempotent).
	id2, err := f.Store.GetOrCreateAutoLabel("finance")
	testutil.MustNoErr(t, err, "GetOrCreateAutoLabel (idempotent)")
	if id2 != id1 {
		t.Errorf("GetOrCreateAutoLabel idempotent: got %d, want %d", id2, id1)
	}

	// Verify it has label_type='auto' and source_id IS NULL.
	var labelType string
	var sourceID interface{}
	err = f.Store.DB().QueryRow(
		`SELECT label_type, source_id FROM labels WHERE id = ?`, id1,
	).Scan(&labelType, &sourceID)
	testutil.MustNoErr(t, err, "query label after GetOrCreateAutoLabel")
	if labelType != "auto" {
		t.Errorf("label_type = %q, want %q", labelType, "auto")
	}
	if sourceID != nil {
		t.Errorf("source_id = %v, want nil", sourceID)
	}
}

// TestGetOrCreateAutoLabel_DifferentNames verifies distinct labels get distinct IDs.
func TestGetOrCreateAutoLabel_DifferentNames(t *testing.T) {
	f := storetest.New(t)

	idFinance, err := f.Store.GetOrCreateAutoLabel("finance")
	testutil.MustNoErr(t, err, "GetOrCreateAutoLabel finance")

	idTravel, err := f.Store.GetOrCreateAutoLabel("travel")
	testutil.MustNoErr(t, err, "GetOrCreateAutoLabel travel")

	if idFinance == idTravel {
		t.Errorf("finance and travel labels have same ID %d, want different IDs", idFinance)
	}
}

// TestInsertLifeEvent verifies event storage and retrieval.
func TestInsertLifeEvent(t *testing.T) {
	f := storetest.New(t)

	msgID := f.CreateMessage("life-event-msg-1")

	err := f.Store.InsertLifeEvent(msgID, "2024-06-15", "job_change", "Started new role at Acme Corp")
	testutil.MustNoErr(t, err, "InsertLifeEvent")

	rows, total, err := f.Store.GetLifeEvents("", 10, 0)
	testutil.MustNoErr(t, err, "GetLifeEvents")
	if total != 1 {
		t.Errorf("GetLifeEvents total = %d, want 1", total)
	}
	if len(rows) != 1 {
		t.Fatalf("GetLifeEvents len = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.MessageID != msgID {
		t.Errorf("MessageID = %d, want %d", r.MessageID, msgID)
	}
	if r.EventDate != "2024-06-15" {
		t.Errorf("EventDate = %q, want %q", r.EventDate, "2024-06-15")
	}
	if r.EventType != "job_change" {
		t.Errorf("EventType = %q, want %q", r.EventType, "job_change")
	}
	if r.Description != "Started new role at Acme Corp" {
		t.Errorf("Description = %q, want %q", r.Description, "Started new role at Acme Corp")
	}
}

// TestInsertEntity verifies entity storage and typed retrieval.
func TestInsertEntity(t *testing.T) {
	f := storetest.New(t)

	msgID := f.CreateMessage("entity-msg-1")

	err := f.Store.InsertEntity(msgID, "company", "Apple Inc.", "apple", "mentioned in email")
	testutil.MustNoErr(t, err, "InsertEntity")

	err = f.Store.InsertEntity(msgID, "person", "Tim Cook", "tim cook", "")
	testutil.MustNoErr(t, err, "InsertEntity person")

	// Filter by type: only company
	rows, total, err := f.Store.GetEntities("company", "", 10, 0)
	testutil.MustNoErr(t, err, "GetEntities company")
	if total != 1 {
		t.Errorf("GetEntities(company) total = %d, want 1", total)
	}
	if len(rows) != 1 {
		t.Fatalf("GetEntities(company) len = %d, want 1", len(rows))
	}
	e := rows[0]
	if e.EntityType != "company" {
		t.Errorf("EntityType = %q, want %q", e.EntityType, "company")
	}
	if e.Value != "Apple Inc." {
		t.Errorf("Value = %q, want %q", e.Value, "Apple Inc.")
	}
	if e.NormalizedValue != "apple" {
		t.Errorf("NormalizedValue = %q, want %q", e.NormalizedValue, "apple")
	}
	if e.Context != "mentioned in email" {
		t.Errorf("Context = %q, want %q", e.Context, "mentioned in email")
	}
}

// TestGetAutoLabels verifies sorted distinct list of category names.
func TestGetAutoLabels(t *testing.T) {
	f := storetest.New(t)

	for _, name := range []string{"work", "finance", "travel", "finance"} {
		_, err := f.Store.GetOrCreateAutoLabel(name)
		testutil.MustNoErr(t, err, "GetOrCreateAutoLabel "+name)
	}

	labels, err := f.Store.GetAutoLabels()
	testutil.MustNoErr(t, err, "GetAutoLabels")

	// finance, travel, work — sorted, distinct
	want := []string{"finance", "travel", "work"}
	if len(labels) != len(want) {
		t.Fatalf("GetAutoLabels len = %d, want %d; got %v", len(labels), len(want), labels)
	}
	for i, w := range want {
		if labels[i] != w {
			t.Errorf("labels[%d] = %q, want %q", i, labels[i], w)
		}
	}
}

// TestGetLifeEvents_TypeFilter verifies filtering by event_type.
func TestGetLifeEvents_TypeFilter(t *testing.T) {
	f := storetest.New(t)

	msgID := f.CreateMessage("life-filter-msg")
	testutil.MustNoErr(t, f.Store.InsertLifeEvent(msgID, "2024-01-01", "travel", "Trip to Japan"), "InsertLifeEvent travel")
	testutil.MustNoErr(t, f.Store.InsertLifeEvent(msgID, "2024-03-15", "health", "Annual checkup"), "InsertLifeEvent health")
	testutil.MustNoErr(t, f.Store.InsertLifeEvent(msgID, "2024-06-01", "travel", "Europe trip"), "InsertLifeEvent travel 2")

	rows, total, err := f.Store.GetLifeEvents("travel", 10, 0)
	testutil.MustNoErr(t, err, "GetLifeEvents travel")
	if total != 2 {
		t.Errorf("GetLifeEvents(travel) total = %d, want 2", total)
	}
	if len(rows) != 2 {
		t.Errorf("GetLifeEvents(travel) len = %d, want 2", len(rows))
	}

	all, allTotal, err := f.Store.GetLifeEvents("", 10, 0)
	testutil.MustNoErr(t, err, "GetLifeEvents all")
	if allTotal != 3 {
		t.Errorf("GetLifeEvents() total = %d, want 3", allTotal)
	}
	_ = all
}

// TestGetEntities_SearchQuery verifies search filtering on value/normalized_value.
func TestGetEntities_SearchQuery(t *testing.T) {
	f := storetest.New(t)

	msgID := f.CreateMessage("entity-search-msg")
	testutil.MustNoErr(t, f.Store.InsertEntity(msgID, "company", "Microsoft Corporation", "microsoft", ""), "InsertEntity microsoft")
	testutil.MustNoErr(t, f.Store.InsertEntity(msgID, "company", "Apple Inc.", "apple", ""), "InsertEntity apple")
	testutil.MustNoErr(t, f.Store.InsertEntity(msgID, "person", "Tim Cook", "tim cook", ""), "InsertEntity person")

	// Search for "micro" — should match Microsoft
	rows, total, err := f.Store.GetEntities("", "micro", 10, 0)
	testutil.MustNoErr(t, err, "GetEntities search micro")
	if total != 1 {
		t.Errorf("GetEntities(micro) total = %d, want 1", total)
	}
	if len(rows) != 1 || rows[0].Value != "Microsoft Corporation" {
		t.Errorf("GetEntities(micro) unexpected result: %+v", rows)
	}
}

// TestGetLifeEventsForExport verifies JOIN to get source_message_id.
func TestGetLifeEventsForExport(t *testing.T) {
	f := storetest.New(t)

	// Create a message with a known source_message_id.
	msgID := f.CreateMessage("source-msg-export-123")
	testutil.MustNoErr(t, f.Store.InsertLifeEvent(msgID, "2024-07-04", "milestone", "Launched product"), "InsertLifeEvent")

	export, err := f.Store.GetLifeEventsForExport("")
	testutil.MustNoErr(t, err, "GetLifeEventsForExport")
	if len(export) != 1 {
		t.Fatalf("GetLifeEventsForExport len = %d, want 1", len(export))
	}
	e := export[0]
	if e.Date != "2024-07-04" {
		t.Errorf("Date = %q, want %q", e.Date, "2024-07-04")
	}
	if e.Type != "milestone" {
		t.Errorf("Type = %q, want %q", e.Type, "milestone")
	}
	if e.SourceMessageID != "source-msg-export-123" {
		t.Errorf("SourceMessageID = %q, want %q", e.SourceMessageID, "source-msg-export-123")
	}
}

// TestGetEntityMessageIDs verifies drill-down by entity value.
func TestGetEntityMessageIDs(t *testing.T) {
	f := storetest.New(t)

	msg1 := f.CreateMessage("entity-drilldown-1")
	msg2 := f.CreateMessage("entity-drilldown-2")

	testutil.MustNoErr(t, f.Store.InsertEntity(msg1, "company", "Google LLC", "google", ""), "InsertEntity msg1")
	testutil.MustNoErr(t, f.Store.InsertEntity(msg2, "company", "google", "google", ""), "InsertEntity msg2")

	ids, err := f.Store.GetEntityMessageIDs("google")
	testutil.MustNoErr(t, err, "GetEntityMessageIDs")
	if len(ids) != 2 {
		t.Errorf("GetEntityMessageIDs len = %d, want 2", len(ids))
	}

	// Also check that normalized_value match works.
	ids2, err := f.Store.GetEntityMessageIDs("google")
	testutil.MustNoErr(t, err, "GetEntityMessageIDs normalized")
	if len(ids2) != 2 {
		t.Errorf("GetEntityMessageIDs(normalized) len = %d, want 2", len(ids2))
	}

	// Verify the store returns a slice containing store IDs.
	found1, found2 := false, false
	for _, id := range ids {
		if id == msg1 {
			found1 = true
		}
		if id == msg2 {
			found2 = true
		}
	}
	if !found1 {
		t.Errorf("msg1 ID %d not found in GetEntityMessageIDs result", msg1)
	}
	if !found2 {
		t.Errorf("msg2 ID %d not found in GetEntityMessageIDs result", msg2)
	}
}

// TestGetOrCreateAutoLabel_PartialUniqueIndex verifies that the partial unique
// index prevents duplicate auto labels with the same name.
func TestGetOrCreateAutoLabel_PartialUniqueIndex(t *testing.T) {
	f := storetest.New(t)

	// Create a regular (non-auto) label with the same name to verify
	// the partial index doesn't conflict with source-specific labels.
	_, err := f.Store.EnsureLabel(f.Source.ID, "LABEL_finance", "finance", "user")
	testutil.MustNoErr(t, err, "EnsureLabel user")

	// Create the auto label — should succeed even though "finance" exists as a user label.
	id, err := f.Store.GetOrCreateAutoLabel("finance")
	testutil.MustNoErr(t, err, "GetOrCreateAutoLabel after user label")
	if id == 0 {
		t.Error("GetOrCreateAutoLabel returned 0")
	}

	// Second call should return same auto label ID.
	id2, err := f.Store.GetOrCreateAutoLabel("finance")
	testutil.MustNoErr(t, err, "GetOrCreateAutoLabel idempotent after user label")
	if id2 != id {
		t.Errorf("GetOrCreateAutoLabel not idempotent: got %d, want %d", id2, id)
	}
}

// storetest.Fixture exposes Source for the partial unique index test.
var _ *store.Store // ensure import used
