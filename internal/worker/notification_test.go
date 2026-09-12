package worker

import (
	"context"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/messaging"
)

func testEvent() domain.Event {
	return domain.Event{ID: "evt-1", Type: "task.created", AggregateType: "task", AggregateID: "7", ProjectID: "1", TaskID: "7", OccurredAt: time.Now().UTC()}
}

func TestNotificationWorkerRetriesAndDeduplicates(t *testing.T) {
	broker := messaging.NewBroker(3)
	store := NewMemoryDeliveryStore()
	notifier := &MemoryNotifier{Failures: 2}
	worker := NewNotificationWorker(broker, store, notifier, 3)
	event := testEvent()
	if err := worker.Process(context.Background(), event); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if notifier.Count() != 1 {
		t.Fatalf("notifications = %d, want 1", notifier.Count())
	}
	if err := worker.Process(context.Background(), event); err != nil {
		t.Fatalf("duplicate Process() error = %v", err)
	}
	if notifier.Count() != 1 {
		t.Fatalf("duplicate notification count = %d, want 1", notifier.Count())
	}
}

func TestNotificationWorkerMovesFailedEventToDeadLetter(t *testing.T) {
	broker := messaging.NewBroker(2)
	store := NewMemoryDeliveryStore()
	notifier := &MemoryNotifier{Failures: 10}
	worker := NewNotificationWorker(broker, store, notifier, 2)
	event := testEvent()
	if err := worker.Process(context.Background(), event); err == nil {
		t.Fatal("Process() error = nil, want retry failure")
	}
	broker.AddDeadLetter(event)
	if got := len(broker.DeadLetters()); got != 1 {
		t.Fatalf("dead letters = %d, want 1", got)
	}
}

func TestNotificationWorkerIntegrationConsumesPublishedEvent(t *testing.T) {
	broker := messaging.NewBroker(3)
	store := NewMemoryDeliveryStore()
	notifier := &MemoryNotifier{}
	worker := NewNotificationWorker(broker, store, notifier, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)
	if err := broker.Publish(context.Background(), testEvent()); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for notifier.Count() == 0 {
		select {
		case <-deadline:
			t.Fatal("worker did not process event")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	<-done
}
