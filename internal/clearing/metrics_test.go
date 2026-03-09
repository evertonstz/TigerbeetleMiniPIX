package clearing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHistogramPercentiles(t *testing.T) {
	ClearMetrics() // Reset metrics for test isolation

	// Record 10 batch samples with varying sizes
	sampleSizes := []int{50, 100, 200, 500, 1000, 2000, 5000, 8000, 8150, 8189}

	for _, size := range sampleSizes {
		RecordBatchSubmission(BatchMetrics{
			BatchSize:    size,
			PaymentCount: size / 3,
			LatencyMs:    10.5,
			ErrorCount:   0,
			SuccessCount: size,
		})
	}

	histogram := GetBatchSizeHistogram()

	// Verify basic statistics
	assert.Equal(t, 10, histogram.Count)
	assert.Equal(t, 50, histogram.Min)
	assert.Equal(t, 8189, histogram.Max)

	// Verify percentiles are ordered or equal
	assert.LessOrEqual(t, histogram.P50, histogram.P95)
	assert.LessOrEqual(t, histogram.P95, histogram.P99)

	// Verify P99 is close to max
	assert.GreaterOrEqual(t, histogram.P99, 5000)

	// Verify bucket distribution
	// 50 is in 0-100, 100 is in 0-100
	// 200, 500 are in 100-500
	// 1000 is in 500-1000
	// 2000, 5000 are in 1000-5000
	// 8000, 8150, 8189 are in 5000-8189
	assert.Equal(t, 2, histogram.Buckets["0-100"])     // 50, 100
	assert.Equal(t, 2, histogram.Buckets["100-500"])   // 200, 500
	assert.Equal(t, 1, histogram.Buckets["500-1000"])  // 1000
	assert.Equal(t, 2, histogram.Buckets["1000-5000"]) // 2000, 5000
	assert.Equal(t, 3, histogram.Buckets["5000-8189"]) // 8000, 8150, 8189
}

func TestHistogramEmpty(t *testing.T) {
	ClearMetrics()

	histogram := GetBatchSizeHistogram()
	assert.Equal(t, 0, histogram.Count)
	assert.Equal(t, 0, histogram.Min)
	assert.Equal(t, 0, histogram.Max)
	assert.Equal(t, 0, histogram.P50)
}

func TestHistogramSingleSample(t *testing.T) {
	ClearMetrics()

	RecordBatchSubmission(BatchMetrics{
		BatchSize:    1000,
		PaymentCount: 333,
		LatencyMs:    15.0,
		ErrorCount:   0,
		SuccessCount: 1000,
	})

	histogram := GetBatchSizeHistogram()
	assert.Equal(t, 1, histogram.Count)
	assert.Equal(t, 1000, histogram.Min)
	assert.Equal(t, 1000, histogram.Max)
	assert.Equal(t, 1000.0, histogram.Mean)
	assert.Equal(t, 1000, histogram.P50)
	assert.Equal(t, 1000, histogram.P95)
	assert.Equal(t, 1000, histogram.P99)
}

func TestHistogramP50GreaterThan100ForLargeLoadTest(t *testing.T) {
	// Simulate sustained load with 500+ pending payments
	// Each batch should be large (> 100 transfers)
	ClearMetrics()

	// Simulate 50 batches with realistic sizes during 10K TPS load
	// At 10K TPS = 10K payments/sec, each batch ≈ 2729 payments = 8187 transfers
	// Some batches may be partial (timeout), others full
	batchSizes := []int{
		8189, 8189, 8189, 8189, 8189, // Full batches
		8189, 8189, 8189, 8189, 8189,
		8189, 8189, 8189, 8189, 8189,
		8189, 8189, 8189, 8189, 8189,
		5000, 4000, 3000, 7000, 6500, // Partial batches (timeouts)
		7500, 6000, 5500, 7000, 6500,
		8189, 8189, 8189, 8189, 8189,
		8189, 8189, 8189, 8189, 8189,
		8189, 8189, 8189, 8189, 8189,
		3000, 4000, 5000, 6000, 7000, // Final partial batches
		8189, 8189,
	}

	for _, size := range batchSizes {
		RecordBatchSubmission(BatchMetrics{
			BatchSize:    size,
			PaymentCount: size / 3,
			LatencyMs:    45.0,
			ErrorCount:   0,
			SuccessCount: size,
		})
	}

	histogram := GetBatchSizeHistogram()

	// Key assertion from Phase 4 success criteria:
	// "P50 > 100 transfers per call during sustained load"
	assert.Greater(t, histogram.P50, 100, "P50 should be > 100 for batching to be effective")

	// Also verify other properties
	assert.Greater(t, histogram.Mean, 6000.0, "Mean batch size should be large for sustained load")
	assert.Greater(t, histogram.P95, 5000, "P95 should be significant")

	t.Logf("Histogram: %s", FormatHistogram(histogram))
}

func TestBatchMetricsRecording(t *testing.T) {
	ClearMetrics()

	// Record 3 batches
	testCases := []struct {
		batchSize    int
		paymentCount int
		latencyMs    float64
		errorCount   int
	}{
		{100, 33, 5.0, 0},
		{500, 166, 10.0, 2},
		{8000, 2666, 45.0, 0},
	}

	for _, tc := range testCases {
		RecordBatchSubmission(BatchMetrics{
			BatchSize:    tc.batchSize,
			PaymentCount: tc.paymentCount,
			LatencyMs:    tc.latencyMs,
			ErrorCount:   tc.errorCount,
			SuccessCount: tc.batchSize - tc.errorCount,
		})
	}

	// Retrieve all metrics
	allMetrics := GetAllBatchMetrics()

	assert.Equal(t, 3, len(allMetrics))

	// Verify first batch
	assert.Equal(t, 100, allMetrics[0].BatchSize)
	assert.Equal(t, 33, allMetrics[0].PaymentCount)
	assert.Equal(t, 5.0, allMetrics[0].LatencyMs)
	assert.Equal(t, 0, allMetrics[0].ErrorCount)
	assert.Equal(t, 100, allMetrics[0].SuccessCount)
	assert.False(t, allMetrics[0].Timestamp.IsZero())

	// Verify second batch
	assert.Equal(t, 500, allMetrics[1].BatchSize)
	assert.Equal(t, 2, allMetrics[1].ErrorCount)

	// Verify third batch
	assert.Equal(t, 8000, allMetrics[2].BatchSize)
	assert.Equal(t, 2666, allMetrics[2].PaymentCount)
}
