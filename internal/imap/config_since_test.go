package imap

import (
	"encoding/json"
	"testing"
	"time"
)

// A persisted per-account sync floor lets `sync` (which runs a full sync for
// IMAP accounts) skip re-pulling an entire mailbox every night. Without it,
// adding an existing mailbox as a new IMAP source backfills all of its history.
func TestConfigEffectiveSince(t *testing.T) {
	tests := []struct {
		name    string
		since   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "empty means no floor",
			since: "",
			want:  time.Time{},
		},
		{
			name:  "valid date parses to UTC midnight",
			since: "2026-07-21",
			want:  time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "malformed date is an error, not a silent zero",
			since:   "07/21/2026",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Host: "imap.gmail.com", Username: "u@example.com", SyncSince: tt.since}
			got, err := c.EffectiveSince()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("EffectiveSince(%q) = %v, want error", tt.since, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("EffectiveSince(%q) unexpected error: %v", tt.since, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("EffectiveSince(%q) = %v, want %v", tt.since, got, tt.want)
			}
		})
	}
}

// sync_config JSON is written by add-imap and read back by sync/sync-full.
// The new field must round-trip and must stay absent for existing accounts
// that predate it, so old configs keep deserializing unchanged.
func TestConfigSyncSinceRoundTrip(t *testing.T) {
	in := &Config{Host: "imap.gmail.com", Port: 993, TLS: true, Username: "u@example.com", SyncSince: "2026-07-21"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out Config
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SyncSince != "2026-07-21" {
		t.Errorf("SyncSince round-trip = %q, want %q", out.SyncSince, "2026-07-21")
	}

	// Backward compatibility: a config written before this field existed.
	var legacy Config
	if err := json.Unmarshal([]byte(`{"host":"outlook.office365.com","port":993,"tls":true,"username":"u@example.com"}`), &legacy); err != nil {
		t.Fatalf("legacy unmarshal: %v", err)
	}
	if legacy.SyncSince != "" {
		t.Errorf("legacy config SyncSince = %q, want empty", legacy.SyncSince)
	}
	since, err := legacy.EffectiveSince()
	if err != nil {
		t.Fatalf("legacy EffectiveSince: %v", err)
	}
	if !since.IsZero() {
		t.Errorf("legacy EffectiveSince = %v, want zero (no filter)", since)
	}

	// omitempty: field must not appear when unset, so existing stored JSON
	// is byte-identical after a rewrite.
	b2, err := json.Marshal(&legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if got := string(b2); contains(got, "sync_since") {
		t.Errorf("unset SyncSince leaked into JSON: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
