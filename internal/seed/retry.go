package seed

import (
	"context"
	"fmt"
	"log"
	"time"
)

// RetryTransient attempts an operation up to 3 times on transient errors.
// On transient error (timeout, unavailable), backs off: 1s, 2s, 4s.
// Returns the result on success, or error if all retries exhausted.
//
// Example usage:
//
//	result, err := RetryTransient(ctx, func() (interface{}, error) {
//	    return client.CreateAccounts(accounts)
//	})
func RetryTransient(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
	maxRetries := 3
	backoffs := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := operation()

		if err == nil {
			return result, nil
		}

		if attempt < maxRetries-1 {
			log.Printf("Transient error (attempt %d/%d): %v. Retrying in %v...",
				attempt+1, maxRetries, err, backoffs[attempt])
			time.Sleep(backoffs[attempt])
			lastErr = err
			continue
		}

		lastErr = err
		break
	}

	return nil, fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}
