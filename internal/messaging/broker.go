package messaging

import (
	"context"
	"errors"
	"sync"
	"time"

	"workhub-service/internal/domain"
)

var ErrClosed = errors.New("broker closed")

const (
	DefaultMaxRetry = 3
	DefaultMaxDead  = 1000
)

type subscription struct {
	mu     sync.Mutex
	ch     chan domain.Event
	closed bool
}

func (s *subscription) send(event domain.Event, ctx context.Context, maxRetry int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrClosed
	}

	for attempt := 0; attempt < maxRetry; attempt++ {
		select {
		case s.ch <- event:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			if attempt < maxRetry-1 {
				// Yield lock briefly to allow subscriber to consume if buffer is full
				s.mu.Unlock()
				select {
				case <-ctx.Done():
					s.mu.Lock()
					return ctx.Err()
				case <-time.After(1 * time.Millisecond):
				}
				s.mu.Lock()
				if s.closed {
					return ErrClosed
				}
			}
		}
	}

	select {
	case s.ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *subscription) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

type Broker struct {
	mu       sync.RWMutex
	closed   bool
	subs     map[string][]*subscription
	dead     []domain.Event
	maxRetry int
	maxDead  int
}

func NewBroker(maxRetry int) *Broker {
	return NewBrokerWithOptions(maxRetry, DefaultMaxDead)
}

func NewBrokerWithOptions(maxRetry, maxDead int) *Broker {
	if maxRetry < 1 {
		maxRetry = DefaultMaxRetry
	}
	if maxDead < 1 {
		maxDead = DefaultMaxDead
	}
	return &Broker{
		subs:     make(map[string][]*subscription),
		dead:     make([]domain.Event, 0),
		maxRetry: maxRetry,
		maxDead:  maxDead,
	}
}

func (b *Broker) MaxRetry() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.maxRetry
}

func (b *Broker) Subscribe(eventType string, buffer int) (<-chan domain.Event, func()) {
	if buffer < 1 {
		buffer = 16
	}
	sub := &subscription{
		ch: make(chan domain.Event, buffer),
	}

	b.mu.Lock()
	if b.closed {
		sub.close()
		b.mu.Unlock()
		return sub.ch, func() {}
	}
	b.subs[eventType] = append(b.subs[eventType], sub)
	b.mu.Unlock()

	var unsubscribeOnce sync.Once
	unsubscribe := func() {
		unsubscribeOnce.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			subs, ok := b.subs[eventType]
			if !ok {
				sub.close()
				return
			}
			for i, candidate := range subs {
				if candidate == sub {
					b.subs[eventType] = append(subs[:i], subs[i+1:]...)
					if len(b.subs[eventType]) == 0 {
						delete(b.subs, eventType)
					}
					break
				}
			}
			sub.close()
		})
	}

	return sub.ch, unsubscribe
}

func (b *Broker) Publish(ctx context.Context, event domain.Event) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrClosed
	}
	subsCopy := append([]*subscription(nil), b.subs[event.Type]...)
	maxRetry := b.maxRetry
	b.mu.RUnlock()

	if len(subsCopy) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(subsCopy))

	for _, sub := range subsCopy {
		wg.Add(1)
		go func(s *subscription) {
			defer wg.Done()
			if err := s.send(event, ctx, maxRetry); err != nil && !errors.Is(err, ErrClosed) {
				errCh <- err
			}
		}(sub)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *Broker) AddDeadLetter(event domain.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.dead) >= b.maxDead {
		b.dead = b.dead[1:]
	}
	b.dead = append(b.dead, event)
}

func (b *Broker) DeadLetters() []domain.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]domain.Event(nil), b.dead...)
}

func (b *Broker) ClearDeadLetters() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dead = make([]domain.Event, 0)
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for eventType, subs := range b.subs {
		for _, sub := range subs {
			sub.close()
		}
		delete(b.subs, eventType)
	}
}
