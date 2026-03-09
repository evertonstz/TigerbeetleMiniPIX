package clearing

import (
	"fmt"
	"sync"
)

func GenerateIdempotencyKey(paymentUUID string, legIndex int, retryCount int) string {
	return fmt.Sprintf("%s:leg%d:retry%d", paymentUUID, legIndex, retryCount)
}

type RetryTracker struct {
	counts map[string]int
	mu     sync.RWMutex
}

func NewRetryTracker() *RetryTracker {
	return &RetryTracker{
		counts: make(map[string]int),
	}
}

func (rt *RetryTracker) GetRetryCount(paymentUUID string) int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	count, exists := rt.counts[paymentUUID]
	if !exists {
		return 0
	}
	return count
}

func (rt *RetryTracker) IncrementRetryCount(paymentUUID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.counts[paymentUUID]++
}

func (rt *RetryTracker) ResetRetryCount(paymentUUID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.counts[paymentUUID] = 0
}
