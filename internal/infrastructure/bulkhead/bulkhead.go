// Package bulkhead provides a concurrency-limiting mechanism to isolate failure domains.
package bulkhead

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"workhub-service/internal/infrastructure/metrics"
)

// ErrBulkheadFull is returned when the bulkhead's concurrency limit has been reached.
var ErrBulkheadFull = errors.New("bulkhead is full")

// Bulkhead limits the number of concurrent executions of a given operation.
// It is safe for concurrent use.
//
// The zero value is not usable; always construct with New.
type Bulkhead struct {
	name string
	sema chan struct{}

	// Cached metric handles — avoids WithLabelValues on the hot path.
	inFlight      prometheus.Gauge
	rejectedTotal prometheus.Counter
}

// New creates a Bulkhead with the given name and maximum concurrency.
//
// Panics if name is empty (programmer error — metric labels would collide).
// Coerces maxConcurrent to at least 1.
func New(name string, maxConcurrent int) *Bulkhead {
	if name == "" {
		panic("bulkhead: name must not be empty")
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Bulkhead{
		name:          name,
		sema:          make(chan struct{}, maxConcurrent),
		inFlight:      metrics.BulkheadInFlight.WithLabelValues(name),
		rejectedTotal: metrics.BulkheadRejectedTotal.WithLabelValues(name),
	}
}

// Do executes fn if the concurrency limit has not been reached.
//
// Behavior:
//   - Non-blocking: returns ErrBulkheadFull immediately if the bulkhead is full.
//   - Respects ctx cancellation before acquiring a slot.
//   - Panics if fn is nil.
//
// The provided ctx is passed through to fn; callers are responsible
// for honoring it inside fn.
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		panic("bulkhead: nil fn passed to Do")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case b.sema <- struct{}{}:
		b.inFlight.Inc()
		defer func() {
			b.inFlight.Dec()
			<-b.sema
		}()
		return fn(ctx)

	default:
		b.rejectedTotal.Inc()
		return ErrBulkheadFull
	}
}
