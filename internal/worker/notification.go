package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/messaging"
	"workhub-service/internal/infrastructure/metrics"
	"workhub-service/internal/infrastructure/retry"

	"github.com/redis/go-redis/v9"
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
	broker   messaging.EventBroker
	store    DeliveryStore
	notify   Notifier
	retryCfg retry.Config
	pool     *WorkerPool
}

func NewNotificationWorker(broker messaging.EventBroker, store DeliveryStore, notify Notifier, retryCfg retry.Config, pool *WorkerPool) *NotificationWorker {
	if retryCfg.MaxAttempts == 0 {
		retryCfg = retry.DefaultConfig()
	}
	return &NotificationWorker{broker: broker, store: store, notify: notify, retryCfg: retryCfg, pool: pool}
}
func (w *NotificationWorker) Run(ctx context.Context) error {
	w.pool.Start(ctx, w.Process)
	defer w.pool.Stop()

	ch, unsubscribe := w.broker.Subscribe("task.created", 32)
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-ch:
			if !ok { // ok == false يعني أن Channel أغلقت.
				return nil
			}
			if err := w.pool.Submit(ctx, event); err != nil {
				slog.ErrorContext(ctx, "failed to submit to pool; moving to dead letter",
					"event_id", event.ID,
					"event_type", event.Type,
					"task_id", event.TaskID,
					"error", err,
				)
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

	attempt := 0
	err = retry.Do(ctx, w.retryCfg, func(ctx context.Context) error {
		attempt++
		if attempt > 1 {
			metrics.WorkerRetriesTotal.WithLabelValues(event.Type).Inc()
		}
		err := w.notify.NotifyTaskCreated(ctx, event)
		if err == nil {
			if err := w.store.MarkDelivered(ctx, event.ID); err != nil {
				return err
			}
			return nil
		}

		// Mark 4xx-equivalent errors as permanent.
		if errors.Is(err, domain.ErrNotFound) ||
			errors.Is(err, domain.ErrAlreadyExists) ||
			errors.Is(err, domain.ErrQuotaExceeded) {
			return retry.Permanent(err)
		}

		return err
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotification, err)
	}
	return nil
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

type RedisDeliveryStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisDeliveryStore(client *redis.Client, ttl time.Duration) *RedisDeliveryStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &RedisDeliveryStore{client: client, ttl: ttl}
}

func (s *RedisDeliveryStore) AlreadyDelivered(ctx context.Context, id string) (bool, error) {
	key := "workhub:delivered:" + id
	n, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *RedisDeliveryStore) MarkDelivered(ctx context.Context, id string) error {
	key := "workhub:delivered:" + id
	return s.client.Set(ctx, key, "1", s.ttl).Err()
}

type LogNotifier struct{ Logger *slog.Logger }

func (n LogNotifier) NotifyTaskCreated(_ context.Context, event domain.Event) error {
	if n.Logger != nil {
		n.Logger.Info("notification delivered", "event_id", event.ID, "task_id", event.TaskID)
	}
	return nil
}

type MemoryNotifier struct { //صممناه لمساعدتنا في الاختبارات.
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
