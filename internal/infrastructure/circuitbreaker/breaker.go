// Package circuitbreaker provides a simple state machine for fail-fast behavior.
package circuitbreaker

import (
	"errors"
	"sync"
	"time"

	"workhub-service/internal/infrastructure/metrics"
)

var ErrOpen = errors.New("circuit breaker is open")

// State represents the current state of the circuit breaker.
type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half-open"
)

// Config holds circuit breaker parameters.
type Config struct {
	// Threshold is the number of consecutive failures before tripping to Open.
	// default: 3
	Threshold int

	// Cooldown is the duration to remain in Open before attempting HalfOpen.
	// default: 10s
	Cooldown time.Duration

	// HalfOpenMax is the number of consecutive successes to close from HalfOpen.
	// default: 3
	HalfOpenMax int

	// Name is a label for telemetry and logs. If empty -> "default".
	Name string

	// DisableTimer disables automatic cooldown scheduling.
	// Useful for deterministic testing where time is advanced manually.
	DisableTimer bool
}

// Breaker implements the circuit breaker pattern.
//
// State transitions:
//
//	Closed   --[Threshold failures]--> Open
//	Open     --[Cooldown elapsed]----> HalfOpen
//	HalfOpen --[HalfOpenMax successes]--> Closed
//	HalfOpen --[1 failure]-----------> Open
type Breaker struct {
	mu          sync.RWMutex
	threshold   int
	cooldown    time.Duration
	halfOpenMax int
	name        string
	state       State
	failures    int
	successes   int
	lastFailure time.Time

	// timeNow can be substituted in tests for deterministic time.
	timeNow func() time.Time

	// timer for automatic cooldown scheduling (may be nil).
	timer *time.Timer

	// disableTimer disables automatic timer scheduling (for tests).
	disableTimer bool

	// closed prevents scheduling new timers after Close().
	closed bool
}

// New creates a new circuit breaker with default settings.
func New(threshold int, cooldown time.Duration) *Breaker {
	return NewWithConfig(Config{
		Threshold: threshold,
		Cooldown:  cooldown,
	})
}

// NewWithConfig creates a new circuit breaker with full configuration.
func NewWithConfig(cfg Config) *Breaker {
	if cfg.Threshold < 1 {
		cfg.Threshold = 3
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 10 * time.Second
	}
	if cfg.HalfOpenMax < 1 {
		cfg.HalfOpenMax = 3
	}
	if cfg.Name == "" {
		cfg.Name = "default"
	}

	b := &Breaker{
		threshold:    cfg.Threshold,
		cooldown:     cfg.Cooldown,
		halfOpenMax:  cfg.HalfOpenMax,
		name:         cfg.Name,
		state:        StateClosed,
		timeNow:      time.Now,
		disableTimer: cfg.DisableTimer,
	}
	b.setMetric(0) // Closed
	return b
}

// State returns the current state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkStateLocked()
	return b.state
}

// Allow returns ErrOpen if the circuit is open.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkStateLocked()
	if b.state == StateOpen {
		return ErrOpen
	}
	return nil
}

// Record observes the result of an operation.
func (b *Breaker) Record(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.checkStateLocked()

	if success {
		b.recordSuccessLocked()
	} else {
		b.recordFailureLocked()
	}
	b.setMetricLocked()
}

func (b *Breaker) recordSuccessLocked() {
	switch b.state {
	case StateHalfOpen:
		b.successes++
		if b.successes >= b.halfOpenMax {
			b.state = StateClosed
			b.failures = 0
			b.successes = 0
			b.stopTimerLocked()
		}
	case StateClosed:
		b.failures = 0
	}
}

func (b *Breaker) recordFailureLocked() {
	switch b.state {
	case StateHalfOpen:
		b.openLocked()
	case StateClosed:
		b.failures++
		if b.failures >= b.threshold {
			b.openLocked()
		}
	}
}

func (b *Breaker) openLocked() {
	b.state = StateOpen
	b.lastFailure = b.timeNow()
	b.successes = 0
	b.scheduleCooldownTimerLocked()
}

func (b *Breaker) scheduleCooldownTimerLocked() {
	if b.disableTimer || b.closed {
		return
	}
	// Stop any existing timer before scheduling a new one.
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.timer = time.AfterFunc(b.cooldown, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.closed {
			return
		}
		b.checkStateLocked()
	})
}

func (b *Breaker) stopTimerLocked() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
}

// checkStateLocked must always be called while holding mu.Lock.
func (b *Breaker) checkStateLocked() {
	if b.state != StateOpen {
		return
	}
	if b.timeNow().Sub(b.lastFailure) >= b.cooldown {
		b.state = StateHalfOpen
		b.successes = 0
		b.stopTimerLocked()
		b.setMetricLocked()
	}
}

// setMetric is used internally by New (lock-free since instance is unshared).
func (b *Breaker) setMetric(state int) {
	metrics.UpdateCacheCircuitState(state)
}

// setMetricLocked updates Prometheus metric while holding mu.Lock.
func (b *Breaker) setMetricLocked() {
	var v int
	switch b.state {
	case StateClosed:
		v = 0
	case StateHalfOpen:
		v = 1
	case StateOpen:
		v = 2
	}
	metrics.UpdateCacheCircuitState(v)
}

// Name returns the breaker's name.
func (b *Breaker) Name() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.name
}

// Close cancels active timers and releases resources. Safe for multiple invocations.
func (b *Breaker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.stopTimerLocked()
}
