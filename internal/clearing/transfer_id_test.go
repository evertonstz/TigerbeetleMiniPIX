package clearing

import (
	"github.com/stretchr/testify/assert"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
	"testing"
)

func TestGenerateDeterministicTransferID(t *testing.T) {
	paymentUUID := "123e4567-e89b-12d3-a456-426614174000"

	// Phase 1 transfers
	id1 := GenerateTransferID(paymentUUID, 1, "phase1")
	id2 := GenerateTransferID(paymentUUID, 2, "phase1")
	id3 := GenerateTransferID(paymentUUID, 3, "phase1")

	// Same input → same ID (deterministic)
	id1_again := GenerateTransferID(paymentUUID, 1, "phase1")
	assert.Equal(t, id1, id1_again)

	// Different leg → different ID
	assert.NotEqual(t, id1, id2)
	assert.NotEqual(t, id2, id3)

	// Phase 2 uses different suffix
	id1_phase2 := GenerateTransferID(paymentUUID, 1, "phase2")
	assert.NotEqual(t, id1, id1_phase2)

	// IDs are valid uint128 (TigerBeetle transfer ID range)
	assert.True(t, id1 > 0)
	assert.True(t, id2 > 0)
	assert.True(t, id3 > 0)
}

func TestTransferChainConstruction(t *testing.T) {
	paymentUUID := "123e4567-e89b-12d3-a456-426614174000"
	amountCents := int64(10000)

	chain := NewTransferChain(paymentUUID, 1001, 1002, amountCents)

	assert.Len(t, chain.Transfers, 3)

	// Leg 1: Sender debit
	assert.Equal(t, types.ToUint128(1001), chain.Transfers[0].DebitAccountID)
	assert.Equal(t, types.ToUint128(3), chain.Transfers[0].CreditAccountID) // Internal bank reserve
	assert.Equal(t, types.ToUint128(uint64(amountCents)), chain.Transfers[0].Amount)
	assert.True(t, (chain.Transfers[0].Flags&0x1) != 0) // Linked flag (bit 0)

	// Leg 2: Reserve to recipient
	assert.Equal(t, types.ToUint128(3), chain.Transfers[1].DebitAccountID) // Internal reserve
	assert.Equal(t, types.ToUint128(1002), chain.Transfers[1].CreditAccountID)
	assert.Equal(t, types.ToUint128(uint64(amountCents)), chain.Transfers[1].Amount)
	assert.True(t, (chain.Transfers[1].Flags&0x1) != 0) // Linked flag (bit 0)

	// Leg 3: Terminal transfer (NO linked flag)
	assert.Equal(t, types.ToUint128(3), chain.Transfers[2].DebitAccountID)  // Internal reserve
	assert.Equal(t, types.ToUint128(2), chain.Transfers[2].CreditAccountID) // Main bank reserve
	assert.Equal(t, types.Uint128{}, chain.Transfers[2].Amount)             // Zero-value for idempotency bridge
	assert.False(t, (chain.Transfers[2].Flags&0x1) != 0)                    // NO linked flag

	// Verify linked references
	assert.Equal(t, chain.Transfers[0].ID, chain.Transfers[1].PendingID)
	assert.Equal(t, chain.Transfers[1].ID, chain.Transfers[2].PendingID)
}

func TestTransferChainWithCustomTimeout(t *testing.T) {
	paymentUUID := "123e4567-e89b-12d3-a456-426614174000"

	chain := NewTransferChainWithTimeout(
		paymentUUID, 1001, 1002, 10000, 60, // 60 second timeout
	)

	// All transfers should have 60 second timeout
	assert.Equal(t, uint32(60), chain.Transfers[0].Timeout)
	assert.Equal(t, uint32(60), chain.Transfers[1].Timeout)
	assert.Equal(t, uint32(60), chain.Transfers[2].Timeout)
}
