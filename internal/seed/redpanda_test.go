package seed

import (
	"context"
	"testing"
	"time"
)

// TestVerifyRedpandaTopic_BrokerReachable verifies that VerifyRedpandaTopic checks broker connectivity
// It should pass when the broker is reachable (even if topic doesn't exist yet).
// Topic auto-creation happens later in Phase 3 when producers first write.
func TestVerifyRedpandaTopic_BrokerReachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := VerifyRedpandaTopic(ctx)

	// Should pass because broker is healthy and reachable
	if err != nil {
		t.Fatalf("expected VerifyRedpandaTopic to pass when broker is reachable, got: %v", err)
	}
}
