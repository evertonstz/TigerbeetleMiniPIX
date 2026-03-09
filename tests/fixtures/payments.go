package fixtures

import (
	"github.com/evertoncorreia/tigerbeetle-minipix/internal/clearing"
)

func SamplePayment() *clearing.PaymentMessage {
	return &clearing.PaymentMessage{
		PaymentUUID:        "123e4567-e89b-12d3-a456-426614174000",
		AmountCents:        100000, // 1000.00 BRL
		SenderAccountID:    1001,
		RecipientAccountID: 1002,
		PixKey:             "alice@example.com",
	}
}

func LargePayment() *clearing.PaymentMessage {
	return &clearing.PaymentMessage{
		PaymentUUID:        "223e4567-e89b-12d3-a456-426614174000",
		AmountCents:        50000000, // 500,000 BRL
		SenderAccountID:    1001,
		RecipientAccountID: 1003,
		PixKey:             "bob@example.com",
	}
}

func SmallPayment() *clearing.PaymentMessage {
	return &clearing.PaymentMessage{
		PaymentUUID:        "323e4567-e89b-12d3-a456-426614174000",
		AmountCents:        100, // 1.00 BRL
		SenderAccountID:    1001,
		RecipientAccountID: 1004,
		PixKey:             "charlie@example.com",
	}
}
