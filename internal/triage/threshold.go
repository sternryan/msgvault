package triage

import "sort"

// SelectAndSort applies the TRIAGE-04 threshold + top-N + deterministic sort
// to a slice of candidates. Sort tuple: Score DESC, Date DESC, MessageID ASC.
// Returns up to maxN candidates whose Score >= threshold.
func SelectAndSort(cands []Candidate, threshold float64, maxN int) []Candidate {
	filtered := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Score >= threshold {
			filtered = append(filtered, c)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Score != filtered[j].Score {
			return filtered[i].Score > filtered[j].Score
		}
		if filtered[i].Date != filtered[j].Date {
			return filtered[i].Date > filtered[j].Date
		}
		return filtered[i].MessageID < filtered[j].MessageID
	})
	if maxN > 0 && len(filtered) > maxN {
		filtered = filtered[:maxN]
	}
	return filtered
}
