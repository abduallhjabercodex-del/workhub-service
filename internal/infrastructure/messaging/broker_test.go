package messaging

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"workhub-service/internal/domain"
)

func testEvent(id, eventType string) domain.Event {
	return domain.Event{
		ID:            id,
		Type:          eventType,
		AggregateType: "task",
		AggregateID:   "10",
		ProjectID:     "1",
		TaskID:        "10",
		OccurredAt:    time.Now().UTC(),
	}
}

// TestBrokerPublishSubscribe يختبر آلية الاشتراك والنشر الأساسية والتحقق من استلام الأحداث عبر القناة المنشأة.
func TestBrokerPublishSubscribe(t *testing.T) {
	broker := NewBroker(3)
	defer broker.Close()

	ch, unsubscribe := broker.Subscribe("task.created", 10)
	defer unsubscribe()

	event := testEvent("evt-1", "task.created")
	if err := broker.Publish(context.Background(), event); err != nil {
		t.Fatalf("Publish error = %v", err)
	}

	select {
	case rec := <-ch:
		if rec.ID != event.ID {
			t.Errorf("got event ID %s, want %s", rec.ID, event.ID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

// TestBrokerConcurrentUnsubscribeDuringPublish يختبر استقرار الناقل عند إلغاء اشتراك مشتركين بشكل متزامن أثناء إرسال الأحداث للتأكد من عدم حدوث Race Condition أو Panic.
func TestBrokerConcurrentUnsubscribeDuringPublish(t *testing.T) {
	broker := NewBroker(3)
	defer broker.Close()

	const numSubscribers = 50
	const numEvents = 100

	var unsubs []func()
	for i := 0; i < numSubscribers; i++ {
		_, unsub := broker.Subscribe("task.created", 100)
		unsubs = append(unsubs, unsub)
	}

	var wg sync.WaitGroup

	// Publisher goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numEvents; i++ {
			_ = broker.Publish(context.Background(), testEvent("evt-concurrent", "task.created"))
			time.Sleep(100 * time.Microsecond)
		}
	}()

	// Unsubscriber goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, unsub := range unsubs {
			unsub()
			time.Sleep(200 * time.Microsecond)
		}
	}()

	wg.Wait()
}

// TestBrokerSlowSubscriberDoesNotBlockFastSubscriber يختبر أن المشترك البطيء (الذي تمتلئ ذاكرة التخزين المؤقت لقناته) لا يتسبب في تعطيل أو حجب المشترك السريع عن استلام أحداثه.
func TestBrokerSlowSubscriberDoesNotBlockFastSubscriber(t *testing.T) {
	broker := NewBroker(1)
	defer broker.Close()

	// Slow subscriber with 1 buffer capacity
	_, unsubSlow := broker.Subscribe("task.updated", 1)
	defer unsubSlow()

	// Fast subscriber with large buffer
	fastCh, unsubFast := broker.Subscribe("task.updated", 10)
	defer unsubFast()

	// Fill slow subscriber buffer with first publish
	evt1 := testEvent("evt-slow-1", "task.updated")
	_ = broker.Publish(context.Background(), evt1)

	// Now publish second event; slow subscriber channel is full
	evt2 := testEvent("evt-fast-2", "task.updated")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := broker.Publish(ctx, evt2); err != nil {
		t.Logf("Publish returned err = %v (expected if slow subscriber context timed out)", err)
	}

	// Fast subscriber should have received both evt1 and evt2 despite slow subscriber buffer being full
	receivedCount := 0
	deadline := time.After(200 * time.Millisecond)
loop:
	for receivedCount < 2 {
		select {
		case <-fastCh:
			receivedCount++
		case <-deadline:
			break loop
		}
	}

	if receivedCount < 2 {
		t.Fatalf("fast subscriber received %d events, want 2 (fast subscriber was affected by slow subscriber)", receivedCount)
	}
}

// TestBrokerDeadLettersBounded يختبر حصر رسائل الخطأ في طابور الرسائل الميتة (Dead Letters Queue) بالحد الأقصى المحدد وإمكانيّة تفريغها.
func TestBrokerDeadLettersBounded(t *testing.T) {
	const maxDead = 5
	broker := NewBrokerWithOptions(3, maxDead)
	defer broker.Close()

	for i := 1; i <= 10; i++ {
		broker.AddDeadLetter(testEvent("evt-dead", "task.failed"))
	}

	dead := broker.DeadLetters()
	if len(dead) != maxDead {
		t.Fatalf("dead letters count = %d, want bounded %d", len(dead), maxDead)
	}

	broker.ClearDeadLetters()
	if len(broker.DeadLetters()) != 0 {
		t.Fatalf("dead letters count after clear = %d, want 0", len(broker.DeadLetters()))
	}
}

// TestBrokerClose يختبر إغلاق ناقل الأحداث والتأكد من إغلاق كافة قنوات المشتركين ومنع نشر أي أحداث جديدة بعد الإغلاق (إرجاع ErrClosed).
func TestBrokerClose(t *testing.T) {
	broker := NewBroker(3)

	ch, _ := broker.Subscribe("task.created", 5)
	broker.Close()

	// Re-publishing after close should return ErrClosed
	if err := broker.Publish(context.Background(), testEvent("evt-1", "task.created")); !errors.Is(err, ErrClosed) {
		t.Errorf("Publish after close = %v, want ErrClosed", err)
	}

	// Subscription channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel still open after broker close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for channel to close")
	}
}
