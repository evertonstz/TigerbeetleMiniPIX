package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/time/rate"
)

// ProducerClient interface defines the methods ProducerWorker needs from kgo.Client
type ProducerClient interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Flush(ctx context.Context) error
	LeaveGroup()
}

// ProducerWorker orchestrates payment generation, rate limiting, and production to Redpanda.
type ProducerWorker struct {
	limiter   *rate.Limiter
	producer  ProducerClient
	generator *PaymentGenerator
	metrics   *MetricsCollector
	topic     string
	tracker   *OffsetTracker
	sendTimes sync.Map // maps paymentUUID → sendTime (Unix nanoseconds)
}

// NewProducerWorker creates a new worker with shared dependencies.
func NewProducerWorker(
	limiter *rate.Limiter,
	producer ProducerClient,
	generator *PaymentGenerator,
	metrics *MetricsCollector,
	topic string,
	tracker *OffsetTracker,
) *ProducerWorker {
	return &ProducerWorker{
		limiter:   limiter,
		producer:  producer,
		generator: generator,
		metrics:   metrics,
		topic:     topic,
		tracker:   tracker,
	}
}

// Run executes the worker for 'count' payments, enforcing rate limiting and producing to Redpanda.
// Spawned as a goroutine by main orchestration.
func (pw *ProducerWorker) Run(ctx context.Context, wg *sync.WaitGroup, count int) {
	defer wg.Done()

	for i := 0; i < count; i++ {
		err := pw.limiter.Wait(ctx)
		if err != nil {
			pw.metrics.RecordError()
			break
		}

		payment, err := pw.generator.Generate()
		if err != nil {
			pw.metrics.RecordError()
			continue
		}

		payloadBytes, err := json.Marshal(payment)
		if err != nil {
			pw.metrics.RecordError()
			continue
		}

		sendTime := time.Now()
		results := pw.producer.ProduceSync(ctx, &kgo.Record{
			Topic: pw.topic,
			Key:   []byte(payment.PaymentUUID),
			Value: payloadBytes,
		})

		if results.FirstErr() != nil {
			pw.metrics.RecordError()
			continue
		}

		if len(results) > 0 && results[0].Record != nil {
			offset := results[0].Record.Offset
			pw.sendTimes.Store(payment.PaymentUUID, sendTime.UnixNano())

			if pw.tracker != nil {
				pw.tracker.RecordMessageOffset(payment.PaymentUUID, offset)

				// Wait for offset confirmation and record E2E latency
				go func(uuid string, offsetVal int64, sendTimeNs int64) {
					if confirmTime, err := pw.tracker.WaitForMessageOffset(ctx, offsetVal, 5*time.Second); err == nil {
						e2eLatency := confirmTime.Sub(time.Unix(0, sendTimeNs)).Nanoseconds()
						pw.metrics.RecordE2ELatency(e2eLatency)
					}
				}(payment.PaymentUUID, offset, sendTime.UnixNano())
			}
		}

		latencyNs := time.Since(sendTime).Nanoseconds()
		pw.metrics.RecordLatency(latencyNs)
		pw.metrics.RecordSent()
	}
}

// ProducerPool manages a fixed pool of workers and delegates work distribution.
type ProducerPool struct {
	workers []*ProducerWorker
	wg      *sync.WaitGroup
}

// NewProducerPool creates a pool of N workers.
func NewProducerPool(
	n int,
	limiter *rate.Limiter,
	producer ProducerClient,
	generator *PaymentGenerator,
	metrics *MetricsCollector,
	topic string,
	tracker *OffsetTracker,
) *ProducerPool {
	workers := make([]*ProducerWorker, n)
	for i := 0; i < n; i++ {
		workers[i] = NewProducerWorker(limiter, producer, generator, metrics, topic, tracker)
	}
	return &ProducerPool{
		workers: workers,
		wg:      &sync.WaitGroup{},
	}
}

// Run distributes totalPayments across the pool and waits for completion.
func (pp *ProducerPool) Run(ctx context.Context, totalPayments int) error {
	paymentsPerWorker := totalPayments / len(pp.workers)
	remainder := totalPayments % len(pp.workers)

	for i, worker := range pp.workers {
		count := paymentsPerWorker
		if i < remainder {
			count++
		}

		pp.wg.Add(1)
		go worker.Run(ctx, pp.wg, count)
	}

	pp.wg.Wait()
	return nil
}
