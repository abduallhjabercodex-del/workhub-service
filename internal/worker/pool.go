// Package worker provides asynchronous processing primitives.
package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/metrics"
)

var (
	ErrQueueFull   = errors.New("worker queue is full")
	ErrPoolStopped = errors.New("worker pool is stopped")
)

// Handler defines a function to process events.
type Handler func(context.Context, domain.Event) error

// WorkerPool manages a pool of workers that process events from a bounded queue.
//
// Lifecycle:
//   - Start launches N goroutines that consume from the queue.
//   - Submit attempts a non-blocking enqueue (fail-fast on full queue).
//   - Stop signals shutdown and waits for in-flight jobs to finish.
//
// Shutdown semantics:
//   - Workers exit on either ctx.Done() OR p.quit (whichever fires first).
//   - Events still in the queue when workers exit are NOT processed
//     (the caller is responsible for draining).
//   - Stop is idempotent (safe to call multiple times).
//   - The queue channel is NEVER closed to avoid a race with Submit.
type WorkerPool struct {
	workers   int
	queueSize int // عدد المهام التي ممكن ان تنتظر
	queue     chan domain.Event
	wg        sync.WaitGroup
	quit      chan struct{} // اغلاق ال pool
	stopOnce  sync.Once
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(workers, queueSize int) *WorkerPool {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 100
	}
	p := &WorkerPool{
		workers:   workers,
		queueSize: queueSize,
		queue:     make(chan domain.Event, queueSize),
		quit:      make(chan struct{}),
	}
	metrics.RegisterWorkerQueueDepthGauge("default", func() float64 {
		return float64(len(p.queue))
	})
	return p
}

// Start launches the workers. Must be called once.
func (p *WorkerPool) Start(ctx context.Context, handler Handler) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func(workerID int) {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-p.quit:
					return
				case event := <-p.queue:
					p.processOne(ctx, event, handler)
				}
			}
		}(i)
	}
}

func (p *WorkerPool) processOne(ctx context.Context, event domain.Event, handler Handler) {
	start := time.Now()
	err := handler(ctx, event)
	eventType := event.Type
	if eventType == "" {
		eventType = "unknown"
	}
	metrics.WorkerProcessingDuration.
		WithLabelValues(eventType).
		Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.WorkerJobsTotal.WithLabelValues(eventType, "failed").Inc()
	} else {
		metrics.WorkerJobsTotal.WithLabelValues(eventType, "success").Inc()
	}
}

// Submit attempts to enqueue an event without blocking.
// Returns ErrQueueFull if the queue is full, ErrPoolStopped if stopped.
func (p *WorkerPool) Submit(ctx context.Context, event domain.Event) error {
	select {
	case <-p.quit:
		return ErrPoolStopped
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	select {
	case <-p.quit:
		return ErrPoolStopped
	case <-ctx.Done():
		return ctx.Err()
	case p.queue <- event: //  نجح الوضع
		return nil
	default: // الطابور ممتلئ!
		return ErrQueueFull //  فشل الوضع، طابور ممتلئ   Backpressure
	}
}

// Stop signals shutdown and waits for all in-flight jobs to finish.
// Idempotent. Does NOT close the queue channel (avoids panic race).
func (p *WorkerPool) Stop() {
	p.stopOnce.Do(func() {
		close(p.quit)
	})
	p.wg.Wait()
}
