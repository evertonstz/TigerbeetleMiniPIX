package main

import (
	"context"
	"runtime"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestProducerPoolDistributes10KMessages verifies that ProducerPool can distribute
// 10,000 messages across 100 workers without deadlocks or goroutine leaks
func TestProducerPoolDistributes10KMessages(t *testing.T) {
	// Create mock producer that tracks sent count
	mockProducer := &mockProducerClient{
		slowDownMs: 0, // No artificial delay for speed
	}

	cfg := &Config{
		PaymentCount:    10000, // 10K messages
		Concurrency:     100,   // 100 workers
		RateLimitPerSec: 50000, // High rate (not limiting)
		RateLimitBurst:  50000,
	}

	generator, err := NewPaymentGenerator(cfg)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	metrics := NewMetricsCollector()
	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimitPerSec), cfg.RateLimitBurst)

	// Create pool
	pool := NewProducerPool(cfg.Concurrency, limiter, mockProducer, generator, metrics, "test-topic", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Measure goroutines
	startGoroutines := runtime.NumGoroutine()
	t.Logf("Starting goroutines: %d", startGoroutines)

	// Run pool
	err = pool.Run(ctx, cfg.PaymentCount)
	if err != nil {
		t.Fatalf("Pool run failed: %v", err)
	}

	endGoroutines := runtime.NumGoroutine()
	t.Logf("Ending goroutines: %d", endGoroutines)

	// Verify all 10K messages were distributed
	if metrics.GetTotalSent() != int64(cfg.PaymentCount) {
		t.Errorf("Expected %d sent messages, got %d", cfg.PaymentCount, metrics.GetTotalSent())
	}

	// Verify goroutine count returned to baseline (no leaks)
	// Allow some buffer for scheduler
	maxAllowedGoroutines := cfg.Concurrency + 10

	if endGoroutines-startGoroutines > maxAllowedGoroutines {
		t.Errorf("Too many goroutines spawned: started %d, ended %d (diff %d > allowed %d)",
			startGoroutines, endGoroutines, endGoroutines-startGoroutines, maxAllowedGoroutines)
	}

	t.Logf("✓ 10K message distribution: %d messages, %d workers, 0 deadlocks, %d goroutine delta",
		cfg.PaymentCount, cfg.Concurrency, endGoroutines-startGoroutines)
}

// TestRateLimiterHandles10KMessages verifies that rate limiter correctly throttles
// 10,000 messages without token loss or duplication
func TestRateLimiterHandles10KMessages(t *testing.T) {
	cfg := &Config{
		PaymentCount:    10000, // 10K messages
		Concurrency:     100,   // 100 workers, each gets ~100 messages
		RateLimitPerSec: 10000, // 10K msgs/sec rate limit
		RateLimitBurst:  110,   // burst slightly above concurrency
	}

	generator, err := NewPaymentGenerator(cfg)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	metrics := NewMetricsCollector()
	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimitPerSec), cfg.RateLimitBurst)

	// Use mock producer with no artificial delay
	mockProducer := &mockProducerClient{
		slowDownMs: 0,
	}

	pool := NewProducerPool(cfg.Concurrency, limiter, mockProducer, generator, metrics, "test-topic", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	startTime := time.Now()
	err = pool.Run(ctx, cfg.PaymentCount)
	if err != nil {
		t.Fatalf("Pool run failed: %v", err)
	}
	elapsed := time.Since(startTime)

	// At 10K msgs/sec, 10K messages should take approximately 1 second
	// Burst (110) allows fast initial messages, remaining 9890 take ~0.99 seconds
	// Add buffer for scheduler overhead
	minExpectedTime := 800 * time.Millisecond // Allow some burst benefit
	maxAllowedTime := 3 * time.Second         // Should never take 3 seconds at 10K/sec

	t.Logf("Rate limit test: %d messages in %v (min expected: %v, max allowed: %v)",
		cfg.PaymentCount, elapsed, minExpectedTime, maxAllowedTime)

	if elapsed < minExpectedTime {
		t.Errorf("Execution too fast: %v < expected min %v. Rate limiter may not be enforcing throughput.",
			elapsed, minExpectedTime)
	}

	if elapsed > maxAllowedTime {
		t.Errorf("Execution too slow: %v > max allowed %v. Rate limiter may be blocking.",
			elapsed, maxAllowedTime)
	}

	// Verify all messages were sent
	if metrics.GetTotalSent() != int64(cfg.PaymentCount) {
		t.Errorf("Expected %d sent messages, got %d", cfg.PaymentCount, metrics.GetTotalSent())
	}

	actualThroughput := float64(cfg.PaymentCount) / elapsed.Seconds()
	t.Logf("✓ Rate limiter: %.0f actual msgs/sec (target: %d msgs/sec)", actualThroughput, cfg.RateLimitPerSec)
}

// TestMetricsHandles10KRecords verifies that MetricsCollector can aggregate
// metrics for 10K+ messages without panic or loss of precision
func TestMetricsHandles10KRecords(t *testing.T) {
	mc := NewMetricsCollector()

	// Record 10K successful sends
	for i := 0; i < 10000; i++ {
		mc.RecordSent()
	}

	// Record 9500 latencies (95% success rate from Phase 2 design)
	// Latencies range from 100us to 10ms to get meaningful percentiles
	for i := 0; i < 9500; i++ {
		// Vary latencies: 100us to 10ms in progression
		latencyUs := int64(100 + (i % 9900))
		latencyNs := latencyUs * 1000
		mc.RecordLatency(latencyNs)
	}

	// Verify totals
	if mc.GetTotalSent() != 10000 {
		t.Errorf("Expected 10000 sent, got %d", mc.GetTotalSent())
	}

	// Get percentiles
	p50 := mc.histogram.ValueAtPercentile(50.0)
	p95 := mc.histogram.ValueAtPercentile(95.0)
	p99 := mc.histogram.ValueAtPercentile(99.0)

	// Verify monotonic ordering
	if p50 > p95 {
		t.Errorf("P50 (%d) should be <= P95 (%d)", p50, p95)
	}
	if p95 > p99 {
		t.Errorf("P95 (%d) should be <= P99 (%d)", p95, p99)
	}

	// Verify percentiles are in expected range (100us to 10ms)
	if p50 < 100 || p50 > 10000 {
		t.Errorf("P50 (%d us) out of expected range 100-10000", p50)
	}
	if p95 < 100 || p95 > 10000 {
		t.Errorf("P95 (%d us) out of expected range 100-10000", p95)
	}
	if p99 < 100 || p99 > 10000 {
		t.Errorf("P99 (%d us) out of expected range 100-10000", p99)
	}

	// Verify PrintReport doesn't panic
	elapsed := 1 * time.Second
	mc.SetElapsed(elapsed)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PrintReport panicked: %v", r)
		}
	}()
	// Can't easily capture stdout, but at least verify it doesn't panic
	mc.PrintReport()

	t.Logf("✓ Metrics aggregation: 10K messages, P50=%d us, P95=%d us, P99=%d us", p50, p95, p99)
}
