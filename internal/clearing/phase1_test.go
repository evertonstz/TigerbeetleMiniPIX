package clearing

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExecutePhase1CreatesPendingTransfers(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	executor := NewPhase1Executor(mockTB)

	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)
	result, err := executor.Execute(context.Background(), chain)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))

	// Verify all 3 transfers were submitted
	for i, transfer := range chain.Transfers {
		assert.Equal(t, transfer.ID, mockTB.CreatedTransfers[i].ID)
	}
}

func TestExecutePhase1HandlesTransferFailure(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	mockTB.SetFailure(true, 0) // Fail the first transfer
	executor := NewPhase1Executor(mockTB)

	chain := NewTransferChain("test-uuid", 1001, 1002, 10000)
	result, err := executor.Execute(context.Background(), chain)

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.ErrorMsg)
	assert.Contains(t, result.ErrorMsg, "rejected")
}
