// Package trustedcontacts implements the TRIAGE-08 static-seed bootstrap:
// pick the top-N senders/recipients (bidirectional volume) from the msgvault
// store and write a hand-editable TOML allowlist used by triage criteria #3
// (curiosity) and #7 (expert).
package trustedcontacts

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Contact is one row in trusted_contacts.toml.
type Contact struct {
	Email  string `toml:"email"`
	Name   string `toml:"name,omitempty"`
	Volume int    `toml:"volume"`
}

// SeedFile is the TOML file shape produced by Bootstrap.
type SeedFile struct {
	GeneratedAt string    `toml:"generated_at"`
	Contacts    []Contact `toml:"contacts"`
}

// DefaultNoiseDomains is the noise-substring allowlist applied when an
// explicit list isn't supplied. Matches at the email-substring level
// (case-insensitive).
var DefaultNoiseDomains = []string{
	"no-reply",
	"noreply",
	"notifications",
	"donotreply",
	"github-noreply",
}

// Bootstrap aggregates senders+recipients from the msgvault SQLite store and
// writes a TOML SeedFile with the top-N contacts (bidirectional volume).
//
// db is an open *sql.DB pointing at msgvault.db (or a fixture). ryanEmail is
// the user's address (used to bias bidirectional weighting). topN, since,
// noiseDomains have sensible zero-defaults (10, 365d, DefaultNoiseDomains).
func Bootstrap(
	ctx context.Context,
	db *sql.DB,
	ryanEmail string,
	topN int,
	since time.Duration,
	noiseDomains []string,
	w io.Writer,
) error {
	if db == nil {
		return fmt.Errorf("bootstrap: nil db")
	}
	if topN <= 0 {
		topN = 10
	}
	if since <= 0 {
		since = 365 * 24 * time.Hour
	}
	if noiseDomains == nil {
		noiseDomains = DefaultNoiseDomains
	}
	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)

	// Aggregate inbound (sender side) and outbound (recipient side) volumes.
	// We use participants.email_address joined to messages via sender_id and
	// to message_recipients.participant_id for outbound. Both since cutoff.
	volumes := make(map[string]int)
	displayNames := make(map[string]string)

	// Inbound: sender_id -> participant.email_address, count messages per sender
	rowsIn, err := db.QueryContext(ctx, `
		SELECT COALESCE(p.email_address, ''),
		       COALESCE(p.display_name, ''),
		       COUNT(*) as c
		FROM messages m
		JOIN participants p ON p.id = m.sender_id
		WHERE p.email_address IS NOT NULL
		  AND p.email_address != ''
		  AND COALESCE(m.sent_at, m.received_at, m.archived_at) >= ?
		GROUP BY p.email_address
	`, cutoff)
	if err != nil {
		return fmt.Errorf("query inbound: %w", err)
	}
	for rowsIn.Next() {
		var email, name string
		var c int
		if err := rowsIn.Scan(&email, &name, &c); err != nil {
			rowsIn.Close()
			return fmt.Errorf("scan inbound: %w", err)
		}
		emailLower := strings.ToLower(strings.TrimSpace(email))
		if emailLower == "" {
			continue
		}
		volumes[emailLower] += c
		if name != "" && displayNames[emailLower] == "" {
			displayNames[emailLower] = name
		}
	}
	rowsIn.Close()

	// Outbound: ryan-as-sender, recipient participant emails
	if ryanEmail != "" {
		rowsOut, err := db.QueryContext(ctx, `
			SELECT COALESCE(p.email_address, ''),
			       COALESCE(p.display_name, ''),
			       COUNT(*) as c
			FROM messages m
			JOIN participants sp ON sp.id = m.sender_id
			JOIN message_recipients mr ON mr.message_id = m.id
			JOIN participants p ON p.id = mr.participant_id
			WHERE LOWER(sp.email_address) = LOWER(?)
			  AND p.email_address IS NOT NULL
			  AND p.email_address != ''
			  AND COALESCE(m.sent_at, m.received_at, m.archived_at) >= ?
			GROUP BY p.email_address
		`, ryanEmail, cutoff)
		if err != nil {
			return fmt.Errorf("query outbound: %w", err)
		}
		for rowsOut.Next() {
			var email, name string
			var c int
			if err := rowsOut.Scan(&email, &name, &c); err != nil {
				rowsOut.Close()
				return fmt.Errorf("scan outbound: %w", err)
			}
			emailLower := strings.ToLower(strings.TrimSpace(email))
			if emailLower == "" {
				continue
			}
			volumes[emailLower] += c
			if name != "" && displayNames[emailLower] == "" {
				displayNames[emailLower] = name
			}
		}
		rowsOut.Close()
	}

	// Filter noise domains (substring match) and self.
	ryanLower := strings.ToLower(strings.TrimSpace(ryanEmail))
	filtered := make([]Contact, 0, len(volumes))
	for email, vol := range volumes {
		if email == ryanLower {
			continue
		}
		if isNoise(email, noiseDomains) {
			continue
		}
		filtered = append(filtered, Contact{
			Email:  email,
			Name:   displayNames[email],
			Volume: vol,
		})
	}

	// Sort: volume desc, then email asc for stability.
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Volume != filtered[j].Volume {
			return filtered[i].Volume > filtered[j].Volume
		}
		return filtered[i].Email < filtered[j].Email
	})

	if len(filtered) > topN {
		filtered = filtered[:topN]
	}

	seed := SeedFile{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Contacts:    filtered,
	}
	enc := toml.NewEncoder(w)
	if err := enc.Encode(seed); err != nil {
		return fmt.Errorf("encode toml: %w", err)
	}
	return nil
}

// isNoise reports whether any noise substring appears in email
// (case-insensitive).
func isNoise(email string, noise []string) bool {
	emailLower := strings.ToLower(email)
	for _, n := range noise {
		if n == "" {
			continue
		}
		if strings.Contains(emailLower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}
