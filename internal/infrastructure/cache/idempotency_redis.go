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
	// stateProcessing — Sentinel value stored in key during execution.
	stateProcessing = "processing"

	// defaultTTL — Retention period for cached idempotent responses.
	defaultTTL = 24 * time.Hour

	// processingTTL — Max duration for "processing" state lock before considering request hung/timed out.
	processingTTL = 30 * time.Second

	// maxBodySize — Maximum cached response body size (256 KB). Responses above this fail open.
	maxBodySize = 256 * 1024 // 256 KB
)

// sensitiveHeaders are stripped and excluded from cached idempotency responses.
var sensitiveHeaders = map[string]struct{}{
	"set-cookie":         {},
	"authorization":      {},
	"www-authenticate":   {},
	"proxy-authenticate": {},
}

// IdempotencyResponse holds the cached HTTP response representation.
type IdempotencyResponse struct {
	StatusCode  int                 `json:"status_code"`
	Headers     map[string][]string `json:"headers"`
	Body        []byte              `json:"body"`
	Fingerprint string              `json:"fingerprint"` // sha256(request-body)
}

// ErrFingerprintMismatch indicates the idempotency key was reused with a conflicting payload body.
var ErrFingerprintMismatch = errors.New("idempotency key reused with different request body")

// ErrInProgress indicates another request with the same idempotency key is currently processing.
var ErrInProgress = errors.New("idempotency key is currently in progress")

// IdempotencyStore defines storage contract for idempotent requests.
type IdempotencyStore interface {
	// Get retrieves cached response if available.
	Get(ctx context.Context, userID, key, fingerprint string) (*IdempotencyResponse, error)

	// Begin attempts atomic key acquisition for exclusive execution.
	// - returns (true, nil) if key acquired -> caller must process operation.
	// - returns (false, nil) if response already cached -> use Get.
	// - returns (false, ErrInProgress) if another request is currently executing.
	// - returns (false, ErrFingerprintMismatch) if key reused with different body.
	Begin(ctx context.Context, userID, key, fingerprint string) (bool, error)

	// Save persists final response and releases lock.
	Save(ctx context.Context, userID, key string, resp *IdempotencyResponse) error

	// Fail releases lock on operation failure allowing retry.
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

// Fingerprint computes sha256 checksum of request body.
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

	// Verify fingerprint: identical key must carry matching payload body
	if resp.Fingerprint != "" && resp.Fingerprint != fingerprint {
		metrics.IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
		return nil, ErrFingerprintMismatch
	}

	metrics.CacheOperationsTotal.WithLabelValues("idempotency_hit", "ok").Inc()
	metrics.IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
	return &resp, nil
}

// Begin attempts atomic key reservation via SET NX.
func (s *RedisIdempotencyStore) Begin(
	ctx context.Context,
	userID, key, fingerprint string,
) (bool, error) {
	// 1) Attempt atomic acquisition of "processing" state
	start := time.Now()
	ok, err := s.client.SetNX(ctx, s.stateKey(userID, key), stateProcessing, processingTTL).Result()
	metrics.CacheOperationDuration.WithLabelValues("idempotency_begin").Observe(time.Since(start).Seconds())

	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_begin", "error").Inc()
		return false, fmt.Errorf("redis setnx: %w", err)
	}

	if ok {
		// Acquisition succeeded: this request is first to execute
		metrics.CacheOperationsTotal.WithLabelValues("idempotency_begin", "ok").Inc()
		return true, nil
	}

	// 2) Acquisition failed: check if request is in-flight or completed
	state, err := s.client.Get(ctx, s.stateKey(userID, key)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("redis get state: %w", err)
	}

	if state == stateProcessing {
		// Request currently in progress
		return false, ErrInProgress
	}

	// 3) Neither active state nor response found: possible crash; retry fetch
	resp, err := s.Get(ctx, userID, key, fingerprint)
	if err != nil {
		return false, err
	}
	if resp != nil {
		// Cached result found: replay
		return false, nil
	}

	// Retry acquisition in rare race condition
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
	// 1) Body size limit: do not cache oversized payloads (fail open)
	if len(resp.Body) > maxBodySize {
		_ = s.Fail(ctx, userID, key)
		return nil
	}

	// 2) Filter sensitive headers
	safeHeaders := make(map[string][]string, len(resp.Headers))
	for k, v := range resp.Headers {
		if _, sensitive := sensitiveHeaders[normalizeHeader(k)]; sensitive {
			continue
		}
		safeHeaders[k] = v
	}
	resp.Headers = safeHeaders

	// 3) Serialize and persist
	data, err := json.Marshal(resp)
	if err != nil {
		_ = s.Fail(ctx, userID, key)
		return fmt.Errorf("marshal: %w", err)
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, s.respKey(userID, key), data, s.ttl)
	pipe.Del(ctx, s.stateKey(userID, key)) // Release "processing" state
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

// Fail releases the processing lock on operation failure (5xx or error).
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
