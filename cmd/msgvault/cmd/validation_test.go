package cmd

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// validateEmail checks email format for testability.
func validateEmail(email string) error {
	if !strings.Contains(email, "@") || strings.HasSuffix(email, "@") || strings.HasPrefix(email, "@") {
		return fmt.Errorf("invalid email address %q: must be in user@domain format", email)
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !strings.Contains(parts[1], ".") {
		return fmt.Errorf("invalid email address %q: must be in user@domain.tld format", email)
	}
	return nil
}

// validateDate checks date format for testability.
func validateDate(date string) error {
	if date == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("invalid date %q: use YYYY-MM-DD format", date)
	}
	return nil
}

func TestEmailValidation(t *testing.T) {
	tests := []struct {
		email   string
		wantErr bool
		errMsg  string
	}{
		{"user@example.com", false, ""},
		{"user@example.co.uk", false, ""},
		{"a@b.c", false, ""},
		{"", true, "must be in user@domain format"},
		{"noatsign", true, "must be in user@domain format"},
		{"@domain.com", true, "must be in user@domain format"},
		{"user@", true, "must be in user@domain format"},
		{"user@nodot", true, "must be in user@domain.tld format"},
		{"@", true, "must be in user@domain format"},
	}

	for _, tc := range tests {
		t.Run(tc.email, func(t *testing.T) {
			err := validateEmail(tc.email)
			if tc.wantErr {
				if err == nil {
					t.Errorf("validateEmail(%q) = nil, want error", tc.email)
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("validateEmail(%q) error = %v, want error containing %q", tc.email, err, tc.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateEmail(%q) = %v, want nil", tc.email, err)
				}
			}
		})
	}
}

func TestDateValidation(t *testing.T) {
	tests := []struct {
		date    string
		wantErr bool
	}{
		{"2024-01-01", false},
		{"2024-12-31", false},
		{"2024-1-1", true},
		{"01-01-2024", true},
		{"2024/01/01", true},
		{"not-a-date", true},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.date, func(t *testing.T) {
			err := validateDate(tc.date)
			if tc.wantErr {
				if err == nil {
					t.Errorf("validateDate(%q) = nil, want error", tc.date)
				}
			} else {
				if err != nil {
					t.Errorf("validateDate(%q) = %v, want nil", tc.date, err)
				}
			}
		})
	}
}
