package search

import (
	"testing"
	"time"
)

// TestParseSize_Direct calls parseSize directly for all suffix variants and
// edge cases, covering the internal function more exhaustively than the
// higher-level Parse() tests.
func TestParseSize_Direct(t *testing.T) {
	tests := []struct {
		input   string
		wantNil bool
		wantVal int64
	}{
		// Plain bytes — the ParseInt path
		{"0", false, 0},
		{"1", false, 1},
		{"1024", false, 1024},
		{"999999", false, 999999},

		// K and KB
		{"1K", false, 1024},
		{"1KB", false, 1024},
		{"100K", false, 100 * 1024},
		{"100KB", false, 100 * 1024},

		// M and MB
		{"1M", false, 1024 * 1024},
		{"1MB", false, 1024 * 1024},
		{"5M", false, 5 * 1024 * 1024},

		// G and GB
		{"1G", false, 1024 * 1024 * 1024},
		{"1GB", false, 1024 * 1024 * 1024},
		{"2G", false, 2 * 1024 * 1024 * 1024},

		// Decimal values
		{"1.5M", false, int64(1.5 * 1024 * 1024)},
		{"0.5K", false, int64(0.5 * 1024)},
		{"2.5G", false, int64(2.5 * 1024 * 1024 * 1024)},

		// Whitespace trimming (parseSize calls strings.TrimSpace)
		{"  5M  ", false, 5 * 1024 * 1024},
		{"  100K  ", false, 100 * 1024},

		// Case-insensitive (parseSize calls strings.ToUpper)
		{"5m", false, 5 * 1024 * 1024},
		{"100k", false, 100 * 1024},
		{"1g", false, 1024 * 1024 * 1024},
		{"1mb", false, 1024 * 1024},
		{"1kb", false, 1024},
		{"1gb", false, 1024 * 1024 * 1024},

		// Invalid inputs
		{"", true, 0},
		{"abc", true, 0},
		{"M", true, 0}, // suffix only, no number
		{"K", true, 0},
		{"-1", false, -1}, // ParseInt accepts negatives
		{"xyzM", true, 0}, // letters before suffix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSize(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseSize(%q) = %d, want nil", tt.input, *got)
				}
			} else {
				if got == nil {
					t.Fatalf("parseSize(%q) = nil, want %d", tt.input, tt.wantVal)
				}
				if *got != tt.wantVal {
					t.Errorf("parseSize(%q) = %d, want %d", tt.input, *got, tt.wantVal)
				}
			}
		})
	}
}

// TestParseDate_Direct calls parseDate directly, covering all four accepted
// formats and confirming invalid inputs return nil.
func TestParseDate_Direct(t *testing.T) {
	tests := []struct {
		input     string
		wantNil   bool
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		// YYYY-MM-DD
		{"2024-01-15", false, 2024, time.January, 15},
		{"2000-12-31", false, 2000, time.December, 31},
		// YYYY/MM/DD
		{"2024/06/01", false, 2024, time.June, 1},
		// MM/DD/YYYY — day ≤ 12 so first slash-format matches
		{"01/01/2024", false, 2024, time.January, 1},
		// DD/MM/YYYY — day > 12, so MM/DD/YYYY fails and DD/MM/YYYY succeeds
		{"25/03/2024", false, 2024, time.March, 25},
		// Invalid
		{"not-a-date", true, 0, 0, 0},
		{"", true, 0, 0, 0},
		{"2024-13-01", true, 0, 0, 0}, // invalid month
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseDate(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseDate(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseDate(%q) = nil, want %04d-%02d-%02d", tt.input, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
			if got.Year() != tt.wantYear || got.Month() != tt.wantMonth || got.Day() != tt.wantDay {
				t.Errorf("parseDate(%q) = %v, want %04d-%02d-%02d", tt.input, got, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
		})
	}
}

// TestParseRelativeDate_Direct calls parseRelativeDate directly for all four
// unit types and verifies the arithmetic.
func TestParseRelativeDate_Direct(t *testing.T) {
	fixedNow := time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		input     string
		wantNil   bool
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		// Days
		{"1d", false, 2025, time.March, 19},
		{"10d", false, 2025, time.March, 10},
		// Weeks
		{"1w", false, 2025, time.March, 13},
		{"2w", false, 2025, time.March, 6},
		// Months
		{"1m", false, 2025, time.February, 20},
		{"12m", false, 2024, time.March, 20},
		// Years
		{"1y", false, 2024, time.March, 20},
		{"5y", false, 2020, time.March, 20},
		// Invalid patterns
		{"", true, 0, 0, 0},
		{"7", true, 0, 0, 0}, // missing unit
		{"abc", true, 0, 0, 0},
		{"1z", true, 0, 0, 0}, // invalid unit letter
		{"d", true, 0, 0, 0},  // missing amount
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseRelativeDate(tt.input, fixedNow)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseRelativeDate(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseRelativeDate(%q) = nil, want %04d-%02d-%02d", tt.input, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
			if got.Year() != tt.wantYear || got.Month() != tt.wantMonth || got.Day() != tt.wantDay {
				t.Errorf("parseRelativeDate(%q) = %v, want %04d-%02d-%02d", tt.input, got, tt.wantYear, tt.wantMonth, tt.wantDay)
			}
		})
	}
}

// TestParse_NegativeInteger verifies that negative plain integer sizes
// (e.g. larger:-1) are handled. ParseInt accepts negatives, so -1 is set.
func TestParse_NegativeInteger(t *testing.T) {
	got := Parse("larger:-1")
	if got == nil {
		t.Fatal("Parse returned nil")
	}
	if got.LargerThan == nil {
		t.Error("LargerThan should be set for 'larger:-1'")
	} else if *got.LargerThan != -1 {
		t.Errorf("LargerThan = %d, want -1", *got.LargerThan)
	}
}

// TestParse_ZeroSizeBytes verifies that "larger:0" is treated as a valid
// zero-byte size (not nil), since ParseInt("0") succeeds.
func TestParse_ZeroSizeBytes(t *testing.T) {
	got := Parse("larger:0")
	if got.LargerThan == nil {
		t.Error("LargerThan should be set (0 bytes) for 'larger:0'")
	} else if *got.LargerThan != 0 {
		t.Errorf("LargerThan = %d, want 0", *got.LargerThan)
	}
}
