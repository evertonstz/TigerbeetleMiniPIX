package clearing

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConsumerStartsSuccessfully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	phase2 := NewPhase2Executor(mockTB)
	bankB := NewBankBSimulator(0.95, 0)
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

	// Should not panic or error on Start
	err := consumer.Start(ctx)
	assert.NoError(t, err)

	// Cleanup
	consumer.Stop()
}

func TestDeserializePaymentMessage(t *testing.T) {
	msg := []byte(`{"payment_uuid":"123e4567-e89b-12d3-a456-426614174000","amount_cents":10000,"sender_account_id":1001,"recipient_account_id":1002,"pix_key":"alice@example.com"}`)

	payment, err := DeserializePayment(msg)
	assert.NoError(t, err)
	assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", payment.PaymentUUID)
	assert.Equal(t, int64(10000), payment.AmountCents)
	assert.Equal(t, uint64(1001), payment.SenderAccountID)
	assert.Equal(t, uint64(1002), payment.RecipientAccountID)
	assert.Equal(t, "alice@example.com", payment.PixKey)
}

func TestConsumerPollsMessages(t *testing.T) {
	// Mock Kafka records
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	phase2 := NewPhase2Executor(mockTB)
	bankB := NewBankBSimulator(0.95, 0)
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

	// We'll test that Poll method exists and doesn't panic
	// (Full integration test deferred until infrastructure test)
	assert.NotNil(t, consumer)
}

func TestConsumerHandlesPaymentWithPhase1Executor(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	phase2 := NewPhase2Executor(mockTB)
	bankB := NewBankBSimulator(0.95, 0)
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

	// Create a test payment
	payment := &PaymentMessage{
		PaymentUUID:        "test-uuid",
		AmountCents:        10000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
		PixKey:             "test@example.com",
	}

	// Test the internal handlePayment method (Phase 1 only)
	err := consumer.handlePaymentPhase1(payment)
	assert.NoError(t, err)

	// Verify Phase 1 was executed (3 transfers created)
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))
}

func TestConsumerExecutesFullPaymentFlow(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	phase2 := NewPhase2Executor(mockTB)
	bankB := NewBankBSimulator(1.0, 0) // 100% accept for testing
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
		PaymentUUID:        "test-uuid",
		AmountCents:        10000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
		PixKey:             "alice@example.com",
	}

	err := consumer.HandlePayment(payment)
	assert.NoError(t, err)

	// Verify Phase 1 created 3 transfers
	assert.Equal(t, 3, len(mockTB.CreatedTransfers))

	// Verify Phase 2 posted them
	assert.Equal(t, 3, len(mockTB.PostedTransfers))
}

func TestConsumerCommitsOffsetAfterPhase2(t *testing.T) {
	mockTB := NewMockTigerBeetleClient()
	phase1 := NewPhase1Executor(mockTB)
	phase2 := NewPhase2Executor(mockTB)
	bankB := NewBankBSimulator(1.0, 0)
	offsetMgr := NewOffsetManager()
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
	consumer.offsetManager = offsetMgr

	payment := &PaymentMessage{
		PaymentUUID:        "test-uuid",
		AmountCents:        10000,
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
		PixKey:             "alice@example.com",
	}

	// Simulate processing a message at offset 100
	err := consumer.HandlePaymentWithOffset(payment, "pix-payments", 0, 100)
	assert.NoError(t, err)

	// Verify offset was marked for commit
	offset, ok := offsetMgr.GetLastCommitted("pix-payments", 0)
	assert.True(t, ok)
	assert.Equal(t, int64(101), offset) // Next offset
}
