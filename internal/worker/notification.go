package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"workhub-service/internal/domain"
	"workhub-service/internal/messaging"
)

var ErrNotification = errors.New("notification delivery failed")

type DeliveryStore interface {
	AlreadyDelivered(context.Context, string) (bool, error)
	MarkDelivered(context.Context, string) error
}

type Notifier interface {
	NotifyTaskCreated(context.Context, domain.Event) error
}

type NotificationWorker struct {
	broker   *messaging.Broker
	store    DeliveryStore
	notify   Notifier
	maxRetry int
}

func NewNotificationWorker(broker *messaging.Broker, store DeliveryStore, notify Notifier, maxRetry int) *NotificationWorker {
	if maxRetry < 1 {
		maxRetry = 3
	}
	return &NotificationWorker{broker: broker, store: store, notify: notify, maxRetry: maxRetry}
}

func (w *NotificationWorker) Run(ctx context.Context) error {
	ch, unsubscribe := w.broker.Subscribe("task.created", 32)
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := w.Process(ctx, event); err != nil {
				w.broker.AddDeadLetter(event)
			}
		}
	}
}

func (w *NotificationWorker) Process(ctx context.Context, event domain.Event) error {
	if event.Type != "task.created" {
		return fmt.Errorf("unsupported event type: %s", event.Type)
	}
	already, err := w.store.AlreadyDelivered(ctx, event.ID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}
	var lastErr error
	for attempt := 1; attempt <= w.maxRetry; attempt++ {
		if err := w.notify.NotifyTaskCreated(ctx, event); err == nil {
			if err := w.store.MarkDelivered(ctx, event.ID); err != nil {
				return err
			}
			return nil
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("%w after %d attempts: %v", ErrNotification, w.maxRetry, lastErr)
}

type MemoryDeliveryStore struct {
	mu        sync.Mutex
	delivered map[string]bool
}

func NewMemoryDeliveryStore() *MemoryDeliveryStore {
	return &MemoryDeliveryStore{delivered: map[string]bool{}}
}

func (s *MemoryDeliveryStore) AlreadyDelivered(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delivered[id], nil
}

func (s *MemoryDeliveryStore) MarkDelivered(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivered[id] = true
	return nil
}

type LogNotifier struct{ Logger *slog.Logger }

func (n LogNotifier) NotifyTaskCreated(_ context.Context, event domain.Event) error {
	if n.Logger != nil {
		n.Logger.Info("notification delivered", "event_id", event.ID, "task_id", event.TaskID)
	}
	return nil
}

type MemoryNotifier struct {
	mu       sync.Mutex
	Events   []domain.Event
	Failures int
}

func (n *MemoryNotifier) NotifyTaskCreated(_ context.Context, event domain.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Failures > 0 {
		n.Failures--
		return ErrNotification
	}
	n.Events = append(n.Events, event)
	return nil
}

func (n *MemoryNotifier) Count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Events)
}
