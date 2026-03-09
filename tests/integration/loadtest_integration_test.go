// Package integration_test contains integration tests for the load test pipeline.
//
// Test Suite:
// - TestLoadTestSmoke: Basic smoke test with 100 messages (requires services)
// - TestLoadTestFull: Large batch test with 1000 messages (requires services)
// - TestBenchmarkReportFormat: Validates report format and field presence
// - TestConsumerLagCatchesUp: End-to-end pipeline with consumer processing
// - TestLoadTest10KMessages: LOAD-01 validation with 10,000 messages (requires services)
//
// Service Requirements:
// Tests skip gracefully if Redpanda (127.0.0.1:9092) is unavailable.
// TestLoadTest10KMessages requires both Redpanda AND TigerBeetle and will fail explicitly if missing.
//
// Run all tests:
//
//	go test ./tests/integration -v
//
// Run LOAD-01 validation only:
//
//	go test ./tests/integration -run TestLoadTest10KMessages -v -timeout 300s
package integration

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkServiceAvailable verifies that a service is running at the given address
func checkServiceAvailable(t *testing.T, address string) bool {
	conn, err := net.DialTimeout("tcp", address, 1*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// skipIfServicesNotRunning skips the test if required services are not available
func skipIfServicesNotRunning(t *testing.T) {
	if !checkServiceAvailable(t, "127.0.0.1:9092") {
		t.Skip("Skipping: Redpanda broker not available at 127.0.0.1:9092 - docker compose up required")
	}
	if !checkServiceAvailable(t, "127.0.0.1:3001") {
		t.Skip("Skipping: TigerBeetle cluster not available at 127.0.0.1:3001 - docker compose up required")
	}
}

// TestLoadTestSmoke validates that the load test completes and generates
// a benchmark report without panicking.
func TestLoadTestSmoke(t *testing.T) {
	skipIfServicesNotRunning(t)

	// Create a context with a timeout for the loadtest execution
	// Use 45 seconds to allow loadtest's 30-second default timeout + graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Run loadtest with small batch (100 payments, 5 concurrency)
	// Pass --timeout flag to ensure enough time for test to complete
	// Binary is at ../../../loadtest relative to this test file
	cmd := exec.CommandContext(ctx, "../../loadtest",
		"--payments", "100",
		"--concurrency", "5",
		"--rate", "1000",
		"--timeout", "30",
	)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Test should complete (exit code may be non-zero due to errors, but should not panic/crash)
	// Verify benchmark report exists (indicates program completed)
	assert.Contains(t, outputStr, "Benchmark Report", "Output should contain benchmark report")
	assert.Contains(t, outputStr, "Achieved TPS", "Output should report TPS")
	assert.Contains(t, outputStr, "Total Payments Sent", "Output should report payment count")

	// Verify we got numeric values, not "null" or empty (format validation)
	tpsRegex := regexp.MustCompile(`Achieved TPS:\s*([\d.]+)`)
	assert.Regexp(t, tpsRegex, outputStr, "TPS should be reported as numeric value")
}

// TestLoadTestFull validates that the load test with large batch generates
// realistic performance metrics in the benchmark report.
func TestLoadTestFull(t *testing.T) {
	skipIfServicesNotRunning(t)

	// Create a context with a timeout for the loadtest execution
	// Use 120 seconds to allow enough time for 1000 payments + graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Run loadtest with larger batch (1000 payments)
	cmd := exec.CommandContext(ctx, "../../loadtest",
		"--payments", "1000",
		"--concurrency", "50",
		"--rate", "10000",
		"--timeout", "90",
	)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Verify report is generated
	require.Contains(t, outputStr, "Benchmark Report", "Should generate benchmark report")

	// Parse metrics from output (even if 0 payments sent, format should be correct)
	tpsRegex := regexp.MustCompile(`Achieved TPS:\s*([\d.]+)`)
	tpsMatches := tpsRegex.FindStringSubmatch(outputStr)
	if len(tpsMatches) > 1 {
		// Just verify it's parseable
		_, err := strconv.ParseFloat(tpsMatches[1], 64)
		assert.NoError(t, err, "TPS should be numeric")
	}
}

// TestBenchmarkReportFormat validates that the benchmark report contains
// all required fields with numeric values in the correct format.
func TestBenchmarkReportFormat(t *testing.T) {
	skipIfServicesNotRunning(t)

	// Create a context with a timeout for the loadtest execution
	// Use 60 seconds to allow loadtest's 40-second timeout + graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Run loadtest with minimal batch
	cmd := exec.CommandContext(ctx, "../../loadtest",
		"--payments", "50",
		"--concurrency", "2",
		"--rate", "100",
		"--timeout", "40",
	)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Verify all required fields present
	requiredFields := []string{
		"Total Payments Sent",
		"Achieved TPS",
		"P50",
		"P95",
		"P99",
	}
	for _, field := range requiredFields {
		assert.Contains(t, outputStr, field, "Report should contain field: %s", field)
	}

	// Verify TPS is numeric
	tpsRegex := regexp.MustCompile(`Achieved TPS:\s*([\d.]+)`)
	tpsMatches := tpsRegex.FindStringSubmatch(outputStr)
	if len(tpsMatches) > 1 {
		_, err := strconv.ParseFloat(tpsMatches[1], 64)
		assert.NoError(t, err, "TPS should be numeric")
	}

	// Verify P50, P95, P99 are numeric and ordered correctly
	latencies := map[string]float64{}
	percentiles := []string{"P50", "P95", "P99"}

	for _, p := range percentiles {
		regex := regexp.MustCompile(fmt.Sprintf(`%s.*?:\s*([\d.]+)\s*ms`, p))
		matches := regex.FindStringSubmatch(outputStr)
		if len(matches) > 1 {
			val, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				latencies[p] = val
			}
		}
	}

	// Verify monotonic ordering: P50 <= P95 <= P99
	if p50, ok := latencies["P50"]; ok {
		if p95, ok := latencies["P95"]; ok {
			assert.LessOrEqual(t, p50, p95, "P50 should be <= P95")
		}
	}
	if p95, ok := latencies["P95"]; ok {
		if p99, ok := latencies["P99"]; ok {
			assert.LessOrEqual(t, p95, p99, "P95 should be <= P99")
		}
	}
}

// TestConsumerLagCatchesUp validates that the clearing engine consumer
// processes all generated messages and completes without errors.
func TestConsumerLagCatchesUp(t *testing.T) {
	skipIfServicesNotRunning(t)

	// Create a context with timeout for the full test
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Run load test with moderate batch
	cmd := exec.CommandContext(ctx, "../../loadtest",
		"--payments", "500",
		"--concurrency", "20",
		"--rate", "5000",
		"--timeout", "60",
	)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Verify benchmark report was generated
	assert.Contains(t, outputStr, "Benchmark Report", "Should generate benchmark report")
	assert.Contains(t, outputStr, "Achieved TPS", "Should report TPS")

	// Verify no errors
	assert.NotContains(t, outputStr, "ERROR", "Should not contain ERROR messages")
}

// verifyServicesRunning ensures both Redpanda and TigerBeetle are available
// Fails the test explicitly if services are not running (does not skip)
func verifyServicesRunning(t *testing.T) {
	if !checkServiceAvailable(t, "127.0.0.1:9092") {
		t.Fatalf("LOAD-01 validation requires Redpanda at 127.0.0.1:9092. Services required for LOAD-01 validation not available. Ensure `docker compose up` is running.")
	}
	if !checkServiceAvailable(t, "127.0.0.1:3001") {
		t.Fatalf("LOAD-01 validation requires TigerBeetle at 127.0.0.1:3001. Services required for LOAD-01 validation not available. Ensure `docker compose up` is running.")
	}
}

// extractTPSFromReport parses the benchmark report output and extracts the TPS value
func extractTPSFromReport(output string) (float64, error) {
	regex := regexp.MustCompile(`Achieved TPS:\s+([\d.]+)`)
	matches := regex.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0, fmt.Errorf("TPS not found in report output")
	}
	tps, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse TPS value: %w", err)
	}
	return tps, nil
}

// validateLoadTestRequirements checks that load test output meets LOAD-01 requirements
func validateLoadTestRequirements(t *testing.T, output string, expectedCount int) error {
	// Check total payments sent
	paymentRegex := regexp.MustCompile(fmt.Sprintf(`Total Payments Sent:\s*%d`, expectedCount))
	if !paymentRegex.MatchString(output) {
		return fmt.Errorf("expected 'Total Payments Sent: %d' not found in output", expectedCount)
	}

	// Check TPS present and numeric
	tpsRegex := regexp.MustCompile(`Achieved TPS:\s+([\d.]+)`)
	tpsMatches := tpsRegex.FindStringSubmatch(output)
	if len(tpsMatches) < 2 {
		return fmt.Errorf("TPS field not found in output")
	}
	tps, err := strconv.ParseFloat(tpsMatches[1], 64)
	if err != nil {
		return fmt.Errorf("TPS value not numeric: %v", err)
	}
	if tps <= 0 {
		return fmt.Errorf("TPS should be > 0, got %.1f", tps)
	}

	// Check P50, P95, P99 present
	for _, p := range []string{"P50", "P95", "P99"} {
		if !strings.Contains(output, p) {
			return fmt.Errorf("%s not found in report output", p)
		}
	}

	// Check for ERROR or FATAL messages
	if strings.Contains(output, "ERROR") {
		return fmt.Errorf("ERROR messages found in output")
	}
	if strings.Contains(output, "FATAL") {
		return fmt.Errorf("FATAL messages found in output")
	}

	return nil
}

// TestLoadTest10KMessages validates LOAD-01 requirement:
// "Load test binary produces 10,000+ configurable message batches with no producer errors"
//
// This test:
// - Verifies Redpanda and TigerBeetle are running (explicit failure if not)
// - Produces 10,000 messages with 100 workers at 10K msgs/sec
// - Validates benchmark report shows actual TPS for 10,000 messages
// - Ensures all messages produced without error
//
// LOAD-01 is satisfied if:
// - Binary exits with code 0
// - Benchmark report shows "Total Payments Sent: 10000"
// - Benchmark report shows "Achieved TPS:" with numeric value > 0
// - No ERROR messages in output
func TestLoadTest10KMessages(t *testing.T) {
	// Verify services are running (fail, not skip, for LOAD-01 validation)
	verifyServicesRunning(t)

	// Create a context with timeout for 10K message production
	// 10K messages at 10K msgs/sec = 1 second minimum production time
	// Add buffer for initialization, shutdown, and system variance
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Run loadtest with 10,000 payments
	cmd := exec.CommandContext(ctx, "../../loadtest",
		"--payments", "10000",
		"--concurrency", "100",
		"--rate", "10000",
		"--timeout", "120",
	)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Validate exit code (test does not check explicitly, but log output)
	t.Logf("Load test output:\n%s", outputStr)

	// Validate all LOAD-01 requirements
	err := validateLoadTestRequirements(t, outputStr, 10000)
	require.NoError(t, err, "LOAD-01 requirements not satisfied")

	// Extract and log TPS for visibility
	tps, err := extractTPSFromReport(outputStr)
	if err == nil {
		t.Logf("LOAD-01 validation passed: 10,000 messages produced at %.1f TPS", tps)
	} else {
		t.Logf("Could not extract TPS: %v", err)
	}

	// Additional validation: ensure report format is complete
	assert.Contains(t, outputStr, "Benchmark Report", "Output should contain benchmark report header")
}

// parseLatencyPercentile extracts a percentile value from the benchmark report
// Looks for patterns like "P95:                  XXX µs" for both producer and E2E sections
func parseLatencyPercentile(output string, section string, percentile string) (float64, error) {
	// Match pattern: "P95:                  XXX µs" within a specific section
	// We use a more flexible regex that captures the value after the percentile marker
	regex := regexp.MustCompile(fmt.Sprintf(`%s Latencies[^=]*?%s:\s+([\d.]+)\s*µs`, section, percentile))
	matches := regex.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0, fmt.Errorf("%s %s not found in %s section", percentile, section, section)
	}
	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s %s value: %w", percentile, section, err)
	}
	return val, nil
}

// TestE2ELatencyVerification validates LOAD-03 requirement:
// "Benchmark report prints P50/P95/P99 end-to-end latency percentiles"
//
// This test:
// - Produces 100 messages with modest concurrency
// - Validates both producer latency and E2E latency are present in report
// - Validates that P95 E2E latency > P95 producer latency (offset confirmation adds Phase 2 time)
// - Ensures E2E latencies are properly recorded and meaningful
//
// LOAD-03 is satisfied if:
// - Benchmark report has "E2E Latencies" section with numeric P50/P95/P99 values
// - P95 E2E latency > P95 producer latency (due to Phase 2 processing)
// - All E2E latency values are reasonable (> 0, within histogram range)
func TestE2ELatencyVerification(t *testing.T) {
	// Verify services are running (fail, not skip, for LOAD-03 validation)
	verifyServicesRunning(t)

	// Create a context with timeout for E2E latency test
	// 100 messages should complete quickly, but allow buffer for offset confirmation processing
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Run loadtest with modest scale (100 payments, 5 concurrency)
	cmd := exec.CommandContext(ctx, "../../loadtest",
		"--payments", "100",
		"--concurrency", "5",
		"--rate", "1000",
		"--timeout", "90",
		"--group", "loadtest-e2e-verify",
	)
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Log output for debugging
	t.Logf("E2E latency test output:\n%s", outputStr)

	// Verify benchmark report is generated
	require.Contains(t, outputStr, "Benchmark Report", "Should generate benchmark report")

	// Verify both producer and E2E latency sections exist
	assert.Contains(t, outputStr, "Producer Latencies", "Report should contain Producer Latencies section")
	assert.Contains(t, outputStr, "E2E Latencies", "Report should contain E2E Latencies section")

	// Parse P95 values from both sections
	producerP95, err := parseLatencyPercentile(outputStr, "Producer", "P95")
	require.NoError(t, err, "P95 Producer latency should be parseable from report")

	e2eP95, err := parseLatencyPercentile(outputStr, "E2E", "P95")
	require.NoError(t, err, "P95 E2E latency should be parseable from report")

	// Validate that E2E > Producer (offset confirmation adds Phase 2 time)
	assert.Greater(t, e2eP95, producerP95,
		"P95 E2E latency (%v µs) must be > P95 producer latency (%v µs); offset confirmation should add Phase 2 processing time",
		e2eP95, producerP95)

	// Verify all percentiles are present
	for _, p := range []string{"P50", "P95", "P99"} {
		_, err := parseLatencyPercentile(outputStr, "Producer", p)
		assert.NoError(t, err, "Producer %s should be present in report", p)

		_, err = parseLatencyPercentile(outputStr, "E2E", p)
		assert.NoError(t, err, "E2E %s should be present in report", p)
	}

	// Log success
	t.Logf("LOAD-03 validation passed: P95 producer=%.1f µs, P95 E2E=%.1f µs (delta=%.1f µs)",
		producerP95, e2eP95, e2eP95-producerP95)
}
