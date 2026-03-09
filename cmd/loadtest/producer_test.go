package main

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/time/rate"
)

// TestWorkerPoolConcurrencyLimit verifies that ProducerWorker properly limits concurrent goroutines.
// We create 10 workers that each spend time in message production, and verify no goroutine explosion.
func TestWorkerPoolConcurrencyLimit(t *testing.T) {
	// Create a mock producer that simulates slow produce (blocking for measurement)
	mockProducer := &mockProducerClient{
		slowDownMs: 20, // Each message takes 20ms to "produce"
	}

	cfg := &Config{
		PaymentCount:    100,   // 100 messages total
		Concurrency:     10,    // Fixed 10 workers
		RateLimitPerSec: 10000, // High rate (not limiting)
		RateLimitBurst:  10000,
	}

	generator, err := NewPaymentGenerator(cfg)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	metrics := NewMetricsCollector()
	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimitPerSec), cfg.RateLimitBurst)

	// Create pool (tracker is nil for testing without actual offset monitoring)
	pool := NewProducerPool(cfg.Concurrency, limiter, mockProducer, generator, metrics, "test-topic", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Measure goroutines during execution
	startGoroutines := runtime.NumGoroutine()
	t.Logf("Starting goroutines: %d", startGoroutines)

	// Run pool (this will take ~200ms: 100 messages * 20ms each, spread across 10 workers)
	err = pool.Run(ctx, cfg.PaymentCount)
	if err != nil {
		t.Fatalf("Pool run failed: %v", err)
	}

	endGoroutines := runtime.NumGoroutine()
	t.Logf("Ending goroutines: %d", endGoroutines)

	// Verify we didn't spawn unbounded goroutines
	// Expected: main thread + N workers + some background tasks
	// If workers spawn new goroutines per message (bug), we'd see >> 100 goroutines
	maxAllowedGoroutines := cfg.Concurrency + 10 // workers + buffer

	if endGoroutines-startGoroutines > maxAllowedGoroutines {
		t.Errorf("Too many goroutines spawned: started %d, ended %d (diff %d > allowed %d)",
			startGoroutines, endGoroutines, endGoroutines-startGoroutines, maxAllowedGoroutines)
	}

	// Verify all messages were produced
	if metrics.GetTotalSent() != int64(cfg.PaymentCount) {
		t.Errorf("Expected %d sent messages, got %d", cfg.PaymentCount, metrics.GetTotalSent())
	}
}

// TestRateLimiterTokenAcquisition verifies that rate limiter enforces target throughput.
// We limit to 100 msgs/sec, process 50 messages, and measure elapsed time.
// 50 msgs at 100 msgs/sec should take at least ~0.4 seconds (minus burst tokens).
func TestRateLimiterTokenAcquisition(t *testing.T) {
	cfg := &Config{
		PaymentCount:    50,  // Small batch
		Concurrency:     2,   // Few workers to control timing
		RateLimitPerSec: 100, // 100 messages per second
		RateLimitBurst:  5,   // Burst size allows fast start
	}

	generator, err := NewPaymentGenerator(cfg)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	metrics := NewMetricsCollector()
	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimitPerSec), cfg.RateLimitBurst)

	// Use a mock producer with no artificial delay
	mockProducer := &mockProducerClient{
		slowDownMs: 0,
	}

	pool := NewProducerPool(cfg.Concurrency, limiter, mockProducer, generator, metrics, "test-topic", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()
	err = pool.Run(ctx, cfg.PaymentCount)
	if err != nil {
		t.Fatalf("Pool run failed: %v", err)
	}
	elapsed := time.Since(startTime)

	// Expected time: 50 msgs / 100 msgs/sec = 0.5 seconds
	// However, burst (5) allows fast initial messages
	// Remaining 45 msgs at 100/sec = 0.45 seconds minimum
	// We add 50ms tolerance for scheduling overhead
	minExpectedTime := time.Duration(400) * time.Millisecond
	maxAllowedTime := time.Duration(2000) * time.Millisecond // sanity check (should never take 2 seconds)

	t.Logf("Rate limit test: %d messages in %v (min expected: %v, max allowed: %v)",
		cfg.PaymentCount, elapsed, minExpectedTime, maxAllowedTime)

	if elapsed < minExpectedTime {
		t.Errorf("Execution too fast: %v < expected min %v. Rate limiter may not be enforcing throughput.",
			elapsed, minExpectedTime)
	}

	if elapsed > maxAllowedTime {
		t.Errorf("Execution too slow: %v > max allowed %v. Workers may be blocked.",
			elapsed, maxAllowedTime)
	}

	// Verify all messages sent
	if metrics.GetTotalSent() != int64(cfg.PaymentCount) {
		t.Errorf("Expected %d sent messages, got %d", cfg.PaymentCount, metrics.GetTotalSent())
	}
}

// mockProducerClient implements ProducerClient for testing without Redpanda
type mockProducerClient struct {
	mu          sync.Mutex
	slowDownMs  int
	recordCount int64
}

// ProduceSync simulates synchronous produce with optional slowdown
func (m *mockProducerClient) ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	results := make(kgo.ProduceResults, len(rs))
	for i, r := range rs {
		atomic.AddInt64(&m.recordCount, 1)
		if m.slowDownMs > 0 {
			time.Sleep(time.Duration(m.slowDownMs) * time.Millisecond)
		}
		// Return a result with the record populated (including offset)
		recordCopy := *r
		recordCopy.Offset = int64(m.recordCount - 1)
		results[i] = kgo.ProduceResult{Record: &recordCopy}
	}
	return results
}

// Flush is a no-op for mock
func (m *mockProducerClient) Flush(ctx context.Context) error {
	return nil
}

// LeaveGroup is a no-op for mock
func (m *mockProducerClient) LeaveGroup() {
	// no-op
}

// Verify that mockProducerClient implements ProducerClient
var _ ProducerClient = (*mockProducerClient)(nil)
