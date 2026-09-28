package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"workhub-service/internal/domain"

	"github.com/redis/go-redis/v9"
)

const (
	RedisChannelPrefix = "workhub:events:"
	RedisDeadLetterKey = "workhub:events:dead_letters"
)

// RedisBroker implements EventBroker using Redis Pub/Sub for distributed deployments.
type RedisBroker struct {
	client  *redis.Client
	deadKey string
	maxDead int64
	mu      sync.Mutex
	closed  bool
	pubsubs []*redis.PubSub
}

func NewRedisBroker(client *redis.Client) *RedisBroker {
	return &RedisBroker{
		client:  client,
		deadKey: RedisDeadLetterKey,
		maxDead: DefaultMaxDead,
		pubsubs: make([]*redis.PubSub, 0),
	}
}

func (b *RedisBroker) Publish(ctx context.Context, event domain.Event) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	b.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	channel := RedisChannelPrefix + event.Type
	if err := b.client.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("publish to redis: %w", err)
	}
	return nil
}

func (b *RedisBroker) Subscribe(eventType string, buffer int) (<-chan domain.Event, func()) {
	if buffer < 1 {
		buffer = 16
	}
	ch := make(chan domain.Event, buffer)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}

	channel := RedisChannelPrefix + eventType
	ctx, cancel := context.WithCancel(context.Background())
	pubsub := b.client.Subscribe(ctx, channel)
	b.pubsubs = append(b.pubsubs, pubsub)
	b.mu.Unlock()

	// Worker goroutine to read from Redis pubsub channel and emit to Go channel
	go func() {
		defer close(ch)
		redisCh := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-redisCh:
				if !ok {
					return
				}
				var event domain.Event
				if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
					slog.Error("failed to unmarshal redis pubsub message", "channel", channel, "error", err)
					continue
				}
				select {
				case ch <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	var unsubscribeOnce sync.Once
	unsubscribe := func() {
		unsubscribeOnce.Do(func() {
			cancel()
			_ = pubsub.Close()

			b.mu.Lock()
			defer b.mu.Unlock()
			for i, p := range b.pubsubs {
				if p == pubsub {
					b.pubsubs = append(b.pubsubs, b.pubsubs[i+1:]...)
					break
				}
			}
		})
	}

	return ch, unsubscribe
}

func (b *RedisBroker) AddDeadLetter(event domain.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal dead letter event", "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := b.client.Pipeline()
	pipe.RPush(ctx, b.deadKey, data)
	pipe.LTrim(ctx, b.deadKey, -b.maxDead, -1)
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("failed to push to dead letter queue in redis", "error", err)
	}
}

func (b *RedisBroker) DeadLetters(ctx context.Context) ([]domain.Event, error) {
	items, err := b.client.LRange(ctx, b.deadKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("read dead letters: %w", err)
	}
	events := make([]domain.Event, 0, len(items))
	for _, item := range items {
		var ev domain.Event
		if err := json.Unmarshal([]byte(item), &ev); err == nil {
			events = append(events, ev)
		}
	}
	return events, nil
}

func (b *RedisBroker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, pubsub := range b.pubsubs {
		_ = pubsub.Close()
	}
	b.pubsubs = nil
}
