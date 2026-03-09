package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/evertoncorreia/tigerbeetle-minipix/internal/clearing"
	tb "github.com/tigerbeetle/tigerbeetle-go"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

func main() {
	configPath := flag.String("config", "", "Path to config file (optional)")
	flag.Parse()

	config := clearing.DefaultConfig()
	_ = configPath // TODO: implement config file loading if needed

	log.Printf("Starting PIX Clearing Engine")
	log.Printf("Redpanda brokers: %v", config.RedpandaBrokers)
	log.Printf("Topic: %s", config.Topic)
	log.Printf("Consumer group: %s", config.ConsumerGroup)
	log.Printf("Bank B accept rate: %.0f%%", config.BankBAcceptRate*100)

	tbClient, err := tb.NewClient(
		types.ToUint128(0),
		[]string{config.TigerBeetleCluster + ":3001"},
	)
	if err != nil {
		log.Fatalf("failed to connect to TigerBeetle: %v", err)
	}
	defer tbClient.Close()

	adapter := clearing.NewTigerBeetleAdapter(tbClient)

	phase1 := clearing.NewPhase1Executor(adapter)
	phase2 := clearing.NewPhase2Executor(adapter)
	bankB := clearing.NewBankBSimulator(config.BankBAcceptRate, config.BankBDelayMS)

	consumer := clearing.NewConsumer(
		config.RedpandaBrokers,
		config.Topic,
		config.ConsumerGroup,
		config,
		phase1,
		phase2,
		bankB,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		log.Printf("Starting consumer...")
		err := consumer.Start(ctx)
		if err != nil {
			log.Printf("Consumer error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Printf("Shutting down PIX Clearing Engine...")
	cancel()
}
