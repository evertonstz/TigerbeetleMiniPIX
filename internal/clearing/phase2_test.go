package clearing

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPhase2ExecutorPostsPendingTransfers(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	executor := NewPhase2Executor(mockTB)

	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)

	// Simulate Phase 1 already executed (transfers created)
	mockTB.CreatedTransfers = append(mockTB.CreatedTransfers,
		chain.Transfers[0],
		chain.Transfers[1],
		chain.Transfers[2],
	)

	// Execute Phase 2 with Bank B accepting
	result, err := executor.Execute(context.Background(), chain, true)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 3, len(mockTB.PostedTransfers))
}

func TestPhase2ExecutorVoidsPendingTransfersOnBankBReject(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	executor := NewPhase2Executor(mockTB)

	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)

	// Simulate Phase 1 already executed
	mockTB.CreatedTransfers = append(mockTB.CreatedTransfers,
		chain.Transfers[0],
		chain.Transfers[1],
		chain.Transfers[2],
	)

	// Execute Phase 2 with Bank B rejecting
	result, err := executor.Execute(context.Background(), chain, false)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 3, len(mockTB.VoidedTransfers))
	assert.Equal(t, 0, len(mockTB.PostedTransfers))
}

func TestPhase2ExecutorHandlesPostError(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	// Configure to fail on post
	mockTB.setPostFailure(0)

	executor := NewPhase2Executor(mockTB)
	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)

	result, err := executor.Execute(context.Background(), chain, true)

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.ErrorMsg)
	assert.False(t, result.ShouldRetry) // Terminal error - no retry
}

func TestPhase2ResultsInTerminalError(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	executor := NewPhase2Executor(mockTB)
	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)

	// Fail the first transfer
	mockTB.setPostFailure(0)
	result, err := executor.Execute(context.Background(), chain, true)

	// Terminal failure - should not retry
	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.False(t, result.ShouldRetry)
	assert.Contains(t, result.ErrorMsg, "failed")
}
