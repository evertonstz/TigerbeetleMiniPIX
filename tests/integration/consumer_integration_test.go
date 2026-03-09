package integration

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/clearing"
	"github.com/evertoncorreia/tigerbeetle-minipix/tests/fixtures"
)

func TestConsumerProcessesMultiplePayments(t *testing.T) {
	mockTB := clearing.NewMockTigerBeetleClient()
	phase1 := clearing.NewPhase1Executor(mockTB)
	phase2 := clearing.NewPhase2Executor(mockTB)
	bankB := clearing.NewBankBSimulator(0.95, 0)
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

	// Process multiple payments
	payments := []*clearing.PaymentMessage{
		fixtures.SamplePayment(),
		fixtures.LargePayment(),
		fixtures.SmallPayment(),
	}

	for _, payment := range payments {
		err := consumer.HandlePayment(payment)
		assert.NoError(t, err, "Payment %s should process without error", payment.PaymentUUID)
	}

	// Verify all payments created 3 transfers each
	assert.Equal(t, 9, len(mockTB.CreatedTransfers), "Should have 9 transfers (3 per payment)")
}

func TestConsumerHandlesErrorsGracefully(t *testing.T) {
	mockTB := clearing.NewMockTigerBeetleClient()
	phase1 := clearing.NewPhase1Executor(mockTB)
	phase2 := clearing.NewPhase2Executor(mockTB)
	bankB := clearing.NewBankBSimulator(0.95, 0)
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

	// Payment with valid accounts should process successfully
	validPayment := &clearing.PaymentMessage{
		PaymentUUID:        "valid-uuid",
		AmountCents:        100000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Should not panic and should complete successfully
	err := consumer.HandlePayment(validPayment)
	assert.NoError(t, err)

	// Simulate phase 1 failure by using nil executor
	nilConsumer := &clearing.Consumer{}
	nilPayment := &clearing.PaymentMessage{
		PaymentUUID:        "nil-test",
		AmountCents:        100000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}
	err = nilConsumer.HandlePayment(nilPayment)
	assert.Error(t, err)
}
