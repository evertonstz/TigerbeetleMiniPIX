package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/healthcheck"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tbAddresses := []string{
		"127.0.0.1:3001",
		"127.0.0.1:3002",
		"127.0.0.1:3003",
		"127.0.0.1:3004",
		"127.0.0.1:3005",
	}

	redpandaBroker := "127.0.0.1:9092"

	log.Println("Starting infrastructure health check...")
	result := healthcheck.ProbeAll(ctx, tbAddresses, redpandaBroker)

	if result.TigerBeetleHealthy && result.RedpandaHealthy {
		log.Println("✓ All systems ready")
		os.Exit(0)
	} else {
		for _, errMsg := range result.Errors {
			log.Printf("ERROR: %s", errMsg)
		}
		log.Println("✗ Infrastructure not ready")
		os.Exit(1)
	}
}
