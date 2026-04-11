package ai

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces Azure OpenAI TPM (tokens per minute) and RPM (requests per minute) limits.
// Uses dual token buckets. Zero value for either limit means unlimited.
type RateLimiter struct {
	mu  sync.Mutex
	tpm *tokenBucket // nil if tpm_limit = 0
	rpm *tokenBucket // nil if rpm_limit = 0
}

type tokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a rate limiter. Zero values disable that limit.
func NewRateLimiter(tpmLimit, rpmLimit int) *RateLimiter {
	rl := &RateLimiter{}
	if tpmLimit > 0 {
		rl.tpm = &tokenBucket{
			tokens:     float64(tpmLimit),
			capacity:   float64(tpmLimit),
			refillRate: float64(tpmLimit) / 60.0,
			lastRefill: time.Now(),
		}
	}
	if rpmLimit > 0 {
		rl.rpm = &tokenBucket{
			tokens:     float64(rpmLimit),
			capacity:   float64(rpmLimit),
			refillRate: float64(rpmLimit) / 60.0,
			lastRefill: time.Now(),
		}
	}
	return rl
}

// Wait blocks until both TPM and RPM budgets allow the request.
// estimatedTokens is the expected token count for TPM tracking.
// Returns an error if context is cancelled while waiting.
func (rl *RateLimiter) Wait(ctx context.Context, estimatedTokens int) error {
	for {
		rl.mu.Lock()
		delay := rl.tryConsume(estimatedTokens)
		rl.mu.Unlock()

		if delay == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// retry
		}
	}
}

// RecordActualTokens adjusts the TPM bucket when actual usage differs from estimate.
func (rl *RateLimiter) RecordActualTokens(estimated, actual int) {
	if rl.tpm == nil {
		return
	}
	diff := estimated - actual
	if diff == 0 {
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	// Return over-estimated tokens, or consume under-estimated
	rl.tpm.tokens += float64(diff)
	if rl.tpm.tokens > rl.tpm.capacity {
		rl.tpm.tokens = rl.tpm.capacity
	}
}

// tryConsume attempts to consume from both buckets. Returns 0 if successful,
// or the duration to wait before retrying.
func (rl *RateLimiter) tryConsume(tokens int) time.Duration {
	now := time.Now()
	maxWait := time.Duration(0)

	if rl.tpm != nil {
		rl.tpm.refill(now)
		if rl.tpm.tokens < float64(tokens) {
			needed := float64(tokens) - rl.tpm.tokens
			wait := time.Duration(needed/rl.tpm.refillRate*1000) * time.Millisecond
			if wait > maxWait {
				maxWait = wait
			}
		}
	}

	if rl.rpm != nil {
		rl.rpm.refill(now)
		if rl.rpm.tokens < 1.0 {
			needed := 1.0 - rl.rpm.tokens
			wait := time.Duration(needed/rl.rpm.refillRate*1000) * time.Millisecond
			if wait > maxWait {
				maxWait = wait
			}
		}
	}

	if maxWait > 0 {
		return maxWait
	}

	// Consume
	if rl.tpm != nil {
		rl.tpm.tokens -= float64(tokens)
	}
	if rl.rpm != nil {
		rl.rpm.tokens -= 1.0
	}
	return 0
}

func (b *tokenBucket) refill(now time.Time) {
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now
}
