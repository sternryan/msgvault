package ai

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// ProgressReporter displays live pipeline progress with count, cost, rate, and ETA.
type ProgressReporter struct {
	mu            sync.Mutex
	startTime     time.Time
	totalMessages int64
	processed     int64
	skipped       int64
	failed        int64
	tokensInput   int64
	tokensOutput  int64
	costUSD       float64
	lastPrintTime time.Time
	printInterval time.Duration
	pipelineType  string
}

// NewProgressReporter creates a progress reporter.
func NewProgressReporter(pipelineType string, totalMessages int64) *ProgressReporter {
	return &ProgressReporter{
		startTime:     time.Now(),
		totalMessages: totalMessages,
		printInterval: 2 * time.Second,
		pipelineType:  pipelineType,
		lastPrintTime: time.Now(),
	}
}

// Update records batch completion stats and prints progress if interval elapsed.
func (p *ProgressReporter) Update(processed, skipped, failed, tokensIn, tokensOut int64, costUSD float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.processed += processed
	p.skipped += skipped
	p.failed += failed
	p.tokensInput += tokensIn
	p.tokensOutput += tokensOut
	p.costUSD += costUSD

	now := time.Now()
	if now.Sub(p.lastPrintTime) >= p.printInterval {
		p.printProgress(now)
		p.lastPrintTime = now
	}
}

// Finish prints the final summary line.
func (p *ProgressReporter) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.printProgress(time.Now())
	elapsed := time.Since(p.startTime)
	fmt.Fprintf(os.Stderr, "\n%s complete: %d processed, %d skipped, %d failed in %s — $%.4f\n",
		p.pipelineType, p.processed, p.skipped, p.failed,
		elapsed.Round(time.Second), p.costUSD)
}

// Stats returns current progress counters (for testing and programmatic use).
func (p *ProgressReporter) Stats() (processed, skipped, failed, tokensIn, tokensOut int64, costUSD float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.processed, p.skipped, p.failed, p.tokensInput, p.tokensOutput, p.costUSD
}

func (p *ProgressReporter) printProgress(now time.Time) {
	elapsed := now.Sub(p.startTime)
	done := p.processed + p.skipped + p.failed
	remaining := p.totalMessages - done

	// Rate: tokens per second
	var tokPerSec float64
	if elapsed.Seconds() > 0 {
		tokPerSec = float64(p.tokensInput+p.tokensOutput) / elapsed.Seconds()
	}

	// ETA based on messages/sec
	var eta string
	if done > 0 && remaining > 0 {
		msgPerSec := float64(done) / elapsed.Seconds()
		etaDur := time.Duration(float64(remaining)/msgPerSec) * time.Second
		eta = etaDur.Round(time.Second).String()
	} else if remaining <= 0 {
		eta = "done"
	} else {
		eta = "calculating..."
	}

	// Carriage return for overwrite-in-place on terminals
	fmt.Fprintf(os.Stderr, "\r  [%s] %d/%d msgs | $%.4f | %.0f tok/s | ETA %s   ",
		p.pipelineType, done, p.totalMessages, p.costUSD, tokPerSec, eta)
}
