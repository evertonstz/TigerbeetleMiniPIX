package clearing

type Config struct {
	// Redpanda
	RedpandaBrokers []string
	Topic           string
	ConsumerGroup   string

	// TigerBeetle
	TigerBeetleCluster string
	TigerBeetlePort    int

	// Bank B Simulation
	BankBAcceptRate float64 // 0.0 - 1.0
	BankBDelayMS    int64   // Milliseconds

	// Transfer
	TransferTimeoutSeconds uint32 // Default pending transfer timeout

	// Batch flush timeout (milliseconds) — when no new messages arrive within this window, submit partial batch
	// Range: 50-500ms recommended. Lower = more responsive but smaller batches; Higher = fewer batches but higher latency
	// Default: 100ms (balances throughput and latency)
	BatchFlushTimeoutMs int64
}

func DefaultConfig() *Config {
	return &Config{
		RedpandaBrokers:        []string{"localhost:9092"},
		Topic:                  "pix-payments",
		ConsumerGroup:          "clearing-engine",
		TigerBeetleCluster:     "127.0.0.1",
		TigerBeetlePort:        3001,
		BankBAcceptRate:        0.95,
		BankBDelayMS:           0,
		TransferTimeoutSeconds: 30,
		BatchFlushTimeoutMs:    100,
	}
}
