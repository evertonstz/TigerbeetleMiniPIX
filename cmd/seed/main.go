package main

import (
	"context"
	"log"
	"os"
	"time"

	tigerbeetle_go "github.com/tigerbeetle/tigerbeetle-go"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/account"
	"github.com/evertoncorreia/tigerbeetle-minipix/internal/healthcheck"
	"github.com/evertoncorreia/tigerbeetle-minipix/internal/seed"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[seed] ")

	var exitCode int
	defer func() {
		if exitCode != 0 {
			log.Printf("Seed failed with exit code %d", exitCode)
		}
		os.Exit(exitCode)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("=== TigerbeetleMiniPIX Account Bootstrap ===")

	log.Println("\nStep 1: Verifying infrastructure health...")
	tbAddresses := []string{
		"127.0.0.1:3001",
		"127.0.0.1:3002",
		"127.0.0.1:3003",
		"127.0.0.1:3004",
		"127.0.0.1:3005",
	}
	probeResult := healthcheck.ProbeAll(ctx, tbAddresses, account.RedpandaBroker)
	if len(probeResult.Errors) > 0 || !probeResult.TigerBeetleHealthy || !probeResult.RedpandaHealthy {
		for _, errMsg := range probeResult.Errors {
			log.Printf("✗ Health check failed: %v", errMsg)
		}
		exitCode = 1
		return
	}
	log.Println("✓ TigerBeetle and Redpanda are healthy")

	log.Println("\nStep 2: Connecting to TigerBeetle...")
	client, err := tigerbeetle_go.NewClient(types.ToUint128(0), tbAddresses)
	if err != nil {
		log.Printf("✗ Failed to create TigerBeetle client: %v", err)
		exitCode = 1
		return
	}
	defer client.Close()
	log.Println("✓ Connected to TigerBeetle cluster")

	log.Println("\nStep 3: Seeding accounts...")
	if err := seed.SeedAll(ctx, client); err != nil {
		log.Printf("✗ Account seeding failed: %v", err)
		exitCode = 1
		return
	}

	log.Println("\nStep 4: Verifying Redpanda topic...")
	if err := seed.VerifyRedpandaTopic(ctx); err != nil {
		log.Printf("✗ Topic verification failed: %v", err)
		exitCode = 1
		return
	}

	log.Println("\n=== Bootstrap Complete ===")
	log.Printf("✓ All 5,005 accounts created/verified")
	log.Printf("✓ Redpanda pix-payments topic verified")
	log.Println("Ready for Phase 3: Core Clearing Engine")
	exitCode = 0
}
