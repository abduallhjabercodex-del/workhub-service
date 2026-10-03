package worker_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/repository"
	"workhub-service/internal/worker"
)

type mockBroker struct {
	mu        sync.Mutex
	events    []domain.Event
	failUntil int
}

func (m *mockBroker) Publish(_ context.Context, e domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failUntil > 0 {
		m.failUntil--
		return errors.New("temporary broker failure")
	}
	m.events = append(m.events, e)
	return nil
}

func (m *mockBroker) Subscribe(string, int) (<-chan domain.Event, func()) {
	ch := make(chan domain.Event)
	return ch, func() { close(ch) }
}
func (m *mockBroker) AddDeadLetter(domain.Event) {}
func (m *mockBroker) Close()                    {}

func TestOutboxProcessor_SuccessFlow(t *testing.T) {
	memRepo := repository.NewMemoryRepository()
	broker := &mockBroker{}

	// Create a project & task in memory repo to generate an outbox event
	p, err := memRepo.CreateProject(context.Background(), domain.Project{Name: "P1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	task, err := memRepo.CreateTaskWithQuota(context.Background(), domain.Task{
		ProjectID: p.ID,
		Title:     "Deploy Outbox",
		Status:    "todo",
	}, domain.AuditEvent{}, 10)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Verify an unpublished outbox record exists
	records, err := memRepo.FetchUnpublishedOutboxEvents(context.Background(), 10)
	if err != nil {
		t.Fatalf("fetch unpublished: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 unpublished outbox event, got %d", len(records))
	}

	processor := worker.NewOutboxProcessor(
		memRepo,
		broker,
		slog.Default(),
		worker.OutboxProcessorConfig{
			PollInterval:  50 * time.Millisecond,
			BatchSize:     10,
			MaxRetryDelay: 1 * time.Second,
		},
	)

	// Process one batch
	processed, err := processor.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("process batch: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed, got %d", processed)
	}

	// Verify broker received the event
	broker.mu.Lock()
	if len(broker.events) != 1 {
		t.Fatalf("expected broker to receive 1 event, got %d", len(broker.events))
	}
	received := broker.events[0]
	broker.mu.Unlock()

	if received.Type != "task.created" {
		t.Errorf("expected event type 'task.created', got %s", received.Type)
	}
	if received.TaskID != task.ID {
		t.Errorf("expected taskID %s, got %s", task.ID, received.TaskID)
	}

	// Verify outbox record is now marked as published
	recordsAfter, err := memRepo.FetchUnpublishedOutboxEvents(context.Background(), 10)
	if err != nil {
		t.Fatalf("fetch unpublished after: %v", err)
	}
	if len(recordsAfter) != 0 {
		t.Fatalf("expected 0 unpublished outbox events, got %d", len(recordsAfter))
	}
}

func TestOutboxProcessor_BrokerFailureBackoff(t *testing.T) {
	memRepo := repository.NewMemoryRepository()
	broker := &mockBroker{failUntil: 2} // will fail 2 times

	p, _ := memRepo.CreateProject(context.Background(), domain.Project{Name: "P2"})
	_, err := memRepo.CreateTaskWithQuota(context.Background(), domain.Task{
		ProjectID: p.ID,
		Title:     "Failing Task",
		Status:    "todo",
	}, domain.AuditEvent{}, 10)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	processor := worker.NewOutboxProcessor(
		memRepo,
		broker,
		slog.Default(),
		worker.OutboxProcessorConfig{
			PollInterval:  20 * time.Millisecond,
			BatchSize:     10,
			MaxRetryDelay: 50 * time.Millisecond,
		},
	)

	// First attempt: should fail
	_, err = processor.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("process batch should not return err for item failure: %v", err)
	}

	// Immediately fetching should return 0 because availableAt is in future backoff
	pendingImmediate, _ := memRepo.FetchUnpublishedOutboxEvents(context.Background(), 10)
	if len(pendingImmediate) != 0 {
		t.Fatalf("expected backoff delay to hide item, but got %d", len(pendingImmediate))
	}

	// Wait for backoff to pass (1s for first retry)
	// We can manually reset availableAt in test or run with short delay
	time.Sleep(1100 * time.Millisecond)

	pendingAfterWait, _ := memRepo.FetchUnpublishedOutboxEvents(context.Background(), 10)
	if len(pendingAfterWait) != 1 {
		t.Fatalf("expected event to become available after backoff, got %d", len(pendingAfterWait))
	}
}

func TestOutboxProcessor_GracefulShutdown(t *testing.T) {
	memRepo := repository.NewMemoryRepository()
	broker := &mockBroker{}

	processor := worker.NewOutboxProcessor(
		memRepo,
		broker,
		slog.Default(),
		worker.OutboxProcessorConfig{
			PollInterval: 10 * time.Millisecond,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- processor.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processor.Run timed out on shutdown")
	}
}
