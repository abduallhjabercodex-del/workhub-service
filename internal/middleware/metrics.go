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

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"strconv"
	"time"
	"github.com/gofiber/fiber/v2"
	"workhub-service/internal/infrastructure/metrics"
)

// MetricsFiber tracks in-flight requests, durations, and status codes.
// Note: Fiber eliminates the need for ResponseWriter wrapper structs like metricsRecorder,
// because c.Response().StatusCode() is directly accessible.
func MetricsFiber() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()
		if path == "/metrics" || path == "/health" || path == "/ready" || path == "/startup" {
			return c.Next()
		}

		route := c.Route().Path
		if route == "" {
			route = "unknown"
		}

		metrics.HTTPRequestsInFlight.WithLabelValues(route).Inc()
		defer metrics.HTTPRequestsInFlight.WithLabelValues(route).Dec()

		start := time.Now()
		err := c.Next()

		status := c.Response().StatusCode()
		statusStr := strconv.Itoa(status)
		metrics.HTTPRequestsTotal.WithLabelValues(c.Method(), route, statusStr).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(c.Method(), route).Observe(time.Since(start).Seconds())

		return err
	}
}
*/
