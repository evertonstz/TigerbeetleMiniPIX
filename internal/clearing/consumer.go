package clearing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Consumer struct {
	brokers       []string
	topic         string
	consumerGroup string
	config        *Config
	client        *kgo.Client
	phase1        *Phase1Executor
	phase2        *Phase2Executor
	bankB         *BankBSimulator
	offsetManager *OffsetManager
}

func NewConsumer(
	brokers []string,
	topic string,
	consumerGroup string,
	config *Config,
	phase1 *Phase1Executor,
	phase2 *Phase2Executor,
	bankB *BankBSimulator,
) *Consumer {
	return &Consumer{
		brokers:       brokers,
		topic:         topic,
		consumerGroup: consumerGroup,
		config:        config,
		phase1:        phase1,
		phase2:        phase2,
		bankB:         bankB,
		offsetManager: NewOffsetManager(),
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(c.brokers...),
		kgo.ConsumerGroup(c.consumerGroup),
		kgo.ConsumeTopics(c.topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		log.Printf("failed to create Kafka client: %v", err)
		return err
	}

	c.client = client
	log.Printf("Consumer started for topic %s in group %s", c.topic, c.consumerGroup)
	return nil
}

func (c *Consumer) Stop() error {
	if c.client != nil {
		c.client.LeaveGroup()
		c.client.Close()
	}
	return nil
}

// GetOffsetManager returns the offset manager for testing purposes
func (c *Consumer) GetOffsetManager() *OffsetManager {
	return c.offsetManager
}

// handlePaymentPhase1 executes only Phase 1 (create pending transfers)
func (c *Consumer) handlePaymentPhase1(payment *PaymentMessage) error {
	if payment == nil {
		return errors.New("payment message is nil")
	}

	chain := NewTransferChain(
		payment.PaymentUUID,
		payment.SenderAccountID,
		payment.RecipientAccountID,
		payment.AmountCents,
	)

	result, err := c.phase1.Execute(context.Background(), chain)
	if err != nil {
		log.Printf("Phase 1 execution error: %v", err)
		return err
	}

	if !result.Success {
		log.Printf("Phase 1 failed: %s", result.ErrorMsg)
		return errors.New(result.ErrorMsg)
	}

	return nil
}

// HandlePayment executes the full payment flow (Phase 1 + Bank B simulation + Phase 2)
func (c *Consumer) HandlePayment(payment *PaymentMessage) error {
	if payment == nil {
		return errors.New("payment message is nil")
	}

	log.Printf("Processing payment %s", payment.PaymentUUID)

	chain := NewTransferChain(
		payment.PaymentUUID,
		payment.SenderAccountID,
		payment.RecipientAccountID,
		payment.AmountCents,
	)

	if c.phase1 == nil {
		return fmt.Errorf("Phase 1 executor not initialized")
	}

	result, err := c.phase1.Execute(context.Background(), chain)
	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("Phase 1 failed: %s", result.ErrorMsg)
	}

	if c.bankB == nil {
		return fmt.Errorf("Bank B simulator not initialized")
	}

	bankBAccepted := c.bankB.Simulate(payment.PaymentUUID)
	log.Printf("Bank B response for payment %s: %v", payment.PaymentUUID, bankBAccepted)

	if c.phase2 == nil {
		return fmt.Errorf("Phase 2 executor not initialized")
	}

	result2, err := c.phase2.Execute(context.Background(), chain, bankBAccepted)
	if err != nil {
		return err
	}

	if !result2.Success {
		return fmt.Errorf("Phase 2 failed: %s", result2.ErrorMsg)
	}

	log.Printf("Payment %s completed successfully", payment.PaymentUUID)
	return nil
}

// HandlePaymentWithOffset executes the full payment flow and commits the offset after successful Phase 2
func (c *Consumer) HandlePaymentWithOffset(payment *PaymentMessage, topic string, partition int32, offset int64) error {
	err := c.HandlePayment(payment)
	if err != nil {
		return err
	}

	c.offsetManager.MarkForCommit(topic, partition, offset+1)

	return nil
}

// Deprecated: Use PollWithBatching() instead.
// Poll processes one payment at a time without batch accumulation.
// For Phase 4+ batching, use PollWithBatching() instead.
func (c *Consumer) Poll(ctx context.Context, handler func(*PaymentMessage) error) error {
	if c.client == nil {
		return errors.New("consumer not started")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fetches := c.client.PollFetches(ctx)

		if fetches.IsClientClosed() {
			return errors.New("kafka client closed")
		}

		fetches.EachRecord(func(r *kgo.Record) {
			payment, err := DeserializePayment(r.Value)
			if err != nil {
				log.Printf("failed to deserialize payment: %v", err)
				return
			}

			if err := handler(payment); err != nil {
				log.Printf("handler error: %v", err)
				return
			}

			c.client.CommitOffsetsSync(ctx, map[string]map[int32]kgo.EpochOffset{
				r.Topic: {
					r.Partition: {
						Epoch:  -1,
						Offset: r.Offset + 1,
					},
				},
			}, nil)
		})
	}
}

// buildCommitMap converts offsetManager offsets into Kafka CommitOffsetsSync format
func (c *Consumer) buildCommitMap() map[string]map[int32]kgo.EpochOffset {
	offsets := c.offsetManager.GetAllOffsets()
	commitMap := make(map[string]map[int32]kgo.EpochOffset)
	for topic, partitions := range offsets {
		commitMap[topic] = make(map[int32]kgo.EpochOffset)
		for partition, offset := range partitions {
			commitMap[topic][partition] = kgo.EpochOffset{
				Epoch:  -1,
				Offset: offset,
			}
		}
	}
	return commitMap
}

// submitAndProcessBatch submits accumulated Phase 1 transfers and processes Phase 2 for all payments in batch
func (c *Consumer) submitAndProcessBatch(ctx context.Context, accumulator *BatchAccumulator) error {
	payments := accumulator.Payments()
	transfers := accumulator.Transfers()

	if len(transfers) == 0 {
		return nil
	}

	log.Printf("[BATCH] Submitting: %d payments (%d transfers)", len(payments), len(transfers))

	startTime := time.Now()

	// Phase 1: Submit all transfers in single batch
	// TigerBeetle returns a flat error array with Index positions corresponding to submitted transfers
	results := c.phase1.tbClient.CreateTransfers(transfers)

	latencyMs := float64(time.Since(startTime).Milliseconds())

	// Map errors to payments using index-based lookup
	// Key insight: results array only contains ERRORS (empty = all success)
	// Each error.Index corresponds to a transfer position in the submitted batch
	phase1ErrorMap := make(map[string]error)

	for _, result := range results {
		if result.Result == types.TransferOK || result.Result == types.TransferExists {
			continue
		}

		// Map transfer index to payment using accumulator's index mapping
		// This prevents off-by-one errors: GetPaymentForTransferIndex handles the range check
		payment, err := accumulator.GetPaymentForTransferIndex(result.Index)
		if err != nil {
			log.Printf("[BATCH] ERROR: Could not map transfer index %d to payment: %v", result.Index, err)
			continue
		}

		phase1ErrorMap[payment.PaymentUUID] = fmt.Errorf("transfer index %d failed: %v", result.Index, result.Result)
		log.Printf("[BATCH] Payment %s Phase 1 FAILED: transfer at index %d = %v",
			payment.PaymentUUID, result.Index, result.Result)
	}

	// Phase 2 & Bank B: Process each payment independently
	phase2CompletedCount := 0
	for _, payment := range payments {
		if _, hasError := phase1ErrorMap[payment.PaymentUUID]; hasError {
			log.Printf("[BATCH] Skipping Phase 2 for %s (Phase 1 failed)", payment.PaymentUUID)
			continue
		}

		bankBAccepted := c.bankB.Simulate(payment.PaymentUUID)
		log.Printf("[BATCH] Payment %s Bank B decision: accepted=%v", payment.PaymentUUID, bankBAccepted)

		chain := NewTransferChain(
			payment.PaymentUUID,
			payment.SenderAccountID,
			payment.RecipientAccountID,
			payment.AmountCents,
		)

		result2, err := c.phase2.Execute(ctx, chain, bankBAccepted)
		if err != nil || !result2.Success {
			log.Printf("[BATCH] Payment %s Phase 2 failed: %v", payment.PaymentUUID, err)
		} else {
			log.Printf("[BATCH] Payment %s Phase 2 success", payment.PaymentUUID)
			phase2CompletedCount++
		}
	}

	log.Printf("[BATCH] Completed: %d / %d payments Phase 2 done (Phase 1 errors: %d)", phase2CompletedCount, len(payments), len(phase1ErrorMap))

	successCount := len(transfers) - len(results)
	RecordBatchSubmission(BatchMetrics{
		BatchSize:    len(transfers),
		PaymentCount: len(payments),
		LatencyMs:    latencyMs,
		ErrorCount:   len(results),
		SuccessCount: successCount,
	})

	histogram := GetBatchSizeHistogram()
	log.Printf("[METRICS] %s", FormatHistogram(histogram))

	return nil
}

// PollWithBatching implements the main polling loop with batch accumulation
// This replaces the old Poll() method and is the new primary entry point
func (c *Consumer) PollWithBatching(ctx context.Context) error {
	if c.client == nil {
		return errors.New("consumer not started")
	}

	accumulator := NewBatchAccumulator()

	flushTimeoutMs := c.config.BatchFlushTimeoutMs
	if flushTimeoutMs == 0 {
		flushTimeoutMs = 100
	}
	flushTicker := time.NewTicker(time.Duration(flushTimeoutMs) * time.Millisecond)
	defer flushTicker.Stop()

	log.Printf("[CONSUMER] Starting poll loop with batch flush timeout: %dms", flushTimeoutMs)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[CONSUMER] Context cancelled, flushing pending batch (%d transfers)", accumulator.Size())
			if accumulator.Size() > 0 {
				_ = c.submitAndProcessBatch(ctx, accumulator)
			}
			return ctx.Err()

		case <-flushTicker.C:
			if accumulator.Size() > 0 {
				log.Printf("[CONSUMER] Timeout triggered, flushing batch (%d transfers)", accumulator.Size())
				if err := c.submitAndProcessBatch(ctx, accumulator); err != nil {
					log.Printf("[CONSUMER] Error submitting batch on timeout: %v", err)
				}
				c.client.CommitOffsetsSync(ctx, c.buildCommitMap(), nil)
				accumulator.Clear()
			}

		default:
			fetches := c.client.PollFetches(ctx)
			if fetches.IsClientClosed() {
				return errors.New("kafka client closed")
			}

			fetches.EachRecord(func(r *kgo.Record) {
				payment, err := DeserializePayment(r.Value)
				if err != nil {
					log.Printf("[CONSUMER] Failed to deserialize payment: %v", err)
					return
				}

				isFull, err := accumulator.Add(payment)
				if err != nil {
					log.Printf("[CONSUMER] Batch capacity reached, submitting and adding new payment")
					_ = c.submitAndProcessBatch(ctx, accumulator)
					c.client.CommitOffsetsSync(ctx, c.buildCommitMap(), nil)
					accumulator.Clear()

					if _, err := accumulator.Add(payment); err != nil {
						log.Printf("[CONSUMER] Error adding payment after clear: %v", err)
						return
					}
					isFull = false
				}

				c.offsetManager.MarkForCommit(r.Topic, r.Partition, r.Offset+1)

				if isFull {
					log.Printf("[CONSUMER] Batch capacity reached after add, submitting batch")
					_ = c.submitAndProcessBatch(ctx, accumulator)
					c.client.CommitOffsetsSync(ctx, c.buildCommitMap(), nil)
					accumulator.Clear()
				}
			})
		}
	}
}
