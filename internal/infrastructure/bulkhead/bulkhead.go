// Package bulkhead provides a concurrency-limiting mechanism to isolate failure domains.
package bulkhead

import (
	"context"
	"errors"

	"workhub-service/internal/infrastructure/metrics"
)

// ErrBulkheadFull is returned when the bulkhead's concurrency limit has been reached.
var ErrBulkheadFull = errors.New("bulkhead is full")

// Bulkhead limits the number of concurrent executions of a given operation.
type Bulkhead struct {
	name string
	sema chan struct{}
}

// New creates a new Bulkhead with the specified name and maximum concurrency.
func New(name string, maxConcurrent int) *Bulkhead {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Bulkhead{
		name: name,
		sema: make(chan struct{}, maxConcurrent),
	}
}

// Do executes the given function if the concurrency limit has not been reached.
// If the limit is reached, it returns ErrBulkheadFull immediately without blocking.
// It also respects the provided context for cancellation before or during execution.
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case b.sema <- struct{}{}:
		metrics.BulkheadInFlight.WithLabelValues(b.name).Inc()
		defer func() {
			metrics.BulkheadInFlight.WithLabelValues(b.name).Dec()
			<-b.sema
		}()
		return fn(ctx)
	default:
		metrics.BulkheadRejectedTotal.WithLabelValues(b.name).Inc()
		return ErrBulkheadFull
	}
}
