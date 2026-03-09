package clearing

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIdempotencyKeyGeneration(t *testing.T) {
	key1 := GenerateIdempotencyKey("payment-123", 1, 0)
	key2 := GenerateIdempotencyKey("payment-123", 1, 0)

	// Same inputs → same key
	assert.Equal(t, key1, key2)

	// Different retry count → different key
	key3 := GenerateIdempotencyKey("payment-123", 1, 1)
	assert.NotEqual(t, key1, key3)
}

func TestRetryCountTracking(t *testing.T) {
	tracker := NewRetryTracker()

	// Initial retry count is 0
	count := tracker.GetRetryCount("payment-123")
	assert.Equal(t, 0, count)

	// Increment retry count
	tracker.IncrementRetryCount("payment-123")
	count = tracker.GetRetryCount("payment-123")
	assert.Equal(t, 1, count)

	// Increment again
	tracker.IncrementRetryCount("payment-123")
	count = tracker.GetRetryCount("payment-123")
	assert.Equal(t, 2, count)

	// Reset retry count
	tracker.ResetRetryCount("payment-123")
	count = tracker.GetRetryCount("payment-123")
	assert.Equal(t, 0, count)
}
