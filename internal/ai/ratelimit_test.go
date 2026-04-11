package ai

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_ZeroLimits_NoBlocking(t *testing.T) {
	// Zero limits mean unlimited — Wait should return immediately
	rl := NewRateLimiter(0, 0)

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := rl.Wait(ctx, 1000); err != nil {
			t.Fatalf("Wait() returned error: %v", err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("unlimited mode took %v, want < 100ms", elapsed)
	}
}

func TestRateLimiter_RPMLimit_BlocksAfterN(t *testing.T) {
	// 6 RPM = 1 request per 10 seconds; bucket starts full at 6 tokens
	rl := NewRateLimiter(0, 6)

	ctx := context.Background()

	// First 6 requests should pass immediately (bucket full)
	start := time.Now()
	for i := 0; i < 6; i++ {
		if err := rl.Wait(ctx, 0); err != nil {
			t.Fatalf("Wait() %d returned error: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("first 6 requests took %v, want < 200ms", elapsed)
	}

	// 7th request should block (bucket empty)
	ctx7, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := rl.Wait(ctx7, 0)
	if err == nil {
		t.Error("7th request should have blocked / timed out, got nil error")
	}
}

func TestRateLimiter_TPMLimit_BlocksWhenExhausted(t *testing.T) {
	// 100 TPM = bucket of 100 tokens, refills at ~1.67/sec
	rl := NewRateLimiter(100, 0)

	ctx := context.Background()

	// Consume the full budget
	if err := rl.Wait(ctx, 100); err != nil {
		t.Fatalf("first Wait() error: %v", err)
	}

	// Next request should block because TPM bucket is exhausted
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := rl.Wait(ctxTimeout, 1)
	if err == nil {
		t.Error("Wait after exhausting TPM should have blocked, got nil error")
	}
}

func TestRateLimiter_RecordActualTokens_RefundsOverestimate(t *testing.T) {
	// 100 TPM
	rl := NewRateLimiter(100, 0)

	ctx := context.Background()

	// Estimate 80 tokens — this should drain 80 from the bucket
	if err := rl.Wait(ctx, 80); err != nil {
		t.Fatalf("Wait() error: %v", err)
	}

	// Actual usage was only 40 — refund 40 tokens
	rl.RecordActualTokens(80, 40)

	// Now bucket should have ~60 tokens available, so a 50-token request should succeed
	ctxOk := context.Background()
	if err := rl.Wait(ctxOk, 50); err != nil {
		t.Errorf("Wait() after refund error: %v (refund should have freed tokens)", err)
	}
}

func TestRateLimiter_RecordActualTokens_NoOp_WhenUnlimited(t *testing.T) {
	// tpm=0 means no TPM limit; RecordActualTokens should be a no-op
	rl := NewRateLimiter(0, 0)
	// Should not panic
	rl.RecordActualTokens(100, 50)
}

func TestRateLimiter_RecordActualTokens_NoOp_WhenSame(t *testing.T) {
	rl := NewRateLimiter(1000, 0)
	// Same estimate and actual — no-op
	rl.RecordActualTokens(100, 100)
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	// Very low TPM so the bucket drains immediately
	rl := NewRateLimiter(1, 0)

	ctx := context.Background()

	// Drain the bucket
	if err := rl.Wait(ctx, 1); err != nil {
		t.Fatalf("first Wait() error: %v", err)
	}

	// Cancel context while waiting
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := rl.Wait(cancelCtx, 1)
	if err == nil {
		t.Error("Wait with cancelled context should return error")
	}
}

func TestRateLimiter_BothLimits(t *testing.T) {
	// 1000 TPM, 10 RPM
	rl := NewRateLimiter(1000, 10)

	ctx := context.Background()

	// 10 small requests should pass immediately (RPM bucket full, TPM ample)
	start := time.Now()
	for i := 0; i < 10; i++ {
		if err := rl.Wait(ctx, 10); err != nil {
			t.Fatalf("Wait() %d error: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("10 requests took %v, want < 200ms", elapsed)
	}

	// 11th request should block (RPM exhausted)
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := rl.Wait(ctxTimeout, 10)
	if err == nil {
		t.Error("11th request should have blocked (RPM exhausted)")
	}
}
