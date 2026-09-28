package bulkhead

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBulkhead_Do_Success(t *testing.T) {
	b := New("test1", 2) // Max 2 concurrent executions
	ctx := context.Background()

	var wg sync.WaitGroup
	var activeCount atomic.Int32
	var maxActive atomic.Int32
	var successCount atomic.Int32
	var rejectCount atomic.Int32

	blockCh := make(chan struct{})

	// Launch 5 concurrent operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := b.Do(ctx, func(innerCtx context.Context) error {
				current := activeCount.Add(1)
				defer activeCount.Add(-1)

				// Track the maximum number of concurrent executions
				for {
					max := maxActive.Load()
					if current > max {
						if maxActive.CompareAndSwap(max, current) {
							break
						}
					} else {
						break
					}
				}

				<-blockCh // block until test says go
				return nil
			})

			if err == nil {
				successCount.Add(1)
			} else if err == ErrBulkheadFull {
				rejectCount.Add(1)
			}
		}()
	}

	// Give goroutines a moment to hit the bulkhead
	time.Sleep(50 * time.Millisecond)

	// Unblock the executing functions
	close(blockCh)

	// Wait for all to finish
	wg.Wait()

	if successCount.Load() != 2 {
		t.Fatalf("expected exactly 2 successful executions, got %d", successCount.Load())
	}

	if rejectCount.Load() != 3 {
		t.Fatalf("expected exactly 3 rejected executions, got %d", rejectCount.Load())
	}

	if maxActive.Load() > 2 {
		t.Fatalf("expected max active executions to be <= 2, got %d", maxActive.Load())
	}
}

func TestBulkhead_Do_ContextCancellation(t *testing.T) {
	b := New("test2", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	err := b.Do(ctx, func(context.Context) error {
		return nil
	})

	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
