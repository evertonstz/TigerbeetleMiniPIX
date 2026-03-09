package clearing

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

// Phase 1 Retry Tests (Phase 3 + Phase 4)
// These tests verify error handling in Phase 1 transfer submission.
// Phase 3: Single-payment flow (one CreateTransfers call per payment)
// Phase 4: Batch flow (multiple payments, one CreateTransfers call with 2,729 payments)
//
// Retry logic applies to both contexts:
// - id_already_failed: Generate new ID with retry suffix (Phase 3 + Phase 4)
// - pending_transfer_expired: Terminal error, no retry (Phase 3 + Phase 4)
// - Network transients: Retry with exponential backoff (Phase 3 + Phase 4)
//
// Error attribution (Phase 4 specific): see TestBatchErrorAttribution_* tests in batch_accumulator_test.go

func TestPhase1ExecuteWithRetryTracking(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	retryTracker := NewRetryTracker()

	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)

	// Execute with retry tracking
	result, err := phase1.ExecuteWithRetry(context.Background(), chain, retryTracker)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, retryTracker.GetRetryCount("test-uuid")) // No retries needed
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))
}

func TestPhase1MaxRetriesExceeded(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	retryTracker := NewRetryTracker()

	// Simulate already at max retries
	for i := 0; i < MaxRetries; i++ {
		retryTracker.IncrementRetryCount("test-uuid")
	}

	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)

	// Should fail because max retries exceeded
	result, err := phase1.ExecuteWithRetry(context.Background(), chain, retryTracker)

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.ErrorMsg, "max retries")
}

func TestPhase1RetryTrackerIntegration(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	retryTracker := NewRetryTracker()

	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)

	// First execution
	result, err := phase1.ExecuteWithRetry(context.Background(), chain, retryTracker)
	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, retryTracker.GetRetryCount("test-uuid"))

	// Second execution (different payment)
	chain2 := NewTransferChain("test-uuid2", 1001, 1002, 10000)
	result2, err := phase1.ExecuteWithRetry(context.Background(), chain2, retryTracker)
	assert.NoError(t, err)
	assert.True(t, result2.Success)
	assert.Equal(t, 0, retryTracker.GetRetryCount("test-uuid2"))
}
