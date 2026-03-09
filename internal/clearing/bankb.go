package clearing

import (
	"math/rand"
)

type BankBSimulator struct {
	AcceptRate float64 // Probability of accepting (0.0 - 1.0)
	DelayMS    int64   // Delay in milliseconds
	rng        *rand.Rand
}

func NewBankBSimulator(acceptRate float64, delayMS int64) *BankBSimulator {
	return &BankBSimulator{
		AcceptRate: acceptRate,
		DelayMS:    delayMS,
		rng:        rand.New(rand.NewSource(rand.Int63())),
	}
}

// Simulate returns true if Bank B accepts, false if rejects
func (b *BankBSimulator) Simulate(paymentUUID string) bool {
	return b.rng.Float64() < b.AcceptRate
}
