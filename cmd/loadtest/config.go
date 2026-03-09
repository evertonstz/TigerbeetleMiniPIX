package main

import (
	"flag"
	"time"
)

// Config holds all load test configuration parameters.
type Config struct {
	// Payments generation
	PaymentCount int   // Total number of payments to generate
	Seed         int64 // Random seed for reproducibility

	// Concurrency and rate limiting
	Concurrency     int // Number of concurrent worker goroutines
	RateLimitPerSec int // Target rate: messages per second
	RateLimitBurst  int // Burst size for rate limiter token bucket
	TimeoutSeconds  int // Timeout for individual operations

	// Redpanda
	RedpandaBrokers string // Comma-separated broker addresses
	Topic           string // Kafka topic for payments
	ConsumerGroup   string // Consumer group for offset tracking

	// TigerBeetle
	TigerBeetleAddr string // TigerBeetle cluster address
}

// DefaultConfig returns sensible defaults for load testing.
func DefaultConfig() *Config {
	return &Config{
		PaymentCount:    10000,
		Seed:            time.Now().UnixNano(),
		Concurrency:     100,
		RateLimitPerSec: 10000,
		RateLimitBurst:  110,
		TimeoutSeconds:  120,
		RedpandaBrokers: "127.0.0.1:9092",
		Topic:           "pix-payments",
		ConsumerGroup:   "clearing-engine",
		TigerBeetleAddr: "127.0.0.1:3001",
	}
}

// BindFlags binds CLI flags to Config fields.
// Call this from main() after parsing flags to populate Config from command-line arguments.
func (c *Config) BindFlags() {
	flag.IntVar(&c.PaymentCount, "payments", c.PaymentCount, "Total number of payments to generate")
	flag.Int64Var(&c.Seed, "seed", c.Seed, "Random seed (default: current Unix nanos)")
	flag.IntVar(&c.Concurrency, "concurrency", c.Concurrency, "Number of concurrent workers")
	flag.IntVar(&c.RateLimitPerSec, "rate", c.RateLimitPerSec, "Target rate in messages per second")
	flag.IntVar(&c.RateLimitBurst, "burst", c.RateLimitBurst, "Rate limiter burst size")
	flag.IntVar(&c.TimeoutSeconds, "timeout", c.TimeoutSeconds, "Operation timeout in seconds")
	flag.StringVar(&c.RedpandaBrokers, "brokers", c.RedpandaBrokers, "Comma-separated Redpanda brokers")
	flag.StringVar(&c.Topic, "topic", c.Topic, "Kafka topic for payments")
	flag.StringVar(&c.ConsumerGroup, "group", c.ConsumerGroup, "Consumer group for offset tracking")
	flag.StringVar(&c.TigerBeetleAddr, "tigerbeetle", c.TigerBeetleAddr, "TigerBeetle cluster address")
}
