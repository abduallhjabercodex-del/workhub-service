package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"workhub-service/internal/infrastructure/metrics"
)

// SecurityHeaders applies OWASP recommended security response headers.
// CSP should be tuned based on deployment needs. Retains unpkg for Swagger UI.
func SecurityHeaders(csp string) func(http.Handler) http.Handler {
	if csp == "" {
		csp = "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://unpkg.com; " +
			"style-src 'self' 'unsafe-inline' https://unpkg.com https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com; " +
			"img-src 'self' data: https://unpkg.com; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Frame-Options", "DENY")                            // Prevent framing / clickjacking
			w.Header().Set("X-Content-Type-Options", "nosniff")                  // Prevent MIME type sniffing
			w.Header().Set("X-XSS-Protection", "0")                              // Disable legacy buggy XSS filters in favor of CSP
			w.Header().Set("Content-Security-Policy", csp)                       // Enforce trusted origins
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin") // Limit referrer leakage
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			// Enforce HSTS only on TLS connections
			if r.TLS != nil {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS enforces explicit origin whitelist. Wildcard '*' prohibited with credentials.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimSpace(o)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, Accept")
					w.Header().Set("Access-Control-Max-Age", "86400") // Cache preflight response for up to 24 hours
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimiterClient holds token bucket state for a single client key.
type rateLimiterClient struct {
	tokens     int
	lastUpdate time.Time
}

// RateLimiterStore provides in-memory rate limiting. Use Redis-backed limiter for distributed clusters.
type RateLimiterStore struct {
	mu            sync.Mutex
	clients       map[string]*rateLimiterClient
	capacity      int
	refillRate    time.Duration
	lastCleanup   time.Time
	cleanupPeriod time.Duration
	maxClients    int
}

func NewRateLimiter(capacity int, refillRate time.Duration) *RateLimiterStore {
	if capacity < 1 {
		capacity = 1
	}
	if refillRate <= 0 {
		refillRate = time.Second
	}
	return &RateLimiterStore{
		clients:       make(map[string]*rateLimiterClient),
		capacity:      capacity,
		refillRate:    refillRate,
		lastCleanup:   time.Now(),
		cleanupPeriod: 5 * time.Minute,
		maxClients:    100_000, // Bound maximum tracked clients to avoid memory exhaustion
	}
}

func (s *RateLimiterStore) Allow(key string) bool {
	allowed, _, _, _, _ := s.AllowWithDetails(key)
	return allowed
}

func (s *RateLimiterStore) AllowWithDetails(key string) (allowed bool, remaining, capacity, resetSec, retryAfterSec int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Periodic cleanup of idle clients
	if now.Sub(s.lastCleanup) > s.cleanupPeriod {
		s.cleanupLocked(now)
		s.lastCleanup = now
	}

	// Protect against unbounded memory growth
	if len(s.clients) >= s.maxClients {
		// Reject request (fail-closed) rather than growing memory indefinitely
		return false, 0, s.capacity, int(now.Add(s.refillRate).Unix()), 1
	}

	c, exists := s.clients[key]
	if !exists {
		s.clients[key] = &rateLimiterClient{
			tokens:     s.capacity - 1, // First request consumes 1 token
			lastUpdate: now,
		}
		resetTime := now.Add(time.Duration(s.capacity) * s.refillRate)
		return true, s.capacity - 1, s.capacity, int(resetTime.Unix()), 0
	}

	elapsed := now.Sub(c.lastUpdate)
	refillTokens := int(elapsed / s.refillRate)
	if refillTokens > 0 {
		c.tokens += refillTokens
		if c.tokens > s.capacity {
			c.tokens = s.capacity
		}
		c.lastUpdate = now
	}

	if c.tokens > 0 {
		c.tokens--
		resetTime := now.Add(time.Duration(s.capacity-c.tokens) * s.refillRate)
		return true, c.tokens, s.capacity, int(resetTime.Unix()), 0
	}

	nextRefill := c.lastUpdate.Add(s.refillRate)  // 10:00:03 + 1 = 10:00:04
	waitSec := int(nextRefill.Sub(now).Seconds()) // 10:00:04 - 10:00:03 = 1 second
	if waitSec < 1 {
		waitSec = 1
	}
	return false, 0, s.capacity, int(nextRefill.Unix()), waitSec // 10:00:03 + 1 = 10:00:04
}

func (s *RateLimiterStore) cleanupLocked(now time.Time) {
	staleThreshold := 10 * s.refillRate
	for k, c := range s.clients {
		if now.Sub(c.lastUpdate) > staleThreshold {
			delete(s.clients, k)
		}
	}
}

// clientIP extracts the client IP address securely:
// - Uses X-Forwarded-For only when the immediate peer is a trusted proxy.
// - Falls back to RemoteAddr otherwise.
func clientIP(r *http.Request, trustedProxies map[string]struct{}) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}

	if _, trusted := trustedProxies[remoteHost]; trusted {
		// First IP in X-Forwarded-For is client originating IP
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			parts := strings.Split(xff, ",")
			first := strings.TrimSpace(parts[0])
			if first != "" {
				return first
			}
		}
	}
	return remoteHost
}

// RateLimit limits requests keyed by authenticated user:ID or client IP address.
func RateLimit(limiter *RateLimiterStore, trustedProxies map[string]struct{}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var key string
			if user, ok := UserFromContext(r.Context()); ok && user.ID != "" {
				key = "user:" + user.ID
			} else {
				key = "ip:" + clientIP(r, trustedProxies)
			}

			allowed, remaining, capacity, resetSec, retryAfterSec := limiter.AllowWithDetails(key)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(capacity))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSec))

			if !allowed {
				scope := "ip"
				if strings.HasPrefix(key, "user:") {
					scope = "user"
				}
				metrics.RateLimitRejectedTotal.WithLabelValues(scope).Inc()

				w.Header().Set("Content-Type", "application/json")
				if retryAfterSec > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
				}
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded","message":"Too many requests. Please try again later."}}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

