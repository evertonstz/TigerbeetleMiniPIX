package clearing

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBatchAccumulator(t *testing.T) {
	acc := NewBatchAccumulator()
	assert.NotNil(t, acc)
	assert.Equal(t, 0, acc.Size())
	assert.False(t, acc.IsFull())
}

func TestAdd_SinglePayment(t *testing.T) {
	acc := NewBatchAccumulator()
	payment := &PaymentMessage{
		PaymentUUID:        "test-1",
		SenderAccountID:    1000,
		RecipientAccountID: 2000,
		AmountCents:        10000,
	}

	isFull, err := acc.Add(payment)
	assert.NoError(t, err)
	assert.False(t, isFull)
	assert.Equal(t, 3, acc.Size()) // 3 transfers
	assert.Equal(t, 1, len(acc.Payments()))
}

func TestAdd_MultiplePayments(t *testing.T) {
	acc := NewBatchAccumulator()

	// Add 3 payments (9 transfers total)
	for i := 0; i < 3; i++ {
		payment := &PaymentMessage{
			PaymentUUID:        fmt.Sprintf("test-%d", i),
			SenderAccountID:    uint64(1000 + i),
			RecipientAccountID: uint64(2000 + i),
			AmountCents:        10000,
		}
		isFull, err := acc.Add(payment)
		assert.NoError(t, err)
		assert.False(t, isFull)
	}

	assert.Equal(t, 9, acc.Size())
	assert.Equal(t, 3, len(acc.Payments()))
}

func TestGetPaymentForTransferIndex_ErrorAttribution(t *testing.T) {
	acc := NewBatchAccumulator()

	// Add 3 payments with 3 transfers each = 9 total transfers
	p1 := &PaymentMessage{PaymentUUID: "p1", SenderAccountID: 1000, RecipientAccountID: 2000, AmountCents: 10000}
	p2 := &PaymentMessage{PaymentUUID: "p2", SenderAccountID: 1001, RecipientAccountID: 2001, AmountCents: 20000}
	p3 := &PaymentMessage{PaymentUUID: "p3", SenderAccountID: 1002, RecipientAccountID: 2002, AmountCents: 30000}

	acc.Add(p1)
	acc.Add(p2)
	acc.Add(p3)

	// Verify indices:
	// p1: [0, 1, 2]
	// p2: [3, 4, 5]
	// p3: [6, 7, 8]

	for idx := uint32(0); idx <= 2; idx++ {
		p, err := acc.GetPaymentForTransferIndex(idx)
		assert.NoError(t, err)
		assert.Equal(t, "p1", p.PaymentUUID)
	}

	for idx := uint32(3); idx <= 5; idx++ {
		p, err := acc.GetPaymentForTransferIndex(idx)
		assert.NoError(t, err)
		assert.Equal(t, "p2", p.PaymentUUID)
	}

	for idx := uint32(6); idx <= 8; idx++ {
		p, err := acc.GetPaymentForTransferIndex(idx)
		assert.NoError(t, err)
		assert.Equal(t, "p3", p.PaymentUUID)
	}
}

func TestClear(t *testing.T) {
	acc := NewBatchAccumulator()

	// Add some data
	for i := 0; i < 5; i++ {
		payment := &PaymentMessage{
			PaymentUUID:        fmt.Sprintf("test-%d", i),
			SenderAccountID:    uint64(1000 + i),
			RecipientAccountID: uint64(2000 + i),
			AmountCents:        10000,
		}
		acc.Add(payment)
	}

	assert.Equal(t, 15, acc.Size())

	// Clear
	acc.Clear()

	assert.Equal(t, 0, acc.Size())
	assert.Equal(t, 0, len(acc.Payments()))
	assert.False(t, acc.IsFull())
}

func TestBatchCapacityBoundary(t *testing.T) {
	acc := NewBatchAccumulator()

	// Add payments until we reach near capacity
	// Max transfers = 8189, each payment = 3 transfers
	// 2729 payments = 8187 transfers (not full yet)
	// 2730 payments = 8190 transfers (would exceed)

	maxPayments := 8189 / 3 // 2729
	var lastIsFull bool
	for i := 0; i < maxPayments; i++ {
		payment := &PaymentMessage{
			PaymentUUID:        fmt.Sprintf("p-%d", i%10000),
			SenderAccountID:    uint64(1000 + i%1000),
			RecipientAccountID: uint64(2000 + i%1000),
			AmountCents:        10000,
		}
		isFull, err := acc.Add(payment)
		assert.NoError(t, err, "error at payment %d", i)
		lastIsFull = isFull
	}

	// After 2729 payments, we have 8187 transfers (not yet at 8189)
	assert.False(t, lastIsFull, "should not be full after 2729 payments (8187 transfers < 8189)")

	// Try to add one more — should fail because 8187 + 3 > 8189
	extraPayment := &PaymentMessage{
		PaymentUUID:        "extra",
		SenderAccountID:    5000,
		RecipientAccountID: 6000,
		AmountCents:        10000,
	}
	isFull, err := acc.Add(extraPayment)
	assert.Error(t, err)
	assert.False(t, isFull)
	assert.Contains(t, err.Error(), "at capacity")
}

func TestBatchErrorAttribution_NoOffByOne(t *testing.T) {
	// Setup: 10 payments, 3 transfers each = 30 total transfers
	acc := NewBatchAccumulator()
	payments := make([]*PaymentMessage, 10)

	for i := 0; i < 10; i++ {
		payments[i] = &PaymentMessage{
			PaymentUUID:        fmt.Sprintf("p%d", i),
			SenderAccountID:    uint64(1000 + i),
			RecipientAccountID: uint64(2000 + i),
			AmountCents:        100000,
		}
		acc.Add(payments[i])
	}

	assert.Equal(t, 30, acc.Size())

	// Simulate TigerBeetle returning 3 errors at specific indices
	// Payment 0: indices [0, 1, 2]
	// Payment 1: indices [3, 4, 5]     <- error at 5
	// Payment 2: indices [6, 7, 8]
	// ...
	// Payment 5: indices [15, 16, 17]  <- error at 15
	// ...
	// Payment 9: indices [27, 28, 29]  <- error at 27

	errorIndices := []uint32{5, 15, 27}
	expectedPaymentUUIDs := []string{"p1", "p5", "p9"}

	for i, errIdx := range errorIndices {
		payment, err := acc.GetPaymentForTransferIndex(errIdx)
		assert.NoError(t, err, "error mapping index %d", errIdx)
		assert.NotNil(t, payment)
		assert.Equal(t, expectedPaymentUUIDs[i], payment.PaymentUUID,
			"error at index %d should map to %s, got %s", errIdx, expectedPaymentUUIDs[i], payment.PaymentUUID)
	}

	// Verify non-error indices map correctly too
	// Index 4 should be payment 1 (indices 3-5)
	p4, _ := acc.GetPaymentForTransferIndex(4)
	assert.Equal(t, "p1", p4.PaymentUUID)

	// Index 16 should be payment 5 (indices 15-17)
	p16, _ := acc.GetPaymentForTransferIndex(16)
	assert.Equal(t, "p5", p16.PaymentUUID)

	// Index 28 should be payment 9 (indices 27-29)
	p28, _ := acc.GetPaymentForTransferIndex(28)
	assert.Equal(t, "p9", p28.PaymentUUID)
}

func TestBatchErrorAttribution_BoundaryIndices(t *testing.T) {
	// Setup: 5 payments, 3 transfers each = 15 total transfers
	// Payment 0: indices [0, 1, 2]
	// Payment 1: indices [3, 4, 5]
	// Payment 2: indices [6, 7, 8]
	// Payment 3: indices [9, 10, 11]
	// Payment 4: indices [12, 13, 14]

	acc := NewBatchAccumulator()
	for i := 0; i < 5; i++ {
		payment := &PaymentMessage{
			PaymentUUID:        fmt.Sprintf("boundary-p%d", i),
			SenderAccountID:    uint64(1000 + i),
			RecipientAccountID: uint64(2000 + i),
			AmountCents:        100000,
		}
		acc.Add(payment)
	}

	// Test boundary cases
	testCases := []struct {
		index             uint32
		expectedPaymentID int
	}{
		{0, 0},  // First index → first payment
		{1, 0},  // Middle of first payment
		{2, 0},  // Last index of first payment
		{3, 1},  // First index of second payment
		{14, 4}, // Last index of last payment
		{13, 4}, // Middle of last payment
		{7, 2},  // Middle of middle payment
	}

	for _, tc := range testCases {
		payment, err := acc.GetPaymentForTransferIndex(tc.index)
		assert.NoError(t, err, "error mapping index %d", tc.index)
		expectedUUID := fmt.Sprintf("boundary-p%d", tc.expectedPaymentID)
		assert.Equal(t, expectedUUID, payment.PaymentUUID,
			"index %d should map to payment %d (%s), got %s",
			tc.index, tc.expectedPaymentID, expectedUUID, payment.PaymentUUID)
	}
}
