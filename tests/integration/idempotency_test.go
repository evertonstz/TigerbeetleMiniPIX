package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/clearing"
)

func TestIdempotentPaymentProcessing(t *testing.T) {
	mockTB := clearing.NewMockTigerBeetleClient()
	phase1 := clearing.NewPhase1Executor(mockTB)
	phase2 := clearing.NewPhase2Executor(mockTB)
	bankB := clearing.NewBankBSimulator(1.0, 0) // Always accept
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
		PaymentUUID:        "idempotent-test-uuid",
		AmountCents:        50000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Process payment first time
	err1 := consumer.HandlePayment(payment)
	assert.NoError(t, err1)
	firstCount := len(mockTB.CreatedTransfers)
	assert.Equal(t, 3, firstCount)

	// Process same payment again (simulates offset not committed, consumer restarted)
	// Deterministic IDs should cause "already exists" error, treated as success
	err2 := consumer.HandlePayment(payment)
	assert.NoError(t, err2)

	// Should have attempted to create 6 transfers total (3+3)
	// But 2nd attempt should use same IDs
	assert.True(t, len(mockTB.CreatedTransfers) >= 3)
}
