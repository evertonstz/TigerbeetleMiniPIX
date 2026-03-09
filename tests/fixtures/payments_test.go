package fixtures

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSamplePaymentHasValidStructure(t *testing.T) {
	payment := SamplePayment()

	assert.NotEmpty(t, payment.PaymentUUID)
	assert.Greater(t, payment.AmountCents, int64(0))
	assert.Greater(t, payment.SenderAccountID, uint64(0))
	assert.Greater(t, payment.RecipientAccountID, uint64(0))
	assert.NotEmpty(t, payment.PixKey)
}

func TestLargePaymentHasHigherAmount(t *testing.T) {
	sample := SamplePayment()
	large := LargePayment()

	assert.Greater(t, large.AmountCents, sample.AmountCents)
}

func TestSmallPaymentHasLowerAmount(t *testing.T) {
	sample := SamplePayment()
	small := SmallPayment()

	assert.Less(t, small.AmountCents, sample.AmountCents)
}

func TestPaymentsHaveDifferentUUIDs(t *testing.T) {
	sample := SamplePayment()
	large := LargePayment()
	small := SmallPayment()

	assert.NotEqual(t, sample.PaymentUUID, large.PaymentUUID)
	assert.NotEqual(t, sample.PaymentUUID, small.PaymentUUID)
	assert.NotEqual(t, large.PaymentUUID, small.PaymentUUID)
}
