package repository

import (
	"context"
	"fmt"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/metrics"
)

// FetchUnpublishedOutboxEvents fetches pending outbox events using FOR UPDATE SKIP LOCKED
// to prevent concurrent workers from claiming the same events.
func (r *PostgresRepository) FetchUnpublishedOutboxEvents(ctx context.Context, batchSize int) ([]domain.OutboxRecord, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	stop := metrics.ObserveDBQuery("SELECT", "outbox_events")
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_id, event_type, aggregate_type, aggregate_id, payload,
		       attempts, available_at, published_at, last_error, created_at
		FROM outbox_events
		WHERE published_at IS NULL
		  AND available_at <= NOW()
		ORDER BY available_at ASC, id ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, batchSize)
	stop(&err)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	var records []domain.OutboxRecord
	for rows.Next() {
		var rec domain.OutboxRecord
		var aggID int64
		if err := rows.Scan(
			&rec.ID,
			&rec.EventID,
			&rec.EventType,
			&rec.AggregateType,
			&aggID,
			&rec.Payload,
			&rec.Attempts,
			&rec.AvailableAt,
			&rec.PublishedAt,
			&rec.LastError,
			&rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan outbox record: %w", err)
		}
		rec.AggregateID = fmt.Sprintf("%d", aggID)
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, mapDBError(err)
	}
	return records, nil
}

// MarkOutboxEventPublished marks an outbox record as successfully published.
func (r *PostgresRepository) MarkOutboxEventPublished(ctx context.Context, id int64) error {
	stop := metrics.ObserveDBQuery("UPDATE", "outbox_events")
	_, err := r.pool.Exec(ctx, `
		UPDATE outbox_events
		SET published_at = NOW(),
		    last_error = NULL
		WHERE id = $1`, id)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	return nil
}

// MarkOutboxEventFailed updates failure metadata and sets exponential backoff on available_at.
func (r *PostgresRepository) MarkOutboxEventFailed(ctx context.Context, id int64, lastErr string, nextRetry time.Duration) error {
	nextAvailableAt := time.Now().UTC().Add(nextRetry)
	stop := metrics.ObserveDBQuery("UPDATE", "outbox_events")
	_, err := r.pool.Exec(ctx, `
		UPDATE outbox_events
		SET attempts = attempts + 1,
		    last_error = $2,
		    available_at = $3
		WHERE id = $1`, id, lastErr, nextAvailableAt)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	return nil
}
