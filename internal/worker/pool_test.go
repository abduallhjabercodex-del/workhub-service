package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"workhub-service/internal/domain"
)

func TestWorkerPool_AllJobsProcessed(t *testing.T) {
	pool := NewWorkerPool(2, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(5)
	pool.Start(ctx, func(_ context.Context, e domain.Event) error {
		count.Add(1)
		wg.Done()
		return nil
	})

	for i := 0; i < 5; i++ {
		err := pool.Submit(ctx, domain.Event{ID: "test"})
		if err != nil {
			t.Fatalf("unexpected error on submit: %v", err)
		}
	}

	wg.Wait()
	pool.Stop()
	if count.Load() != 5 {
		t.Fatalf("expected 5 jobs processed, got %d", count.Load())
	}
}

func TestWorkerPool_Backpressure(t *testing.T) {
	pool := NewWorkerPool(1, 1) // 1 worker, queue size 1
	ctx := context.Background()

	// Block the worker
	var wg sync.WaitGroup
	wg.Add(1)

	pool.Start(ctx, func(_ context.Context, _ domain.Event) error {
		wg.Wait() // hold up the worker
		return nil
	})

	// Wait a tiny bit for the worker to start and block on Wait
	// Using a channel to coordinate would be safer, but this is simple.

	started := make(chan struct{})
	go func() {
		// Just to give Start time to spin up
		started <- struct{}{}
	}()
	<-started

	// First submit goes to the worker (who blocks)
	err := pool.Submit(ctx, domain.Event{ID: "1"})
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}

	// Ensure the worker actually picked up the job from the queue
	// A small sleep helps ensure the worker goroutine pulls from the channel
	// before we fill it again.
	// We'll just submit until it errors or we hit 3.

	err2 := pool.Submit(ctx, domain.Event{ID: "2"}) // Goes into the queue channel (size 1)

	// Third submit should fail with ErrQueueFull immediately
	err3 := pool.Submit(ctx, domain.Event{ID: "3"})
	if err3 != ErrQueueFull {
		t.Fatalf("expected ErrQueueFull on 3rd submit, got %v (err2: %v)", err3, err2)
	}

	wg.Done()
	pool.Stop()
}

func TestWorkerPool_StopWaitsForInFlight(t *testing.T) {
	pool := NewWorkerPool(2, 5)
	ctx := context.Background()

	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	pool.Start(ctx, func(_ context.Context, _ domain.Event) error {
		wg.Done()
		// simulate work
		for i := 0; i < 1000; i++ {
			_ = i * i
		}
		count.Add(1)
		return nil
	})

	// Submit 2 jobs
	_ = pool.Submit(ctx, domain.Event{ID: "1"})
	_ = pool.Submit(ctx, domain.Event{ID: "2"})

	// Wait for workers to pick them up
	wg.Wait()

	// Stop the pool. It should block until jobs finish.
	pool.Stop()

	if count.Load() != 2 {
		t.Fatalf("expected Stop to wait for in-flight jobs, count = %d", count.Load())
	}
}
