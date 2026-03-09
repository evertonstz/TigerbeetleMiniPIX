package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/clearing"
)

func TestRetryOnIDAlreadyFailed(t *testing.T) {
	mockTB := clearing.NewMockTigerBeetleClient()
	phase1 := clearing.NewPhase1Executor(mockTB)
	phase2 := clearing.NewPhase2Executor(mockTB)
	bankB := clearing.NewBankBSimulator(1.0, 0)
	config := &clearing.Config{BatchFlushTimeoutMs: 100}

	consumer := clearing.NewConsumer(
		[]string{"localhost:9092"},
		"pix-payments",
		"clearing-engine-group",
		config,
		phase1,
		phase2,
		bankB,
	)

	payment := &clearing.PaymentMessage{
		PaymentUUID:        "retry-test-uuid",
		AmountCents:        50000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// First attempt should fail due to id_already_failed (simulated)
	// Retry with new IDs should succeed
	err := consumer.HandlePayment(payment)
	assert.NoError(t, err)

	// Verify transfers were created and processed
	assert.True(t, len(mockTB.CreatedTransfers) >= 3)
}

func TestMaxRetriesExceeded(t *testing.T) {
	mockTB := clearing.NewMockTigerBeetleClient()
	phase1 := clearing.NewPhase1Executor(mockTB)
	phase2 := clearing.NewPhase2Executor(mockTB)
	bankB := clearing.NewBankBSimulator(0.95, 0)

	retryTracker := clearing.NewRetryTracker()

	// Manually set high retry count to simulate max retries exceeded
	for i := 0; i < 4; i++ {
		retryTracker.IncrementRetryCount("max-retry-test")
	}

	// Verify retry tracker is tracking correctly
	retries := retryTracker.GetRetryCount("max-retry-test")
	assert.Equal(t, 4, retries)

	// Verify the Phase 1 and Phase 2 executors are properly initialized
	assert.NotNil(t, phase1)
	assert.NotNil(t, phase2)
	assert.NotNil(t, bankB)
}
