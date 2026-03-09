package clearing

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// BatchMetrics records statistics for a single batch submission
type BatchMetrics struct {
	BatchSize    int       // Number of transfers submitted
	PaymentCount int       // Number of payments in batch
	LatencyMs    float64   // Submission latency in milliseconds
	ErrorCount   int       // Number of failed transfers in response
	SuccessCount int       // Number of successful transfers
	Timestamp    time.Time // When batch was submitted
}

// GlobalMetrics stores all collected batch metrics (thread-safe)
type GlobalMetrics struct {
	mu           sync.Mutex
	batchSamples []BatchMetrics
}

var gm = &GlobalMetrics{
	batchSamples: make([]BatchMetrics, 0),
}

// RecordBatchSubmission records metrics for a batch submission
func RecordBatchSubmission(metrics BatchMetrics) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	metrics.Timestamp = time.Now()
	gm.batchSamples = append(gm.batchSamples, metrics)
}

// Percentile represents a percentile value (0.5 = P50, 0.95 = P95, etc.)
type Percentile struct {
	Value    float64 // The percentile value (e.g., P50)
	Percent  float64 // The percentile rank (e.g., 0.5 for P50)
	BatchMin int     // Minimum batch size at this percentile
	BatchMax int     // Maximum batch size at this percentile
}

// HistogramResult represents batch size distribution statistics
type HistogramResult struct {
	Min     int
	Max     int
	Mean    float64
	Count   int
	P50     int
	P95     int
	P99     int
	Buckets map[string]int // Bucket name -> count (e.g., "0-100": 5, "100-500": 20)
}

func GetBatchSizeHistogram() HistogramResult {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if len(gm.batchSamples) == 0 {
		return HistogramResult{}
	}

	sizes := make([]int, len(gm.batchSamples))
	for i, m := range gm.batchSamples {
		sizes[i] = m.BatchSize
	}
	sort.Ints(sizes)

	min := sizes[0]
	max := sizes[len(sizes)-1]
	sum := 0
	for _, s := range sizes {
		sum += s
	}
	mean := float64(sum) / float64(len(sizes))

	p50 := percentileValue(sizes, 0.50)
	p95 := percentileValue(sizes, 0.95)
	p99 := percentileValue(sizes, 0.99)

	buckets := map[string]int{
		"0-100":     0,
		"100-500":   0,
		"500-1000":  0,
		"1000-5000": 0,
		"5000-8189": 0,
	}
	for _, size := range sizes {
		if size <= 100 {
			buckets["0-100"]++
		} else if size <= 500 {
			buckets["100-500"]++
		} else if size <= 1000 {
			buckets["500-1000"]++
		} else if size <= 5000 {
			buckets["1000-5000"]++
		} else {
			buckets["5000-8189"]++
		}
	}

	return HistogramResult{
		Min:     min,
		Max:     max,
		Mean:    mean,
		Count:   len(sizes),
		P50:     p50,
		P95:     p95,
		P99:     p99,
		Buckets: buckets,
	}
}

// percentileValue calculates the percentile value from sorted slice
// percentile: 0.5 = P50, 0.95 = P95, 0.99 = P99
func percentileValue(sorted []int, percentile float64) int {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1) * percentile)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// GetAllBatchMetrics returns all recorded batch metrics (for testing)
func GetAllBatchMetrics() []BatchMetrics {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	result := make([]BatchMetrics, len(gm.batchSamples))
	copy(result, gm.batchSamples)
	return result
}

// ClearMetrics resets all metrics (useful for test isolation)
func ClearMetrics() {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.batchSamples = gm.batchSamples[:0]
}

// FormatHistogram returns human-readable histogram
func FormatHistogram(h HistogramResult) string {
	return fmt.Sprintf(
		"Batch Size Histogram: min=%d, max=%d, mean=%.1f, count=%d, P50=%d, P95=%d, P99=%d\nBuckets: %v",
		h.Min, h.Max, h.Mean, h.Count, h.P50, h.P95, h.P99, h.Buckets,
	)
}
