package clearing

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBankBSimulatorAcceptsProbabilistically(t *testing.T) {
	sim := NewBankBSimulator(0.95, 0)

	// Run 100 simulations
	accepts := 0
	for i := 0; i < 100; i++ {
		if sim.Simulate("test-uuid") {
			accepts++
		}
	}

	// Should be approximately 95 accepts (allowing 15% variance)
	assert.Greater(t, accepts, 80)
	assert.Less(t, accepts, 100)
}

func TestBankBSimulatorCanConfigureRejectRate(t *testing.T) {
	sim := NewBankBSimulator(0.5, 0) // 50% accept rate

	accepts := 0
	for i := 0; i < 100; i++ {
		if sim.Simulate("test-uuid") {
			accepts++
		}
	}

	// Should be approximately 50 accepts
	assert.Greater(t, accepts, 35)
	assert.Less(t, accepts, 65)
}

func TestBankBSimulatorWithDelay(t *testing.T) {
	sim := NewBankBSimulator(0.95, 100) // 100ms delay

	// Verify delay parameter is stored
	assert.Equal(t, int64(100), sim.DelayMS)
}
