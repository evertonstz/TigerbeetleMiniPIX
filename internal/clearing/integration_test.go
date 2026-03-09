package clearing

import (
	"github.com/stretchr/testify/assert"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
	"testing"
)

func TestLinkedTransferChainE2E(t *testing.T) {
	// Simulate full flow: Phase 1 pending → Bank B accept → Phase 2 post
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	phase2 := NewPhase2Executor(mockTB)
	bankB := NewBankBSimulator(1.0, 0) // 100% accept
	cfg := DefaultConfig()

	consumer := NewConsumer(
		[]string{"localhost:9092"},
		"pix-payments",
		"clearing-engine-group",
		cfg,
		phase1,
		phase2,
		bankB,
	)

	payment := &PaymentMessage{
		PaymentUUID:        "e2e-test-uuid",
		AmountCents:        50000, // 500.00 BRL
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Execute full flow
	err := consumer.HandlePayment(payment)
	assert.NoError(t, err)

	// Verify Phase 1: 3 pending transfers created
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))

	// Verify linked chain structure
	transfers := mockTB.CreatedTransfers
	assert.Equal(t, types.ToUint128(uint64(1001)), transfers[0].DebitAccountID)
	assert.Equal(t, types.ToUint128(uint64(3)), transfers[0].CreditAccountID)
	assert.True(t, transfers[0].Flags&0x1 != 0) // linked flag

	assert.Equal(t, types.ToUint128(uint64(3)), transfers[1].DebitAccountID)
	assert.Equal(t, types.ToUint128(uint64(1002)), transfers[1].CreditAccountID)
	assert.True(t, transfers[1].Flags&0x1 != 0)

	assert.Equal(t, types.ToUint128(uint64(3)), transfers[2].DebitAccountID)
	assert.Equal(t, types.ToUint128(uint64(2)), transfers[2].CreditAccountID)
	assert.False(t, transfers[2].Flags&0x1 != 0) // NO linked flag

	// Verify Phase 2: all 3 posted
	assert.Equal(t, 3, len(mockTB.PostedTransfers))
}

func TestLinkedTransferChainRejectE2E(t *testing.T) {
	// Simulate full flow with Bank B rejection
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	phase2 := NewPhase2Executor(mockTB)
	bankB := NewBankBSimulator(0.0, 0) // 0% accept = always reject
	cfg := DefaultConfig()

	consumer := NewConsumer(
		[]string{"localhost:9092"},
		"pix-payments",
		"clearing-engine-group",
		cfg,
		phase1,
		phase2,
		bankB,
	)

	payment := &PaymentMessage{
		PaymentUUID:        "e2e-reject-uuid",
		AmountCents:        50000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
	}

	// Execute full flow
	err := consumer.HandlePayment(payment)
	assert.NoError(t, err)

	// Verify Phase 1: 3 pending transfers created
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))

	// Verify Phase 2: all 3 voided (not posted)
	assert.Equal(t, 3, len(mockTB.VoidedTransfers))
	assert.Equal(t, 0, len(mockTB.PostedTransfers))
}
