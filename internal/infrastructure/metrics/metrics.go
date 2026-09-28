package metrics

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ============================================================
// HTTP Metrics
// ============================================================

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests",
		},
		[]string{"method", "route", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "workhub",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "route"},
	)

	HTTPRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "workhub",
			Name:      "http_requests_in_flight",
			Help:      "Current number of in-flight HTTP requests",
		},
		[]string{"route"},
	)
)

// ============================================================
// DB Metrics
// ============================================================

var (
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "workhub",
			Name:      "db_query_duration_seconds",
			Help:      "Database query duration in seconds",
		},
		[]string{"operation", "table"},
	)

	DBQueryErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "db_query_errors_total",
			Help:      "Database query errors",
		},
		[]string{"operation", "table", "error_type"},
	)
)

// registerGaugeOnce يضمن أن GaugeFunc واحد لكل (metric, labels)
// ويسجّل تحذيرًا بدل الفشل الصامت.
var registeredGauges sync.Map

func registerGaugeOnce(key string, opts prometheus.GaugeOpts, f func() float64) {
	if _, loaded := registeredGauges.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	g := prometheus.NewGaugeFunc(opts, f)
	if err := prometheus.Register(g); err != nil {
		var already prometheus.AlreadyRegisteredError
		if !errors.As(err, &already) {
			slog.Warn("failed to register gauge", "key", key, "error", err)
		}
	}
}

func RegisterDBPoolGauge(state string, f func() float64) {
	registerGaugeOnce("db_pool_"+state, prometheus.GaugeOpts{
		Namespace:   "workhub",
		Name:        "db_pool_connections",
		Help:        "Database connection pool state",
		ConstLabels: prometheus.Labels{"state": state},
	}, f)
}

// ============================================================
// Cache Metrics
// ============================================================

var (
	CacheOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "cache_operations_total",
			Help:      "Total cache operations (op=hit|miss|get|set|invalidate, result=ok|error)",
		},
		[]string{"op", "result"},
	)

	CacheOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "workhub",
			Name:      "cache_operation_duration_seconds",
			Help:      "Cache operation duration in seconds",
		},
		[]string{"op"},
	)

	// CacheCircuitState — Gauge بسيط (بلا labels).
	// 0=closed, 1=half_open, 2=open
	//
	// ⚠️ التنبيهات في deploy/prometheus/alerts.yml ولوحات Grafana
	// كانت تستخدم اسمًا بديلًا `workhub_cache_circuit_state_value`.
	// يجب تحديثها إلى `workhub_cache_circuit_state`.
	CacheCircuitState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "workhub",
			Name:      "cache_circuit_state",
			Help:      "Cache circuit breaker state (0=closed, 1=half_open, 2=open)",
		},
	)
)

// UpdateCacheCircuitState — يُستدعى من circuit breaker عند تغيّر الحالة.
func UpdateCacheCircuitState(state int) {
	CacheCircuitState.Set(float64(state))
}

// ============================================================
// Worker Metrics
// ============================================================

var (
	WorkerJobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "worker_jobs_total",
			Help:      "Total worker jobs processed (result=success|failed|dead_letter)",
		},
		[]string{"type", "result"},
	)

	WorkerProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "workhub",
			Name:      "worker_processing_duration_seconds",
			Help:      "Worker job processing duration in seconds",
		},
		[]string{"type"},
	)

	WorkerRetriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "worker_retries_total",
			Help:      "Total worker job retries",
		},
		[]string{"type"},
	)
)

func RegisterWorkerQueueDepthGauge(queue string, f func() float64) {
	registerGaugeOnce("worker_queue_"+queue, prometheus.GaugeOpts{
		Namespace:   "workhub",
		Name:        "worker_queue_depth",
		Help:        "Current depth of worker queue",
		ConstLabels: prometheus.Labels{"queue": queue},
	}, f)
}

// ============================================================
// Rate Limiting / Load Shedding
// ============================================================

var (
	// RateLimitRejectedTotal — scope values: "auth", "projects", "global", "user", "ip"
	RateLimitRejectedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "rate_limit_rejected_total",
			Help:      "Total requests rejected by rate limiting",
		},
		[]string{"scope"},
	)

	LoadShedRejectedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "load_shed_rejected_total",
			Help:      "Total requests rejected by load shedding",
		},
	)
)

// ============================================================
// Idempotency
// ============================================================

var (
	IdempotencyRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "idempotency_requests_total",
			Help:      "Total idempotency requests (result=new|replayed|error)",
		},
		[]string{"result"},
	)
)

// ============================================================
// Bulkhead
// ============================================================

var (
	BulkheadInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "workhub",
			Name:      "bulkhead_in_flight",
			Help:      "Current number of in-flight requests in bulkhead",
		},
		[]string{"name"},
	)

	BulkheadRejectedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "workhub",
			Name:      "bulkhead_rejected_total",
			Help:      "Total requests rejected by bulkhead",
		},
		[]string{"name"},
	)
)

// ============================================================
// Helper
// ============================================================

// ObserveDBQuery — يبدأ مؤقتًا ويعيد دالة defer.
//
// الاستخدام:
//
//	stop := metrics.ObserveDBQuery("SELECT", "projects")
//	row, err := r.pool.Query(ctx, ...)
//	stop(&err)
//
// ملاحظة: التوقيع يستخدم *error لكي يقرأ القيمة النهائية بعد تنفيذ الاستعلام.
func ObserveDBQuery(operation, table string) func(err *error) {
	start := time.Now()
	return func(err *error) {
		DBQueryDuration.WithLabelValues(operation, table).Observe(time.Since(start).Seconds())
		if err != nil && *err != nil {
			DBQueryErrorsTotal.WithLabelValues(operation, table, "error").Inc()
		}
	}
}
