package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/messaging"
)

// OutboxProcessorConfig configures the polling and retry behavior for the Outbox Relay.
type OutboxProcessorConfig struct {
	PollInterval  time.Duration
	BatchSize     int
	MaxRetryDelay time.Duration
}

// DefaultOutboxConfig provides production-ready defaults.
func DefaultOutboxConfig() OutboxProcessorConfig {
	return OutboxProcessorConfig{
		PollInterval:  500 * time.Millisecond,
		BatchSize:     50,
		MaxRetryDelay: 1 * time.Minute,
	}
}

// OutboxProcessor periodically reads unpublished events from the database outbox
// table and relays them to the EventBroker (Redis or In-Memory), guaranteeing At-Least-Once delivery.
type OutboxProcessor struct {
	repo      domain.OutboxRepository
	publisher messaging.EventBroker
	logger    *slog.Logger
	cfg       OutboxProcessorConfig
}

// NewOutboxProcessor initializes an OutboxProcessor instance.
func NewOutboxProcessor(
	repo domain.OutboxRepository,
	publisher messaging.EventBroker,
	logger *slog.Logger,
	cfg OutboxProcessorConfig,
) *OutboxProcessor {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxRetryDelay <= 0 {
		cfg.MaxRetryDelay = 1 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboxProcessor{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
		cfg:       cfg,
	}
}

// ProcessBatch fetches and publishes a single batch of unpublished outbox records.
func (p *OutboxProcessor) ProcessBatch(ctx context.Context) (int, error) {
	records, err := p.repo.FetchUnpublishedOutboxEvents(ctx, p.cfg.BatchSize)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}

	publishedCount := 0
	for _, record := range records {
		event := domain.Event{
			ID:            record.EventID,
			Type:          record.EventType,
			AggregateType: record.AggregateType,
			AggregateID:   record.AggregateID,
			Payload:       record.Payload,
			OccurredAt:    record.CreatedAt,
		}

		// Extract metadata if stored in payload
		var meta struct {
			ProjectID     string `json:"projectId"`
			TaskID        string `json:"taskId"`
			CorrelationID string `json:"correlationId"`
		}
		if err := json.Unmarshal(record.Payload, &meta); err == nil {
			if meta.ProjectID != "" {
				event.ProjectID = meta.ProjectID
			}
			if meta.TaskID != "" {
				event.TaskID = meta.TaskID
			}
			if meta.CorrelationID != "" {
				event.CorrelationID = meta.CorrelationID
			}
		}

		if err := p.publisher.Publish(ctx, event); err != nil {
			p.logger.ErrorContext(ctx, "failed to publish outbox event to broker",
				"event_id", record.EventID,
				"event_type", record.EventType,
				"attempts", record.Attempts+1,
				"error", err,
			)

			// Exponential backoff with ceiling
			backoff := time.Duration(1<<uint(record.Attempts)) * time.Second
			if backoff > p.cfg.MaxRetryDelay {
				backoff = p.cfg.MaxRetryDelay
			}
			_ = p.repo.MarkOutboxEventFailed(ctx, record.ID, err.Error(), backoff)
			continue
		}

		if err := p.repo.MarkOutboxEventPublished(ctx, record.ID); err != nil {
			p.logger.ErrorContext(ctx, "failed to mark outbox event as published",
				"event_id", record.EventID,
				"error", err,
			)
			continue
		}

		publishedCount++
		p.logger.DebugContext(ctx, "outbox event published and marked successfully",
			"event_id", record.EventID,
			"event_type", record.EventType,
		)
	}

	return len(records), nil
}

// Run begins the polling loop until the context is cancelled.
func (p *OutboxProcessor) Run(ctx context.Context) error {
	p.logger.InfoContext(ctx, "outbox relay processor started",
		"poll_interval", p.cfg.PollInterval,
		"batch_size", p.cfg.BatchSize,
	)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.InfoContext(ctx, "outbox relay processor shutting down")
			return ctx.Err()
		case <-ticker.C:
			for {
				processed, err := p.ProcessBatch(ctx)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return err
					}
					p.logger.WarnContext(ctx, "outbox batch processing error", "error", err)
					break
				}
				// Drain all backlogged records immediately before waiting for the next ticker tick
				if processed < p.cfg.BatchSize {
					break
				}
			}
		}
	}
}
