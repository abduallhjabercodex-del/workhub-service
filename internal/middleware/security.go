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

// SecurityHeaders — رؤوس أمان OWASP.
// CSP يجب أن يُضبط حسب احتياجاتك. إذا كنت تستخدم Swagger UI، أبقِ unpkg.
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
			w.Header().Set("X-Frame-Options", "DENY")                            // يمنع تحميل الصفحة داخل iframe في نطاق آخر.
			w.Header().Set("X-Content-Type-Options", "nosniff")                  // يمنع المتصفح من محاولة تخمين نوع المحتوى.
			w.Header().Set("X-XSS-Protection", "0")                              // أوقفت الحماية المدمجة في المتصفحات القديمة لمنع هجمات XSS، ونعتمد في ذلك على تنظيف المدخلات (Sanitization) من جهة السيرفر.
			w.Header().Set("Content-Security-Policy", csp)                       // يحدد مصادر المحتوى الموثوقة.
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin") // يقلل من كمية المعلومات التي يتم إرسالها مع كل طلب.
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			// HSTS فقط عند HTTPS.
			if r.TLS != nil {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS — whitelist صريح. لا تستخدم "*" مع Authorization.
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
					w.Header().Set("Access-Control-Max-Age", "86400") // يمكن للمتصفح أن يحتفظ بنتيجة CORS preflight لمدة تصل إلى 24 ساعة وفق سياسات المتصفح.  وهذا يقلل عدد طلبات OPTIONS.
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

// rateLimiterClient — Token Bucket لكل عميل.
type rateLimiterClient struct {
	tokens     int
	lastUpdate time.Time
}

// RateLimiterStore — In-Memory. لا تستخدمه في Production Cluster.
// للإنتاج: استخدم Redis-backed limiter (sliding window + Lua).
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
		maxClients:    100_000, // حد أقصى لتفادي Memory DoS
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

	// Cleanup دوري للـ clients الخاملة
	if now.Sub(s.lastCleanup) > s.cleanupPeriod {
		s.cleanupLocked(now)
		s.lastCleanup = now
	}

	// حماية من Memory DoS
	if len(s.clients) >= s.maxClients {
		// ارفض الطلب (fail-closed) بدل النمو.
		return false, 0, s.capacity, int(now.Add(s.refillRate).Unix()), 1
	}

	c, exists := s.clients[key]
	if !exists {
		s.clients[key] = &rateLimiterClient{
			tokens:     s.capacity - 1, // أول طلب يستهلك رصيد
			lastUpdate: now,            // آخر تحديث للرصيد
		}
		resetTime := now.Add(time.Duration(s.capacity) * s.refillRate)    // وقت إعادة تعبئة الرصيد
		return true, s.capacity - 1, s.capacity, int(resetTime.Unix()), 0 // السماح بالطلب  //3 / 1 = 3
	}

	elapsed := now.Sub(c.lastUpdate)            // المدة المنقضية منذ آخر طلب 10:00  //lastUpdate = 10:00:00  now = 10:00:03  elapsed = 3 seconds
	refillTokens := int(elapsed / s.refillRate) // عدد التوكنز التي تمت إعادة تعبئتها
	if refillTokens > 0 {
		c.tokens += refillTokens //10 + 3 = 13
		if c.tokens > s.capacity {
			c.tokens = s.capacity
		}
		c.lastUpdate = now //lastUpdate = 10:00:03
	}

	if c.tokens > 0 {
		c.tokens--
		resetTime := now.Add(time.Duration(s.capacity-c.tokens) * s.refillRate) // 10 - 9 = 1
		return true, c.tokens, s.capacity, int(resetTime.Unix()), 0             // 13-10=3 * 1=3 => 10:00:03 + 3 seconds = 10:00:06
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

// clientIP يستخرج IP بأمان:
// - يستخدم X-Forwarded-For فقط إذا كان الطلب من proxy موثوق.
// - وإلا يستخدم RemoteAddr.
func clientIP(r *http.Request, trustedProxies map[string]struct{}) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}

	if _, trusted := trustedProxies[remoteHost]; trusted {
		// أول IP في X-Forwarded-For هو الأصلي.
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

// RateLimit — يستخدم user:ID إذا موجود، وإلا IP.
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
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(capacity))      // أقصى عدد طلبات مسموح بها.
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining)) // عدد الطلبات المتبقية.
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSec))      // الوقت المتبقي لإعادة تعيين العداد (بالثواني).

			if !allowed {
				scope := "ip"
				if strings.HasPrefix(key, "user:") {
					scope = "user"
				}
				metrics.RateLimitRejectedTotal.WithLabelValues(scope).Inc()

				w.Header().Set("Content-Type", "application/json")
				if retryAfterSec > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec)) // الوقت المتبقي قبل إعادة المحاولة.
				}
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded","message":"Too many requests. Please try again later."}}`))
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
	"strconv"
	"strings"
	"time"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"workhub-service/internal/infrastructure/metrics"
)

// SecurityHeadersFiber sets OWASP security headers in Fiber
func SecurityHeadersFiber(csp string) fiber.Handler {
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

	return func(c *fiber.Ctx) error {
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-XSS-Protection", "0")
		c.Set("Content-Security-Policy", csp)
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		if c.Protocol() == "https" {
			c.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		return c.Next()
	}
}

// CORSFiber can either use Fiber's built-in cors middleware or custom handler:
func CORSFiber(allowedOrigins []string) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins:     strings.Join(allowedOrigins, ", "),
		AllowCredentials: true,
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Authorization,Content-Type,X-Request-ID,Accept",
		MaxAge:           86400,
	})
}

// RateLimitFiber limits request rate per user/IP in Fiber
func RateLimitFiber(capacity int, refillRate time.Duration, trustedProxies map[string]struct{}) fiber.Handler {
	limiter := NewRateLimiter(capacity, refillRate)

	return func(c *fiber.Ctx) error {
		var key string
		if user, ok := UserFromFiberContext(c); ok && user.ID != "" {
			key = "user:" + user.ID
		} else {
			key = "ip:" + c.IP()
		}

		allowed, remaining, capVal, resetSec, retryAfterSec := limiter.AllowWithDetails(key)
		c.Set("X-RateLimit-Limit", strconv.Itoa(capVal))
		c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Set("X-RateLimit-Reset", strconv.Itoa(resetSec))

		if !allowed {
			scope := "ip"
			if strings.HasPrefix(key, "user:") {
				scope = "user"
			}
			metrics.RateLimitRejectedTotal.WithLabelValues(scope).Inc()

			if retryAfterSec > 0 {
				c.Set("Retry-After", strconv.Itoa(retryAfterSec))
			}
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "rate_limit_exceeded",
					"message": "Too many requests. Please try again later.",
				},
			})
		}

		return c.Next()
	}
}
*/
