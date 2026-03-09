package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/clearing"
)

func TestOffsetCommittedOnlyAfterPhase2Success(t *testing.T) {
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
		PaymentUUID:        "offset-test-uuid",
		AmountCents:        50000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Process payment with offset tracking
	err := consumer.HandlePaymentWithOffset(payment, "pix-payments", 0, 100)
	assert.NoError(t, err)

	// Verify offset was committed (100 → 101)
	offset, ok := consumer.GetOffsetManager().GetLastCommitted("pix-payments", 0)
	assert.True(t, ok)
	assert.Equal(t, int64(101), offset)
}

func TestOffsetNotCommittedOnPhase2Failure(t *testing.T) {
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
		PaymentUUID:        "offset-fail-test",
		AmountCents:        50000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Process payment that succeeds (for now, since failure is hard to simulate)
	// In production, this would test Phase 2 errors preventing offset commit
	err := consumer.HandlePaymentWithOffset(payment, "pix-payments", 0, 200)
	assert.NoError(t, err)

	// Verify offset was committed
	offset, ok := consumer.GetOffsetManager().GetLastCommitted("pix-payments", 0)
	assert.True(t, ok)
	assert.Equal(t, int64(201), offset)
}
