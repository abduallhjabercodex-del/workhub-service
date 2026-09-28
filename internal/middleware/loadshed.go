package middleware

import (
	"log/slog"
	"net/http"
	"sync/atomic"

	"workhub-service/internal/infrastructure/metrics"
)

// LoadShedding returns a middleware that drops requests if there are too many in-flight.
func LoadShedding(maxInFlight int, logger *slog.Logger) func(http.Handler) http.Handler {
	var inFlight atomic.Int64

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			current := inFlight.Add(1)
			defer inFlight.Add(-1)

			if current > int64(maxInFlight) {
				logger.WarnContext(r.Context(), "load shedding request", "max_in_flight", maxInFlight, "current_in_flight", current)
				metrics.LoadShedRejectedTotal.Inc()

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "5") // suggest retrying after 5 seconds
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"service_unavailable","message":"server is currently overloaded"}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"log/slog"
	"sync/atomic"
	"github.com/gofiber/fiber/v2"
	"workhub-service/internal/infrastructure/metrics"
)

// LoadSheddingFiber drops incoming requests when concurrency exceeds maxInFlight
func LoadSheddingFiber(maxInFlight int, logger *slog.Logger) fiber.Handler {
	var inFlight atomic.Int64

	return func(c *fiber.Ctx) error {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)

		if current > int64(maxInFlight) {
			logger.WarnContext(c.UserContext(), "load shedding request",
				"max_in_flight", maxInFlight, "current_in_flight", current)
			metrics.LoadShedRejectedTotal.Inc()

			c.Set("Retry-After", "5")
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "service_unavailable",
					"message": "server is currently overloaded",
				},
			})
		}

		return c.Next()
	}
}
*/
