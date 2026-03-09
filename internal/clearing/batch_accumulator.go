package clearing

import (
	"fmt"
	"sync"
	"time"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/account"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

const TransfersPerPayment = 3

// PaymentTransferIndex maps a payment to its transfer indices within a batch
type PaymentTransferIndex struct {
	PaymentUUID string
	StartIndex  int // Position of first transfer in accumulated batch
	EndIndex    int // Position of last transfer in accumulated batch
}

type BatchAccumulator struct {
	mu             sync.Mutex
	payments       []*PaymentMessage
	transfers      []types.Transfer
	paymentIndices []PaymentTransferIndex
	maxTransfers   int
	lastFlushTime  time.Time
}

func NewBatchAccumulator() *BatchAccumulator {
	return &BatchAccumulator{
		payments:       make([]*PaymentMessage, 0),
		transfers:      make([]types.Transfer, 0, account.TigerBeetleBatchSize),
		paymentIndices: make([]PaymentTransferIndex, 0),
		maxTransfers:   account.TigerBeetleBatchSize,
		lastFlushTime:  time.Now(),
	}
}

// Add appends a payment and its 3-transfer chain to the batch.
// Returns (true, nil) if batch is now full (>= maxTransfers);
// Returns (false, error) if adding would exceed capacity;
// Returns (false, nil) if batch has room after adding.
func (b *BatchAccumulator) Add(payment *PaymentMessage) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	chain := NewTransferChain(
		payment.PaymentUUID,
		payment.SenderAccountID,
		payment.RecipientAccountID,
		payment.AmountCents,
	)

	newTransferCount := len(b.transfers) + TransfersPerPayment
	if newTransferCount > b.maxTransfers {
		return false, fmt.Errorf("batch at capacity: %d transfers + 3 would exceed %d", len(b.transfers), b.maxTransfers)
	}

	startIdx := len(b.transfers)
	b.payments = append(b.payments, payment)
	b.transfers = append(b.transfers, chain.Transfers[:]...)
	b.paymentIndices = append(b.paymentIndices, PaymentTransferIndex{
		PaymentUUID: payment.PaymentUUID,
		StartIndex:  startIdx,
		EndIndex:    startIdx + TransfersPerPayment - 1,
	})

	newCount := len(b.transfers)
	isFull := newCount >= b.maxTransfers
	return isFull, nil
}

// GetPaymentForTransferIndex maps a transfer index (from TigerBeetle error response)
// to the PaymentMessage that owns it. Used for error attribution.
func (b *BatchAccumulator) GetPaymentForTransferIndex(transferIdx uint32) (*PaymentMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := int(transferIdx)

	for _, paymentIdx := range b.paymentIndices {
		if idx >= paymentIdx.StartIndex && idx <= paymentIdx.EndIndex {
			for _, payment := range b.payments {
				if payment.PaymentUUID == paymentIdx.PaymentUUID {
					return payment, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("transfer index %d not found in batch (batch has %d transfers)", idx, len(b.transfers))
}

// IsFull returns true if batch has reached capacity
func (b *BatchAccumulator) IsFull() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.transfers) >= b.maxTransfers
}

// Clear resets the accumulator for the next batch
func (b *BatchAccumulator) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.payments = b.payments[:0]
	b.transfers = b.transfers[:0]
	b.paymentIndices = b.paymentIndices[:0]
	b.lastFlushTime = time.Now()
}

// Size returns the current number of accumulated transfers
func (b *BatchAccumulator) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.transfers)
}

// Transfers returns a copy of accumulated transfers
func (b *BatchAccumulator) Transfers() []types.Transfer {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]types.Transfer, len(b.transfers))
	copy(result, b.transfers)
	return result
}

// Payments returns a copy of accumulated payments
func (b *BatchAccumulator) Payments() []*PaymentMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]*PaymentMessage, len(b.payments))
	copy(result, b.payments)
	return result
}
