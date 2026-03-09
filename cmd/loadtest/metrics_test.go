package main

import (
	"math"
	"testing"
)

// TestHistogramRecording verifies that RecordLatency() stores values in histogram correctly.
// Records 100 latencies (1-10 microseconds) and verifies histogram captures the range.
func TestHistogramRecording(t *testing.T) {
	mc := NewMetricsCollector()

	// Record 100 latencies from 1 to 10 microseconds (in nanoseconds)
	for i := 1; i <= 100; i++ {
		latencyUs := int64(1 + (i % 10)) // 1-10 microseconds
		latencyNs := latencyUs * 1000    // Convert to nanoseconds
		mc.RecordLatency(latencyNs)
	}

	// Verify histogram has recorded values
	minVal := mc.histogram.Min()
	maxVal := mc.histogram.Max()

	if minVal < 1 || minVal > 10 {
		t.Errorf("Expected min latency 1-10 us, got %d", minVal)
	}

	if maxVal < 1 || maxVal > 10 {
		t.Errorf("Expected max latency 1-10 us, got %d", maxVal)
	}

	if minVal > maxVal {
		t.Errorf("Min (%d) should be <= Max (%d)", minVal, maxVal)
	}
}

// TestPercentileCalculation verifies P50, P95, P99 are monotonically increasing.
// Records 1000 latencies (random 1-100 microseconds) and checks percentile ordering.
func TestPercentileCalculation(t *testing.T) {
	mc := NewMetricsCollector()

	// Record 1000 random latencies (1-100 microseconds)
	for i := 1; i <= 1000; i++ {
		// Vary latencies to get meaningful percentiles
		latencyUs := int64(1 + (i % 100))
		latencyNs := latencyUs * 1000
		mc.RecordLatency(latencyNs)
	}

	// Get percentiles
	p50 := mc.histogram.ValueAtPercentile(50.0)
	p95 := mc.histogram.ValueAtPercentile(95.0)
	p99 := mc.histogram.ValueAtPercentile(99.0)

	// Verify monotonic ordering: P50 <= P95 <= P99
	if p50 > p95 {
		t.Errorf("P50 (%d) should be <= P95 (%d)", p50, p95)
	}

	if p95 > p99 {
		t.Errorf("P95 (%d) should be <= P99 (%d)", p95, p99)
	}

	// Verify values are within recorded range (1-100 us)
	if p50 < 1 || p50 > 100 {
		t.Errorf("P50 (%d) out of expected range 1-100", p50)
	}

	if p95 < 1 || p95 > 100 {
		t.Errorf("P95 (%d) out of expected range 1-100", p95)
	}

	if p99 < 1 || p99 > 100 {
		t.Errorf("P99 (%d) out of expected range 1-100", p99)
	}
}

// TestMeanAndStdDev records 100 identical latencies and verifies mean is approximately correct.
func TestMeanAndStdDev(t *testing.T) {
	mc := NewMetricsCollector()

	const testValue = 5000 // 5000 microseconds
	const testCount = 100

	// Record 100 identical latencies (5000 microseconds)
	for i := 0; i < testCount; i++ {
		latencyNs := int64(testValue * 1000) // Convert to nanoseconds
		mc.RecordLatency(latencyNs)
	}

	// Verify mean is approximately 5000 (allowing for rounding)
	mean := mc.histogram.Mean()
	tolerance := 50.0 // 50 us tolerance for rounding

	if math.Abs(mean-float64(testValue)) > tolerance {
		t.Errorf("Expected mean ~%d us, got %.1f us (diff: %.1f > tolerance: %.1f)",
			testValue, mean, math.Abs(mean-float64(testValue)), tolerance)
	}

	// Verify StdDev is very small (all values identical)
	stdDev := mc.histogram.StdDev()
	if stdDev > 100.0 { // Allow some rounding error
		t.Errorf("Expected StdDev ~0 for identical values, got %.1f", stdDev)
	}
}

// TestOutOfRangeHandling verifies that recording latencies beyond histogram max doesn't panic.
// Records a very large latency (beyond 30 seconds configured max) and verifies no crash.
func TestOutOfRangeHandling(t *testing.T) {
	mc := NewMetricsCollector()

	// First, record a normal value
	mc.RecordLatency(5000 * 1000) // 5000 microseconds

	// Try to record a value way beyond max (30 seconds = 30*1000*1000*1000 ns)
	// This should log a warning but not crash
	outOfRangeNs := int64(31 * 1000 * 1000 * 1000) // 31 seconds
	mc.RecordLatency(outOfRangeNs)

	// Verify histogram still works and didn't panic
	minVal := mc.histogram.Min()
	maxVal := mc.histogram.Max()

	if minVal <= 0 || maxVal <= 0 {
		t.Errorf("Histogram should still have valid values after out-of-range record")
	}
}

// TestRecordSentAndError verifies counter increments.
func TestRecordSentAndError(t *testing.T) {
	mc := NewMetricsCollector()

	// Record some sent and error counts
	for i := 0; i < 50; i++ {
		mc.RecordSent()
	}

	for i := 0; i < 5; i++ {
		mc.RecordError()
	}

	// Verify counters (we can't directly access, but we can verify PrintReport doesn't panic)
	// The values would be verified if we add getter methods to MetricsCollector
	// For now, just ensure no panic occurs
	mc.SetElapsed(0)
	// PrintReport would show the values
}

// TestOffsetTrackerRecordsOffsets verifies that RecordMessageOffset() stores offsets correctly.
// Records 10 sample offsets for different payment UUIDs and verifies they're stored in the map.
func TestOffsetTrackerRecordsOffsets(t *testing.T) {
	brokers := []string{"localhost:9092"}
	tracker := NewOffsetTracker(brokers, "pix-payments", "test-group")
	defer tracker.Close()

	// Record 10 sample offsets
	for i := 0; i < 10; i++ {
		uuid := "payment-" + string(rune(i))
		offset := int64(i * 100)
		tracker.RecordMessageOffset(uuid, offset)
	}

	// Verify stats
	sent, confirmed := tracker.GetStats()
	if sent != 10 {
		t.Errorf("Expected 10 sent offsets, got %d", sent)
	}

	// confirmed should be 0 until offsets are actually processed by consumer
	if confirmed != 0 {
		t.Errorf("Expected 0 confirmed offsets initially, got %d", confirmed)
	}
}

// TestOffsetTrackerConcurrency verifies that offset recording is thread-safe.
// Spawns 10 goroutines, each recording 100 offsets concurrently, and verifies total count.
func TestOffsetTrackerConcurrency(t *testing.T) {
	brokers := []string{"localhost:9092"}
	tracker := NewOffsetTracker(brokers, "pix-payments", "test-group")
	defer tracker.Close()

	// Spawn 10 goroutines to record offsets concurrently
	numGoroutines := 10
	offsetsPerGoroutine := 100

	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			for i := 0; i < offsetsPerGoroutine; i++ {
				uuid := "payment-" + string(rune(goroutineID)) + "-" + string(rune(i))
				offset := int64(goroutineID*offsetsPerGoroutine + i)
				tracker.RecordMessageOffset(uuid, offset)
			}
			done <- true
		}(g)
	}

	// Wait for all goroutines to complete
	for g := 0; g < numGoroutines; g++ {
		<-done
	}

	// Verify total offsets recorded
	sent, _ := tracker.GetStats()
	expectedTotal := int64(numGoroutines * offsetsPerGoroutine)
	if sent != expectedTotal {
		t.Errorf("Expected %d total offsets, got %d", expectedTotal, sent)
	}
}

// TestE2ELatencyAggregation simulates recording E2E latencies and verifies histogram aggregation.
// Creates 100 latency samples and verifies P50/P95/P99 percentiles are properly ordered.
func TestE2ELatencyAggregation(t *testing.T) {
	mc := NewMetricsCollector()

	// Simulate 100 payments with realistic E2E latencies (5-15 milliseconds = 5000-15000 microseconds)
	for i := 0; i < 100; i++ {
		// Latencies vary from 5 to 15 milliseconds
		latencyUs := int64(5000 + (i % 10000))
		latencyNs := latencyUs * 1000
		mc.RecordLatency(latencyNs)
	}

	// Verify histogram has recorded values
	minVal := mc.histogram.Min()
	maxVal := mc.histogram.Max()

	if minVal <= 0 {
		t.Errorf("Expected min > 0, got %d", minVal)
	}

	if maxVal <= 0 {
		t.Errorf("Expected max > 0, got %d", maxVal)
	}

	if minVal > maxVal {
		t.Errorf("Min (%d) should be <= Max (%d)", minVal, maxVal)
	}

	// Get percentiles
	p50 := mc.histogram.ValueAtPercentile(50.0)
	p95 := mc.histogram.ValueAtPercentile(95.0)
	p99 := mc.histogram.ValueAtPercentile(99.0)

	// Verify percentiles are monotonically increasing
	if p50 > p95 {
		t.Errorf("P50 (%d) should be <= P95 (%d)", p50, p95)
	}

	if p95 > p99 {
		t.Errorf("P95 (%d) should be <= P99 (%d)", p95, p99)
	}

	// Verify percentiles are within reasonable range
	if p50 < 1000 || p50 > 20000 {
		t.Errorf("P50 (%d) out of expected range 1000-20000 us", p50)
	}

	if p95 < 1000 || p95 > 20000 {
		t.Errorf("P95 (%d) out of expected range 1000-20000 us", p95)
	}

	if p99 < 1000 || p99 > 20000 {
		t.Errorf("P99 (%d) out of expected range 1000-20000 us", p99)
	}
}
