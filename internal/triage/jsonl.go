package triage

import (
	"encoding/json"
	"fmt"
	"io"
)

// Emit writes line-delimited JSON to w, one Candidate per line. Per Pitfall 2
// the encoder is disabled for HTML escaping and uses no time.Now() — output
// is byte-identical for byte-identical inputs.
func Emit(w io.Writer, cands []Candidate) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, c := range cands {
		if err := enc.Encode(c); err != nil {
			return fmt.Errorf("emit candidate %s: %w", c.MessageID, err)
		}
	}
	return nil
}
