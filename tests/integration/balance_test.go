package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/clearing"
)

func TestBalanceCorrectnessSinglePaymentAccept(t *testing.T) {
	// Create a mock TigerBeetle client with balance tracking
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
		PaymentUUID:        "balance-test-accept",
		AmountCents:        10000, // 100.00 BRL
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Execute payment (accepted by Bank B)
	err := consumer.HandlePayment(payment)
	assert.NoError(t, err)

	// Verify transfers were created and posted
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))
	assert.Equal(t, 3, len(mockTB.PostedTransfers))
}

func TestBalanceCorrectnessSinglePaymentReject(t *testing.T) {
	mockTB := clearing.NewMockTigerBeetleClient()
	phase1 := clearing.NewPhase1Executor(mockTB)
	phase2 := clearing.NewPhase2Executor(mockTB)
	bankB := clearing.NewBankBSimulator(0.0, 0) // Always reject
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
		PaymentUUID:        "balance-test-reject",
		AmountCents:        10000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Execute payment (rejected by Bank B)
	err := consumer.HandlePayment(payment)
	assert.NoError(t, err)

	// Verify transfers were created but voided (not posted)
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))
	assert.Equal(t, 3, len(mockTB.VoidedTransfers))
	assert.Equal(t, 0, len(mockTB.PostedTransfers))
}
