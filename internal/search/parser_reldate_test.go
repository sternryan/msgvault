package search

import (
	"testing"
	"time"
)

// TestParseRelativeDate_EdgeCases covers the uncovered branches in
// parseRelativeDate: uppercase unit letters, non-digit prefixes,
// too-short values, invalid units, and zero amounts.
func TestParseRelativeDate_EdgeCases(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p := &Parser{Now: func() time.Time { return fixedNow }}

	tests := []struct {
		name       string
		query      string
		wantBefore bool
		wantAfter  bool
	}{
		{
			name:       "uppercase unit D",
			query:      "newer_than:7D",
			wantAfter:  true,
			wantBefore: false,
		},
		{
			name:       "uppercase unit W",
			query:      "older_than:2W",
			wantBefore: true,
			wantAfter:  false,
		},
		{
			name:       "non-digit prefix ignored",
			query:      "newer_than:abc",
			wantAfter:  false,
			wantBefore: false,
		},
		{
			name:       "too short value (single char)",
			query:      "newer_than:d",
			wantAfter:  false,
			wantBefore: false,
		},
		{
			name:       "invalid unit letter",
			query:      "newer_than:7x",
			wantAfter:  false,
			wantBefore: false,
		},
		{
			name:       "zero amount",
			query:      "newer_than:0d",
			wantAfter:  false,
			wantBefore: false,
		},
		{
			name:       "non-digit in numeric prefix",
			query:      "older_than:1a2d",
			wantBefore: false,
			wantAfter:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.query)
			if tt.wantAfter && got.AfterDate == nil {
				t.Errorf("expected AfterDate to be set")
			}
			if !tt.wantAfter && got.AfterDate != nil {
				t.Errorf("expected AfterDate to be nil, got %v", got.AfterDate)
			}
			if tt.wantBefore && got.BeforeDate == nil {
				t.Errorf("expected BeforeDate to be set")
			}
			if !tt.wantBefore && got.BeforeDate != nil {
				t.Errorf("expected BeforeDate to be nil, got %v", got.BeforeDate)
			}
		})
	}
}
