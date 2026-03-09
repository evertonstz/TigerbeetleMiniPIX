package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// MetricsCollector tracks performance metrics during load test execution using HDR Histogram.
type MetricsCollector struct {
	mu           sync.Mutex
	histogram    *hdrhistogram.Histogram // Producer latency (send → ProduceSync only)
	e2eHistogram *hdrhistogram.Histogram // E2E latency (send → offset confirmed)
	startTime    time.Time
	elapsed      time.Duration
	totalSent    int64
	errorCount   int64
}

// NewMetricsCollector creates a new metrics collector with HDR histogram.
// Histogram tracks latencies in microseconds, range 1 us to 30 seconds, 3 significant figures.
func NewMetricsCollector() *MetricsCollector {
	hist := hdrhistogram.New(1, 30*1000*1000, 3)
	e2eHist := hdrhistogram.New(1, 30*1000*1000, 3)

	return &MetricsCollector{
		histogram:    hist,
		e2eHistogram: e2eHist,
		startTime:    time.Now(),
	}
}

// RecordLatency records end-to-end latency in nanoseconds.
// Converts to microseconds for histogram and gracefully handles out-of-range values.
func (mc *MetricsCollector) RecordLatency(latencyNs int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	latencyUs := latencyNs / 1000
	if err := mc.histogram.RecordValue(latencyUs); err != nil {
		log.Printf("Warning: latency out of histogram range: %d us (error: %v)", latencyUs, err)
	}
}

// RecordE2ELatency records true end-to-end latency (send → offset confirmed) in nanoseconds.
// Converts to microseconds for histogram and gracefully handles out-of-range values.
func (mc *MetricsCollector) RecordE2ELatency(latencyNs int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	latencyUs := latencyNs / 1000
	if err := mc.e2eHistogram.RecordValue(latencyUs); err != nil {
		log.Printf("Warning: E2E latency out of histogram range: %d us (error: %v)", latencyUs, err)
	}
}

// RecordSent increments the sent counter.
func (mc *MetricsCollector) RecordSent() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.totalSent++
}

// RecordError increments the error counter.
func (mc *MetricsCollector) RecordError() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.errorCount++
}

// SetElapsed records the total test duration.
func (mc *MetricsCollector) SetElapsed(d time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.elapsed = d
}

// GetTotalSent returns the count of successfully sent messages.
func (mc *MetricsCollector) GetTotalSent() int64 {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.totalSent
}

// PrintReport outputs a formatted benchmark report with TPS and latency percentiles.
func (mc *MetricsCollector) PrintReport() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	elapsedSec := mc.elapsed.Seconds()
	var tps float64
	if elapsedSec > 0 {
		tps = float64(mc.totalSent) / elapsedSec
	}

	// Producer latency metrics
	p50 := mc.histogram.ValueAtPercentile(50.0)
	p95 := mc.histogram.ValueAtPercentile(95.0)
	p99 := mc.histogram.ValueAtPercentile(99.0)
	minLatency := mc.histogram.Min()
	maxLatency := mc.histogram.Max()
	meanLatency := mc.histogram.Mean()
	stdDev := mc.histogram.StdDev()

	// E2E latency metrics
	e2eP50 := mc.e2eHistogram.ValueAtPercentile(50.0)
	e2eP95 := mc.e2eHistogram.ValueAtPercentile(95.0)
	e2eP99 := mc.e2eHistogram.ValueAtPercentile(99.0)
	e2eMinLatency := mc.e2eHistogram.Min()
	e2eMaxLatency := mc.e2eHistogram.Max()
	e2eMeanLatency := mc.e2eHistogram.Mean()
	e2eStdDev := mc.e2eHistogram.StdDev()

	fmt.Printf(`
=== Benchmark Report (Load Test) ===
Total Payments Sent:    %d
Total Errors:           %d
Total Time:             %.2f seconds
Achieved TPS:           %.1f

=== Producer Latencies (send → ProduceSync) ===
  P50:                  %d µs
  P95:                  %d µs
  P99:                  %d µs
  Min:                  %d µs
  Max:                  %d µs
  Mean:                 %.1f µs
  StdDev:               %.1f µs

=== E2E Latencies (send → Phase 2 confirmation) ===
  P50:                  %d µs
  P95:                  %d µs
  P99:                  %d µs
  Min:                  %d µs
  Max:                  %d µs
  Mean:                 %.1f µs
  StdDev:               %.1f µs

==================================
`, mc.totalSent, mc.errorCount, elapsedSec, tps,
		p50, p95, p99, minLatency, maxLatency, meanLatency, stdDev,
		e2eP50, e2eP95, e2eP99, e2eMinLatency, e2eMaxLatency, e2eMeanLatency, e2eStdDev)
}
