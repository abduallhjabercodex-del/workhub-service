package repository

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"workhub-service/internal/infrastructure/bulkhead"
	"workhub-service/internal/infrastructure/metrics"
)

const defaultListTasksBulkheadSize = 10

type PostgresRepository struct {
	pool             *pgxpool.Pool
	listTaskBulkhead *bulkhead.Bulkhead
}

// poolGaugeOnce — يضمن تسجيل gauges مرة واحدة فقط لكل pool.
var poolGaugeOnce sync.Once

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	if pool == nil {
		panic("repository: nil pool passed to NewPostgresRepository")
	}

	poolGaugeOnce.Do(func() {
		metrics.RegisterDBPoolGauge("idle", func() float64 {
			return float64(pool.Stat().IdleConns())
		})
		metrics.RegisterDBPoolGauge("acquired", func() float64 {
			return float64(pool.Stat().AcquiredConns())
		})
		metrics.RegisterDBPoolGauge("max", func() float64 {
			return float64(pool.Stat().MaxConns())
		})
	})

	return &PostgresRepository{
		pool:             pool,
		listTaskBulkhead: bulkhead.New("list_tasks", defaultListTasksBulkheadSize),
	}
}

func (r *PostgresRepository) Close() { r.pool.Close() }

func (r *PostgresRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
