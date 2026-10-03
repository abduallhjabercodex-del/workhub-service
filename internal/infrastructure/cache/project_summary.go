package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/circuitbreaker"
	"workhub-service/internal/infrastructure/metrics"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type ProjectSummaryCache struct {
	client  *redis.Client
	ttl     time.Duration
	logger  *slog.Logger
	sf      singleflight.Group
	breaker *circuitbreaker.Breaker
}

func NewProjectSummaryCache(client *redis.Client, ttl time.Duration, logger *slog.Logger, breaker *circuitbreaker.Breaker) *ProjectSummaryCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &ProjectSummaryCache{client: client, ttl: ttl, logger: logger, breaker: breaker}
}

func (c *ProjectSummaryCache) key(projectID string) string {
	return "workhub:project-summary:" + projectID
}

func (c *ProjectSummaryCache) Get(ctx context.Context, projectID string) (domain.ProjectSummary, bool) {
	if c.breaker != nil {
		// If circuit breaker is open (Redis unavailable), Allow() returns error
		if err := c.breaker.Allow(); err != nil {
			c.logger.Debug("cache circuit open, skipping get", "key", c.key(projectID))
			return domain.ProjectSummary{}, false // Skip Redis and fail open to primary database
		}
	}

	start := time.Now()
	value, err := c.client.Get(ctx, c.key(projectID)).Result()
	metrics.CacheOperationDuration.WithLabelValues("get").Observe(time.Since(start).Seconds())
	if errors.Is(err, redis.Nil) {
		if c.breaker != nil {
			c.breaker.Record(true)
		}
		metrics.CacheOperationsTotal.WithLabelValues("miss", "ok").Inc()
		c.logger.Info("cache miss", "key", c.key(projectID))
		return domain.ProjectSummary{}, false
	}
	if err != nil {
		if c.breaker != nil {
			c.breaker.Record(false)
		}
		metrics.CacheOperationsTotal.WithLabelValues("get", "error").Inc()
		c.logger.Warn("cache read failed", "error", err)
		return domain.ProjectSummary{}, false
	}

	if c.breaker != nil {
		c.breaker.Record(true)
	}

	var summary domain.ProjectSummary
	if err := json.Unmarshal([]byte(value), &summary); err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("get", "error").Inc()
		c.logger.Warn("cache decode failed", "error", err)
		return domain.ProjectSummary{}, false
	}
	metrics.CacheOperationsTotal.WithLabelValues("hit", "ok").Inc()
	c.logger.Info("cache hit", "key", c.key(projectID))
	return summary, true
}
func (c *ProjectSummaryCache) Set(ctx context.Context, summary domain.ProjectSummary) {
	if c.breaker != nil {
		if err := c.breaker.Allow(); err != nil {
			c.logger.Debug("cache circuit open, skipping set", "key", c.key(summary.ProjectID))
			return
		}
	}

	value, err := json.Marshal(summary)
	if err != nil {
		return
	}
	start := time.Now()
	err = c.client.Set(ctx, c.key(summary.ProjectID), value, c.ttl).Err()
	metrics.CacheOperationDuration.WithLabelValues("set").Observe(time.Since(start).Seconds())
	if err != nil {
		if c.breaker != nil {
			c.breaker.Record(false)
		}
		metrics.CacheOperationsTotal.WithLabelValues("set", "error").Inc()
		c.logger.Warn("cache write failed", "error", err)
	} else {
		metrics.CacheOperationsTotal.WithLabelValues("set", "ok").Inc()
		if c.breaker != nil {
			c.breaker.Record(true)
		}
	}
}

func (c *ProjectSummaryCache) Invalidate(ctx context.Context, projectID string) error {
	if c.breaker != nil {
		if err := c.breaker.Allow(); err != nil {
			c.logger.Debug("cache circuit open, skipping invalidate", "project_id", projectID)
			return nil
		}
	}

	start := time.Now()
	err := c.client.Del(ctx, c.key(projectID)).Err()
	metrics.CacheOperationDuration.WithLabelValues("invalidate").Observe(time.Since(start).Seconds())
	if err != nil {
		if c.breaker != nil {
			c.breaker.Record(false)
		}
		metrics.CacheOperationsTotal.WithLabelValues("invalidate", "error").Inc()
		c.logger.Warn("cache invalidation failed", "error", err)
		return err
	}

	if c.breaker != nil {
		c.breaker.Record(true)
	}
	metrics.CacheOperationsTotal.WithLabelValues("invalidate", "ok").Inc()
	c.logger.Info("cache invalidated", "project_id", projectID)
	return nil
}

// GetOrCompute prevents cache stampede (thundering herd problem).
// When cache expires and 100 concurrent requests target the same projectID,
// singleflight guarantees computeFn runs only once against PostgreSQL,
// and all concurrent callers share the resulting payload.
func (c *ProjectSummaryCache) GetOrCompute(
	ctx context.Context,
	projectID string,
	computeFn func(ctx context.Context) (domain.ProjectSummary, error),
) (domain.ProjectSummary, error) {
	if summary, ok := c.Get(ctx, projectID); ok {
		return summary, nil
	}

	// 5-second computation timeout ceiling
	computeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	val, err, _ := c.sf.Do(projectID, func() (any, error) {
		if summary, ok := c.Get(computeCtx, projectID); ok {
			return summary, nil
		}
		summary, computeErr := computeFn(computeCtx)
		if computeErr != nil {
			return domain.ProjectSummary{}, computeErr
		}
		c.Set(computeCtx, summary)
		return summary, nil
	})

	if err != nil {
		return domain.ProjectSummary{}, err
	}
	return val.(domain.ProjectSummary), nil
}
