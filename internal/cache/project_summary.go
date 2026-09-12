package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"workhub-service/internal/domain"
	"github.com/redis/go-redis/v9"
)

type ProjectSummaryCache struct {
	client *redis.Client
	ttl    time.Duration
	logger *slog.Logger
}

func NewProjectSummaryCache(client *redis.Client, ttl time.Duration, logger *slog.Logger) *ProjectSummaryCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &ProjectSummaryCache{client: client, ttl: ttl, logger: logger}
}

func (c *ProjectSummaryCache) key(projectID string) string {
	return "workhub:project-summary:" + projectID
}

func (c *ProjectSummaryCache) Get(ctx context.Context, projectID string) (domain.ProjectSummary, bool) {
	value, err := c.client.Get(ctx, c.key(projectID)).Result()
	if errors.Is(err, redis.Nil) {
		c.logger.Info("cache miss", "key", c.key(projectID))
		return domain.ProjectSummary{}, false
	}
	if err != nil {
		c.logger.Warn("cache read failed", "error", err)
		return domain.ProjectSummary{}, false
	}
	var summary domain.ProjectSummary
	if err := json.Unmarshal([]byte(value), &summary); err != nil {
		c.logger.Warn("cache decode failed", "error", err)
		return domain.ProjectSummary{}, false
	}
	c.logger.Info("cache hit", "key", c.key(projectID))
	return summary, true
}

func (c *ProjectSummaryCache) Set(ctx context.Context, summary domain.ProjectSummary) {
	value, err := json.Marshal(summary)
	if err != nil {
		return
	}
	if err := c.client.Set(ctx, c.key(summary.ProjectID), value, c.ttl).Err(); err != nil {
		c.logger.Warn("cache write failed", "error", err)
	}
}

func (c *ProjectSummaryCache) Invalidate(ctx context.Context, projectID string) {
	if err := c.client.Del(ctx, c.key(projectID)).Err(); err != nil {
		c.logger.Warn("cache invalidation failed", "error", err)
	} else {
		c.logger.Info("cache invalidated", "project_id", projectID)
	}
}
