// Package retry provides retry with exponential backoff and jitter.
package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// Config holds the configuration for retry behavior.
type Config struct {
	MaxAttempts  int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	JitterFactor float64 // [0, 1]
}

// DefaultConfig returns a reasonable default configuration.
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  5,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		JitterFactor: 0.3,
	}
}

// Validate checks the configuration.
func (c Config) Validate() error {
	if c.MaxAttempts < 1 {
		return errors.New("retry: MaxAttempts must be >= 1")
	}
	if c.BaseDelay <= 0 {
		return errors.New("retry: BaseDelay must be > 0")
	}
	if c.MaxDelay < c.BaseDelay {
		return errors.New("retry: MaxDelay must be >= BaseDelay")
	}
	if c.JitterFactor < 0 || c.JitterFactor > 1 {
		return errors.New("retry: JitterFactor must be in [0, 1]")
	}
	return nil
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as non-retriable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err is permanent.
func IsPermanent(err error) bool {
	var pErr *permanentError
	return errors.As(err, &pErr)
}

// OnRetry is called before each retry sleep.
type OnRetry func(attempt int, err error, delay time.Duration)

// Do executes fn with retries according to cfg.
func Do(ctx context.Context, cfg Config, fn func(context.Context) error) error {
	return DoWithCallback(ctx, cfg, fn, nil)
}

// DoWithCallback is like Do, but calls onRetry before each retry.
func DoWithCallback(ctx context.Context, cfg Config, fn func(context.Context) error, onRetry OnRetry) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		if IsPermanent(err) {
			return err
		}
		lastErr = err

		if attempt == cfg.MaxAttempts {
			break
		}

		delay := computeDelay(cfg, attempt)
		if onRetry != nil {
			onRetry(attempt, err, delay)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// computeDelay calculates exponential backoff with jitter.
// Caps at MaxDelay without overflow.
func computeDelay(cfg Config, attempt int) time.Duration {
	d := cfg.BaseDelay // Start with base delay (e.g. 100ms)

	// 1. Exponential Backoff
	for i := 1; i < attempt; i++ {
		d *= 2 // Double backoff on each attempt (e.g. 100ms -> 200ms -> 400ms)
	}
	// Cap delay at MaxDelay ceiling
	if d > cfg.MaxDelay {
		d = cfg.MaxDelay
	}

	// 2. Jitter
	if cfg.JitterFactor > 0 {
		// Generate random float between -1 and +1 scaled by jitter factor (+/- 30%)
		jitter := (rand.Float64()*2 - 1) * cfg.JitterFactor

		// Apply randomized multiplier to delay
		d = time.Duration(float64(d) * (1 + jitter))
	}

	return d
}
