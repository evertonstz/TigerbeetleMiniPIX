package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/time/rate"
)

func main() {
	cfg := DefaultConfig()
	cfg.BindFlags()
	flag.Parse()

	log.Printf("Starting load test with config: Payments=%d, Concurrency=%d, Rate=%d msg/sec, Burst=%d",
		cfg.PaymentCount, cfg.Concurrency, cfg.RateLimitPerSec, cfg.RateLimitBurst)

	startTime := time.Now()
	metrics := NewMetricsCollector()

	brokers := strings.Split(cfg.RedpandaBrokers, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	producer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequestTimeoutOverhead(time.Duration(cfg.TimeoutSeconds)*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create Redpanda producer: %v", err)
	}
	defer producer.Flush(context.Background())
	defer producer.LeaveGroup()

	tracker := NewOffsetTracker(brokers, cfg.Topic, cfg.ConsumerGroup)
	defer tracker.Close()

	generator, err := NewPaymentGenerator(cfg)
	if err != nil {
		log.Fatalf("Failed to create payment generator: %v", err)
	}

	limiter := rate.NewLimiter(
		rate.Limit(float64(cfg.RateLimitPerSec)),
		cfg.RateLimitBurst,
	)

	pool := NewProducerPool(
		cfg.Concurrency,
		limiter,
		producer,
		generator,
		metrics,
		cfg.Topic,
		tracker,
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.TimeoutSeconds)*time.Second,
	)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- pool.Run(ctx, cfg.PaymentCount)
	}()

	select {
	case <-sigChan:
		log.Println("Graceful shutdown initiated...")
		cancel()
		<-errChan
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Load test failed: %v", err)
		}
	}

	elapsed := time.Since(startTime)
	metrics.SetElapsed(elapsed)
	metrics.PrintReport()
}
