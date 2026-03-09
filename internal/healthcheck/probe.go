package healthcheck

import (
	"context"
	"fmt"
	"log"
	"time"

	tigerbeetle_go "github.com/tigerbeetle/tigerbeetle-go"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
	"github.com/twmb/franz-go/pkg/kgo"
)

// ProbeResult holds the result of health check operations.
type ProbeResult struct {
	TigerBeetleHealthy bool
	RedpandaHealthy    bool
	Errors             []string
}

// ProbeTigerBeetle checks if the TigerBeetle cluster is healthy and in consensus.
// It creates a client, issues a LookupAccounts query, and confirms the response.
func ProbeTigerBeetle(ctx context.Context, addresses []string) error {
	client, err := tigerbeetle_go.NewClient(types.ToUint128(0), addresses)
	if err != nil {
		return fmt.Errorf("TigerBeetle client creation failed: %w", err)
	}
	defer client.Close()

	// Use a non-existent ID to verify cluster is responding
	accounts, err := client.LookupAccounts([]types.Uint128{types.ToUint128(9999999)})
	if err != nil {
		return fmt.Errorf("TigerBeetle cluster not responding: %w", err)
	}

	log.Printf("TigerBeetle cluster is healthy ✓ (found %d accounts)", len(accounts))
	return nil
}

// ProbeRedpanda checks if the Redpanda broker is accepting connections.
// It creates a Kafka client and issues a Ping command.
func ProbeRedpanda(ctx context.Context, broker string) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.DialTimeout(5*time.Second),
		kgo.ConnIdleTimeout(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("Redpanda client creation failed: %w", err)
	}
	defer client.Close()

	err = client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("Redpanda broker not responding: %w", err)
	}

	log.Printf("Redpanda broker is healthy ✓")
	return nil
}

// ProbeAll runs both TigerBeetle and Redpanda health checks.
// It returns a ProbeResult with the status of both services and any errors encountered.
func ProbeAll(ctx context.Context, tbAddresses []string, redpandaBroker string) *ProbeResult {
	result := &ProbeResult{
		Errors: []string{},
	}

	if err := ProbeTigerBeetle(ctx, tbAddresses); err != nil {
		result.Errors = append(result.Errors, err.Error())
		result.TigerBeetleHealthy = false
	} else {
		result.TigerBeetleHealthy = true
	}

	if err := ProbeRedpanda(ctx, redpandaBroker); err != nil {
		result.Errors = append(result.Errors, err.Error())
		result.RedpandaHealthy = false
	} else {
		result.RedpandaHealthy = true
	}

	return result
}
