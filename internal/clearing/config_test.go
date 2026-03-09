package clearing

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDefaultConfigHasCorrectValues(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, []string{"localhost:9092"}, config.RedpandaBrokers)
	assert.Equal(t, "pix-payments", config.Topic)
	assert.Equal(t, "clearing-engine", config.ConsumerGroup)
	assert.Equal(t, "127.0.0.1", config.TigerBeetleCluster)
	assert.Equal(t, 3001, config.TigerBeetlePort)
	assert.Equal(t, 0.95, config.BankBAcceptRate)
	assert.Equal(t, int64(0), config.BankBDelayMS)
	assert.Equal(t, uint32(30), config.TransferTimeoutSeconds)
}

func TestConfigCanBeCustomized(t *testing.T) {
	config := &Config{
		RedpandaBrokers:        []string{"broker1:9092", "broker2:9092"},
		Topic:                  "custom-topic",
		ConsumerGroup:          "custom-group",
		TigerBeetleCluster:     "192.168.1.100",
		TigerBeetlePort:        3001,
		BankBAcceptRate:        0.75,
		BankBDelayMS:           100,
		TransferTimeoutSeconds: 60,
	}

	assert.Equal(t, 2, len(config.RedpandaBrokers))
	assert.Equal(t, "custom-topic", config.Topic)
	assert.Equal(t, "custom-group", config.ConsumerGroup)
	assert.Equal(t, "192.168.1.100", config.TigerBeetleCluster)
	assert.Equal(t, 3001, config.TigerBeetlePort)
	assert.Equal(t, 0.75, config.BankBAcceptRate)
	assert.Equal(t, int64(100), config.BankBDelayMS)
	assert.Equal(t, uint32(60), config.TransferTimeoutSeconds)
}
