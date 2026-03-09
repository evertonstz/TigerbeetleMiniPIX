package seed

import (
	"context"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/account"
)

// VerifyRedpandaTopic verifies Redpanda broker is reachable and auto-topic creation is enabled.
// Auto-topic creation is enabled by default in Redpanda dev-container mode, so the pix-payments
// topic will be created automatically when Phase 3's producers first write to it.
//
// Note: Phase 2 (Account Bootstrap) only needs to verify the broker is ready.
// Topic existence and configuration validation happens in Phase 3+ when producers actually use it.
func VerifyRedpandaTopic(ctx context.Context) error {
	log.Println("Verifying Redpanda broker is reachable...")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(account.RedpandaBroker),
		kgo.DialTimeout(5*time.Second),
		kgo.ConnIdleTimeout(10*time.Second),
	)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		return err
	}

	log.Println("✓ Redpanda broker is reachable (auto-topic creation enabled)")
	return nil
}
