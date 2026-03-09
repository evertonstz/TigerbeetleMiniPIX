package main

import (
	"testing"
)

// TestPaymentGeneratorNeverSelfPayment verifies that after 10,000 attempts,
// no self-payment is generated (sender != recipient).
func TestPaymentGeneratorNeverSelfPayment(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 12345 // Reproducible

	pg, err := NewPaymentGenerator(cfg)
	if err != nil {
		t.Fatalf("NewPaymentGenerator failed: %v", err)
	}

	// Run 10,000 generations to ensure no self-payments
	for i := 0; i < 10000; i++ {
		payment, err := pg.Generate()
		if err != nil {
			t.Fatalf("Generate() failed at attempt %d: %v", i, err)
		}

		if payment.SenderAccountID == payment.RecipientAccountID {
			t.Fatalf("Self-payment detected at attempt %d: sender=%d, recipient=%d",
				i, payment.SenderAccountID, payment.RecipientAccountID)
		}
	}
}

// TestPaymentAmountRespectBalance verifies that generated amounts are positive
// and respect the per-payment cap of 100,000 units.
func TestPaymentAmountRespectBalance(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 54321

	pg, err := NewPaymentGenerator(cfg)
	if err != nil {
		t.Fatalf("NewPaymentGenerator failed: %v", err)
	}

	for i := 0; i < 1000; i++ {
		payment, err := pg.Generate()
		if err != nil {
			t.Fatalf("Generate() failed at attempt %d: %v", i, err)
		}

		// Amount must be positive
		if payment.AmountCents <= 0 {
			t.Fatalf("Invalid amount at attempt %d: %d (must be > 0)", i, payment.AmountCents)
		}

		// Amount must be capped at 100,000 per payment
		if payment.AmountCents > 100000 {
			t.Fatalf("Amount exceeds cap at attempt %d: %d > 100000", i, payment.AmountCents)
		}
	}
}

// TestPaymentUserIDRange verifies that sender and recipient are always
// in the user pool range [100001, 105000].
func TestPaymentUserIDRange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 99999

	pg, err := NewPaymentGenerator(cfg)
	if err != nil {
		t.Fatalf("NewPaymentGenerator failed: %v", err)
	}

	minID := uint64(100001)
	maxID := uint64(105000)

	for i := 0; i < 1000; i++ {
		payment, err := pg.Generate()
		if err != nil {
			t.Fatalf("Generate() failed at attempt %d: %v", i, err)
		}

		if payment.SenderAccountID < minID || payment.SenderAccountID > maxID {
			t.Fatalf("Sender ID out of range at attempt %d: %d not in [%d, %d]",
				i, payment.SenderAccountID, minID, maxID)
		}

		if payment.RecipientAccountID < minID || payment.RecipientAccountID > maxID {
			t.Fatalf("Recipient ID out of range at attempt %d: %d not in [%d, %d]",
				i, payment.RecipientAccountID, minID, maxID)
		}
	}
}
