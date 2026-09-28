package messaging

import (
	"context"
	"testing"
	"time"

	"workhub-service/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisBroker_PublishAndSubscribe(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	broker := NewRedisBroker(client)
	defer broker.Close()

	ch, unsub := broker.Subscribe("task.created", 10)
	defer unsub()

	event := domain.Event{
		ID:         "evt-101",
		Type:       "task.created",
		TaskID:     "555",
		OccurredAt: time.Now().UTC(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Give redis pubsub subscription a moment to settle in miniredis
	time.Sleep(50 * time.Millisecond)

	if err := broker.Publish(ctx, event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case received := <-ch:
		if received.ID != event.ID || received.TaskID != event.TaskID {
			t.Fatalf("received mismatch: got %+v, want %+v", received, event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event on redis pubsub")
	}
}

func TestRedisBroker_DeadLetter(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	broker := NewRedisBroker(client)
	defer broker.Close()

	event := domain.Event{
		ID:         "evt-dead-1",
		Type:       "task.created",
		TaskID:     "999",
		OccurredAt: time.Now().UTC(),
	}

	broker.AddDeadLetter(event)

	ctx := context.Background()
	dead, err := broker.DeadLetters(ctx)
	if err != nil {
		t.Fatalf("failed to get dead letters: %v", err)
	}
	if len(dead) != 1 {
		t.Fatalf("expected 1 dead letter, got %d", len(dead))
	}
	if dead[0].ID != "evt-dead-1" {
		t.Fatalf("expected dead letter ID evt-dead-1, got %s", dead[0].ID)
	}
}
