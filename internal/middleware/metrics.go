package middleware

import (
	"net/http"
	"strconv"
	"time"

	"workhub-service/internal/infrastructure/metrics"
)

type metricsRecorder struct {
	http.ResponseWriter
	status int
}

func (r *metricsRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *metricsRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Metrics records HTTP metrics for the request.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/metrics" || path == "/health" || path == "/ready" || path == "/startup" || path == "/healthz" || path == "/readyz" || path == "/startupz" {
			next.ServeHTTP(w, r)
			return
		}

		route := r.Pattern
		if route == "" {
			route = "unknown"
		}

		metrics.HTTPRequestsInFlight.WithLabelValues(route).Inc()
		defer metrics.HTTPRequestsInFlight.WithLabelValues(route).Dec()

		start := time.Now()
		rec := &metricsRecorder{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		statusStr := strconv.Itoa(rec.status)
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, route, statusStr).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

