package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// OffsetEvent represents a message offset confirmation event.
type OffsetEvent struct {
	PaymentUUID string
	ConfirmTime time.Time
}

// OffsetTracker monitors consumer group offset to detect when messages have been processed.
// It tracks sent message offsets and polls the consumer group offset to detect when they've been confirmed.
// Creates its own consumer client to join the consumer group and track offsets.
type OffsetTracker struct {
	client         *kgo.Client // Consumer client for offset queries
	topic          string
	consumerGroup  string
	brokers        []string
	offsetMap      map[string]int64 // PaymentUUID -> message offset
	mu             sync.RWMutex
	sentCount      int64
	confirmedCount int64
}

// NewOffsetTracker creates a new offset tracker with its own consumer client.
// The consumer client joins the specified consumer group to track offsets.
func NewOffsetTracker(brokers []string, topic, consumerGroup string) *OffsetTracker {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(consumerGroup),
	)
	if err != nil {
		log.Fatalf("Failed to create offset tracker consumer: %v", err)
	}

	return &OffsetTracker{
		client:        client,
		topic:         topic,
		consumerGroup: consumerGroup,
		brokers:       brokers,
		offsetMap:     make(map[string]int64),
	}
}

// RecordMessageOffset stores the offset of a produced message for tracking.
// Called after successful produce to correlate offset with payment UUID.
func (ot *OffsetTracker) RecordMessageOffset(paymentUUID string, offset int64) {
	ot.mu.Lock()
	defer ot.mu.Unlock()
	ot.offsetMap[paymentUUID] = offset
	ot.sentCount++
}

// WaitForMessageOffset polls the consumer group offset until it reaches or exceeds the target offset.
// Returns the confirmation time when the group offset >= target offset, or an error if timeout exceeded.
// Uses non-blocking offset polling with 50ms intervals.
func (ot *OffsetTracker) WaitForMessageOffset(ctx context.Context, targetOffset int64, timeout time.Duration) (time.Time, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return time.Time{}, fmt.Errorf("timeout waiting for offset %d", targetOffset)
		}

		committedOffsets := ot.client.CommittedOffsets()
		if topicOffsets, ok := committedOffsets[ot.topic]; ok {
			if len(topicOffsets) > 0 {
				epochOffset := topicOffsets[int32(0)]
				groupOffset := epochOffset.Offset
				if groupOffset >= targetOffset {
					ot.mu.Lock()
					ot.confirmedCount++
					ot.mu.Unlock()
					return time.Now(), nil
				}
			}
		}

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		}
	}
}

// MonitorOffsets runs in a background goroutine and polls consumer offset periodically.
// Returns a channel that emits OffsetEvent when a message offset is confirmed.
// Each event contains PaymentUUID and confirmation time for latency calculation.
func (ot *OffsetTracker) MonitorOffsets(ctx context.Context) <-chan OffsetEvent {
	eventChan := make(chan OffsetEvent, 100) // Buffered channel for non-blocking sends

	go func() {
		defer close(eventChan)

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		lastProcessedOffset := int64(-1)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				committedOffsets := ot.client.CommittedOffsets()
				if topicOffsets, ok := committedOffsets[ot.topic]; ok && len(topicOffsets) > 0 {
					epochOffset := topicOffsets[int32(0)]
					currentOffset := epochOffset.Offset

					if currentOffset > lastProcessedOffset {
						ot.mu.RLock()
						for uuid, msgOffset := range ot.offsetMap {
							if msgOffset <= currentOffset && msgOffset > lastProcessedOffset {
								select {
								case eventChan <- OffsetEvent{
									PaymentUUID: uuid,
									ConfirmTime: time.Now(),
								}:
								default:
								}
							}
						}
						ot.mu.RUnlock()

						lastProcessedOffset = currentOffset
					}
				}
			}
		}
	}()

	return eventChan
}

// GetStats returns sent and confirmed message counts for monitoring.
func (ot *OffsetTracker) GetStats() (sent, confirmed int64) {
	ot.mu.RLock()
	defer ot.mu.RUnlock()
	return ot.sentCount, ot.confirmedCount
}

// Close closes the offset tracker's consumer client.
func (ot *OffsetTracker) Close() error {
	ot.client.LeaveGroup()
	ot.client.Flush(context.Background())
	return nil
}
