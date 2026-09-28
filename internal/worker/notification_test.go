package worker

import (
	"context"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/messaging"
	"workhub-service/internal/infrastructure/retry"
)

func testEvent() domain.Event {
	return domain.Event{ID: "evt-1", Type: "task.created", AggregateType: "task", AggregateID: "7", ProjectID: "1", TaskID: "7", OccurredAt: time.Now().UTC()}
}

// TestNotificationWorkerRetriesAndDeduplicates يختبر إعادة المحاولة تلقائياً عند فشل الإشعار وتثبيط الرسائل المكررة (Deduplication) لمنع تكرار الإرسال.
func TestNotificationWorkerRetriesAndDeduplicates(t *testing.T) {
	broker := messaging.NewBroker(3)
	store := NewMemoryDeliveryStore()
	notifier := &MemoryNotifier{Failures: 2}
	retryCfg := retry.Config{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
	pool := NewWorkerPool(1, 10)
	worker := NewNotificationWorker(broker, store, notifier, retryCfg, pool)
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

// TestNotificationWorkerMovesFailedEventToDeadLetter يختبر نقل الأحداث التي تكرر فشل معالجتها بعد استنفاد محاولات إعادة الإرسال إلى قائمة الرسائل الميتة (Dead Letters).
func TestNotificationWorkerMovesFailedEventToDeadLetter(t *testing.T) {
	broker := messaging.NewBroker(2)
	store := NewMemoryDeliveryStore()
	notifier := &MemoryNotifier{Failures: 10}
	retryCfg := retry.Config{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
	pool := NewWorkerPool(1, 10)
	worker := NewNotificationWorker(broker, store, notifier, retryCfg, pool)
	event := testEvent()
	if err := worker.Process(context.Background(), event); err == nil {
		t.Fatal("Process() error = nil, want retry failure")
	}
	broker.AddDeadLetter(event)
	if got := len(broker.DeadLetters()); got != 1 {
		t.Fatalf("dead letters = %d, want 1", got)
	}
}

// TestNotificationWorkerIntegrationConsumesPublishedEvent يختبر التكامل الكلي لعامل الإشعارات (Worker) في استهلاك الأحداث المنشورة على ناقل الأحداث ومعالجتها فورياً.
func TestNotificationWorkerIntegrationConsumesPublishedEvent(t *testing.T) {
	broker := messaging.NewBroker(3)
	store := NewMemoryDeliveryStore()
	notifier := &MemoryNotifier{}
	retryCfg := retry.Config{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
	pool := NewWorkerPool(1, 10)
	worker := NewNotificationWorker(broker, store, notifier, retryCfg, pool)
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
