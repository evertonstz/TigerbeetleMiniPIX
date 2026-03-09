package main

import (
	"fmt"
	"math/rand"
	"time"
)

// PaymentMessage represents a payment
type PaymentMessage struct {
	PaymentUUID        string
	SenderAccountID    uint64
	RecipientAccountID uint64
	AmountCents        int64
}

// PaymentGenerator generates random payments
type PaymentGenerator struct {
	rng      *rand.Rand
	accounts []uint64
	balances map[uint64]int64
}

// NewPaymentGenerator creates a generator
func NewPaymentGenerator(cfg *Config) (*PaymentGenerator, error) {
	source := rand.NewSource(cfg.Seed)
	rng := rand.New(source)

	accounts := make([]uint64, 5000)
	for i := 0; i < 5000; i++ {
		accounts[i] = uint64(100001 + i)
	}

	balances := make(map[uint64]int64)
	for _, id := range accounts {
		balances[id] = 1000000
	}

	return &PaymentGenerator{
		rng:      rng,
		accounts: accounts,
		balances: balances,
	}, nil
}

// Generate creates a random payment
func (pg *PaymentGenerator) Generate() (*PaymentMessage, error) {
	senderIdx := pg.rng.Intn(len(pg.accounts))
	senderID := pg.accounts[senderIdx]

	var receiverID uint64
	for {
		receiverIdx := pg.rng.Intn(len(pg.accounts))
		receiverID = pg.accounts[receiverIdx]
		if receiverID != senderID {
			break
		}
	}

	senderBalance := pg.balances[senderID]
	maxAmount := senderBalance
	if maxAmount > 100000 {
		maxAmount = 100000
	}
	amount := int64(pg.rng.Intn(int(maxAmount))) + 1

	timestamp := time.Now().UnixNano()
	random := pg.rng.Intn(1000000)
	paymentUUID := fmt.Sprintf("%d-%d", timestamp, random)

	return &PaymentMessage{
		PaymentUUID:        paymentUUID,
		SenderAccountID:    senderID,
		RecipientAccountID: receiverID,
		AmountCents:        amount,
	}, nil
}
