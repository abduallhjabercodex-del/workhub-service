package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"workhub-service/internal/infrastructure/metrics"

	"github.com/redis/go-redis/v9"
)

const (
	// stateProcessing — قيمة مخزّنة في مفتاح الحالة أثناء التنفيذ.
	stateProcessing = "processing"

	// defaultTTL — مدة تخزين النتيجة النهائية.
	defaultTTL = 24 * time.Hour

	// processingTTL — مدة اعتبار "قيد التنفيذ" صالحة.
	// إذا تجاوزتها دون نتيجة نهائية → الطلب فشل/عُلّق.
	processingTTL = 30 * time.Second

	// maxBodySize — حد أقصى لحجم الـ response body المخزَّن.
	// إذا تجاوز → لا نخزّن (fail-open، نعتمد على idempotency المستقبلي).
	maxBodySize = 256 * 1024 // 256 KB
)

// sensitiveHeaders — لا تُخزَّن (أو تُخزَّن فارغة).
var sensitiveHeaders = map[string]struct{}{
	"set-cookie":         {},
	"authorization":      {},
	"www-authenticate":   {},
	"proxy-authenticate": {},
}

// IdempotencyResponse — النتيجة المخزَّنة.
type IdempotencyResponse struct {
	StatusCode  int                 `json:"status_code"`
	Headers     map[string][]string `json:"headers"`
	Body        []byte              `json:"body"`
	Fingerprint string              `json:"fingerprint"` // sha256(request-body)
}

// ErrFingerprintMismatch — نفس المفتاح بجسم مختلف.
var ErrFingerprintMismatch = errors.New("idempotency key reused with different request body")

// ErrInProgress — نفس المفتاح قيد التنفيذ الآن.
var ErrInProgress = errors.New("idempotency key is currently in progress")

// IdempotencyStore — عقد التخزين.
type IdempotencyStore interface {
	// Get يقرأ النتيجة المخزّنة، إن وُجدت.
	Get(ctx context.Context, userID, key, fingerprint string) (*IdempotencyResponse, error)

	// Begin يحاول حجز المفتاح للتنفيذ الحصري.
	// - يعيد (true, nil) إذا نجح الحجز → على المستدعي تنفيذ العملية.
	// - يعيد (false, nil) إذا كانت النتيجة مخزّنة بالفعل → استخدم Get.
	// - يعيد (false, ErrInProgress) إذا كان طلب آخر قيد التنفيذ.
	// - يعيد (false, ErrFingerprintMismatch) إذا نفس المفتاح بجسم مختلف.
	Begin(ctx context.Context, userID, key, fingerprint string) (bool, error)

	// Save يخزّن النتيجة النهائية.
	Save(ctx context.Context, userID, key string, resp *IdempotencyResponse) error

	// Fail يُحرّر الحجز عند فشل العملية (يسمح بإعادة المحاولة).
	Fail(ctx context.Context, userID, key string) error
}

type RedisIdempotencyStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisIdempotencyStore(client *redis.Client, ttl time.Duration) *RedisIdempotencyStore {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &RedisIdempotencyStore{client: client, ttl: ttl}
}

func (s *RedisIdempotencyStore) respKey(userID, key string) string {
	return fmt.Sprintf("workhub:idempotency:%s:%s:resp", userID, key)
}

func (s *RedisIdempotencyStore) stateKey(userID, key string) string {
	return fmt.Sprintf("workhub:idempotency:%s:%s:state", userID, key)
}

// Fingerprint يحسب sha256 من body.
func Fingerprint(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

func (s *RedisIdempotencyStore) Get(
	ctx context.Context,
	userID, key, fingerprint string,
) (*IdempotencyResponse, error) {
	start := time.Now()
	val, err := s.client.Get(ctx, s.respKey(userID, key)).Result()
	metrics.CacheOperationDuration.WithLabelValues("idempotency_get").Observe(time.Since(start).Seconds())

	if errors.Is(err, redis.Nil) {
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_miss", "ok").Inc()
		metrics.IdempotencyRequestsTotal.WithLabelValues("new").Inc()
		return nil, nil
	}
	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_get", "error").Inc()
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var resp IdempotencyResponse
	if err := json.Unmarshal([]byte(val), &resp); err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_get", "error").Inc()
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	// فحص الـ fingerprint: نفس المفتاح يجب أن يحمل نفس الـ body.
	if resp.Fingerprint != "" && resp.Fingerprint != fingerprint {
		metrics.IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
		return nil, ErrFingerprintMismatch
	}

	metrics.CacheOperationsTotal.WithLabelValues("idempotency_hit", "ok").Inc()
	metrics.IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
	return &resp, nil
}

// Begin — حجز ذرّي للمفتاح عبر SET NX.
func (s *RedisIdempotencyStore) Begin(
	ctx context.Context,
	userID, key, fingerprint string,
) (bool, error) {
	// 1) حاول حجز حالة "processing" (ذرّي).
	start := time.Now()
	ok, err := s.client.SetNX(ctx, s.stateKey(userID, key), stateProcessing, processingTTL).Result()
	metrics.CacheOperationDuration.WithLabelValues("idempotency_begin").Observe(time.Since(start).Seconds())

	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_begin", "error").Inc()
		return false, fmt.Errorf("redis setnx: %w", err)
	}

	if ok {
		// نجح الحجز — نحن أول من ينفّذ.
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_begin", "ok").Inc()
		return true, nil
	}

	// 2) الحجز فشل — هناك طلب آخر. هل هو قيد التنفيذ أم انتهى؟
	state, err := s.client.Get(ctx, s.stateKey(userID, key)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("redis get state: %w", err)
	}

	if state == stateProcessing {
		// طلب آخر قيد التنفيذ.
		return false, ErrInProgress
	}

	// 3) لا state + لا resp → بدأ للتو ثم فشل. حاول مجددًا.
	resp, err := s.Get(ctx, userID, key, fingerprint)
	if err != nil {
		return false, err
	}
	if resp != nil {
		// النتيجة موجودة → استخدمها.
		return false, nil
	}

	// حاول الحجز مرة أخرى (race نادر).
	ok, err = s.client.SetNX(ctx, s.stateKey(userID, key), stateProcessing, processingTTL).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx retry: %w", err)
	}
	if !ok {
		return false, ErrInProgress
	}
	return true, nil
}

func (s *RedisIdempotencyStore) Save(
	ctx context.Context,
	userID, key string,
	resp *IdempotencyResponse,
) error {
	// 1) حجم الـ body: إذا تجاوز الحد → لا نخزّن النتيجة (fail-open).
	if len(resp.Body) > maxBodySize {
		_ = s.Fail(ctx, userID, key)
		return nil
	}

	// 2) فلترة الـ headers الحساسة.
	safeHeaders := make(map[string][]string, len(resp.Headers))
	for k, v := range resp.Headers {
		if _, sensitive := sensitiveHeaders[normalizeHeader(k)]; sensitive {
			continue
		}
		safeHeaders[k] = v
	}
	resp.Headers = safeHeaders

	// 3) تسلسل + تخزين.
	data, err := json.Marshal(resp)
	if err != nil {
		_ = s.Fail(ctx, userID, key)
		return fmt.Errorf("marshal: %w", err)
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, s.respKey(userID, key), data, s.ttl)
	pipe.Del(ctx, s.stateKey(userID, key)) // حرّر حالة "processing"
	start := time.Now()
	_, err = pipe.Exec(ctx)
	metrics.CacheOperationDuration.WithLabelValues("idempotency_save").Observe(time.Since(start).Seconds())

	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_save", "error").Inc()
		return fmt.Errorf("redis pipeline: %w", err)
	}
	metrics.CacheOperationsTotal.WithLabelValues("idempotency_save", "ok").Inc()
	return nil
}

// Fail — يُحرّر الحجز عند فشل العملية (5xx, error).
func (s *RedisIdempotencyStore) Fail(ctx context.Context, userID, key string) error {
	if err := s.client.Del(ctx, s.stateKey(userID, key)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

func normalizeHeader(h string) string {
	// RFC 7230: header names case-insensitive.
	b := make([]byte, 0, len(h))
	for i := 0; i < len(h); i++ {
		c := h[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b = append(b, c)
	}
	return string(b)
}
