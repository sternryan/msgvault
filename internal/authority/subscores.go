package authority

import "math"

// VolumeNorm computes log10(1+v)/log10(1+maxV). Per D-01: log compression
// of the heavy tail so a 1000-msg newsletter doesn't drown a 50-msg true
// expert. Returns 0.0 when maxV <= 0 (guard against division by zero on
// an empty corpus).
func VolumeNorm(v, maxV int) float64 {
	if maxV <= 0 {
		return 0.0
	}
	return math.Log10(1+float64(v)) / math.Log10(1+float64(maxV))
}

// ResponseRate7d returns repliedWithin7d / inboundCount, or 0.0 when
// inboundCount is 0 (guard against division by zero).
func ResponseRate7d(inboundCount, repliedWithin7d int) float64 {
	if inboundCount == 0 {
		return 0.0
	}
	return float64(repliedWithin7d) / float64(inboundCount)
}

// LinkQuality returns matched/total, or 0.0 when total is 0 (D-03 literal:
// no renormalization across present signals; sender with no URLs scores 0).
func LinkQuality(matched, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return float64(matched) / float64(total)
}

// Composite blends the three sub-scores per D-02:
// 0.2*volume + 0.4*replyRate + 0.4*linkQ. Frozen for v1.
func Composite(volume, replyRate, linkQ float64) float64 {
	return 0.2*volume + 0.4*replyRate + 0.4*linkQ
}
