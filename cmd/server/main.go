package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"workhub-service/internal/domain"
	"workhub-service/internal/handler"
	"workhub-service/internal/infrastructure/cache"
	"workhub-service/internal/infrastructure/circuitbreaker"
	"workhub-service/internal/infrastructure/config"
	"workhub-service/internal/infrastructure/logger"
	"workhub-service/internal/infrastructure/messaging"
	"workhub-service/internal/infrastructure/retry"
	"workhub-service/internal/infrastructure/telemetry"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
	"workhub-service/internal/worker"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := setupLogger(cfg.LogLevel)

	shutdownTracer, err := telemetry.InitTracer()
	if err != nil {
		logger.Warn("failed to initialize tracer", "error", err)
	} else {
		defer func() {
			if err := shutdownTracer(context.Background()); err != nil {
				logger.Error("failed to shutdown tracer", "error", err)
			}
		}()
	}

	// --- Database ---
	var projects domain.ProjectRepository
	var tasks domain.TaskRepository
	var closeStore func() = func() {}

	if cfg.DatabaseURL == "" {
		if cfg.AppEnv == "production" {
			return errors.New("DATABASE_URL required in production")
		}
		logger.Warn("using in-memory repository (development only)")
		memory := repository.NewMemoryRepository()
		projects, tasks = memory, memory
	} else {
		pool, err := setupPool(cfg)
		if err != nil {
			return err
		}
		store := repository.NewPostgresRepository(pool)
		projects, tasks = store, store
		closeStore = store.Close
		logger.Info("postgres connected")
	}
	defer closeStore()

	// --- Redis ---
	var redisClient *redis.Client
	var summaryCache service.SummaryCache
	var deliveryStore worker.DeliveryStore = worker.NewMemoryDeliveryStore()

	if cfg.RedisURL != "" {
		client, err := setupRedis(cfg)
		if err != nil {
			if cfg.AppEnv == "production" {
				return fmt.Errorf("redis required in production: %w", err)
			}
			logger.Warn("redis unavailable in dev", "error", err)
		} else {
			redisClient = client
			defer func() {
				if err := redisClient.Close(); err != nil {
					logger.Error("redis close failed", "error", err)
				}
			}()
			// Pass breaker to cache
			// يتطلب 5 إخفاقات متتالية ليُفتح (Open)،
			// ويبقى مفتوحاً لمدة 10 ثوانٍ (Cooldown) قبل أن يحاول مجدداً.

			redisBreaker := circuitbreaker.NewWithConfig(circuitbreaker.Config{
				Threshold: cfg.RedisCircuitBreakerThreshold, // default 5
				Cooldown:  cfg.RedisCircuitBreakerCooldown,  // default 10s
				Name:      "redis",
			})
			defer redisBreaker.Close()

			// ثم مررناه إلى خدمة الكاش
			summaryCache = cache.NewProjectSummaryCache(redisClient, cfg.RedisCacheTTL, logger, redisBreaker)
			deliveryStore = worker.NewRedisDeliveryStore(redisClient, 24*time.Hour)
			logger.Info("redis enabled")
		}
	}

	// --- Repos ---
	var users domain.UserRepository
	var authRepo domain.AuthRepository
	var memberships domain.MembershipRepository
	var quotas domain.QuotaRepository
	if u, ok := projects.(domain.UserRepository); ok {
		users = u
	}
	if a, ok := projects.(domain.AuthRepository); ok {
		authRepo = a
	}
	if m, ok := projects.(domain.MembershipRepository); ok {
		memberships = m
	}
	if q, ok := projects.(domain.QuotaRepository); ok {
		quotas = q
	}

	// --- Event Broker ---
	var eventBroker messaging.EventBroker
	if cfg.BrokerURL != "" {
		brokerOpts, err := redis.ParseURL(cfg.BrokerURL)
		if err != nil {
			return fmt.Errorf("parse broker url: %w", err)
		}
		brokerClient := redis.NewClient(brokerOpts)
		pingCtx, cancelPing := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelPing()
		if err := brokerClient.Ping(pingCtx).Err(); err != nil {
			return fmt.Errorf("ping broker: %w", err)
		}
		defer brokerClient.Close()
		redisBroker := messaging.NewRedisBroker(brokerClient)
		defer redisBroker.Close()
		eventBroker = redisBroker
		logger.Info("distributed redis broker connected", "broker_url", cfg.BrokerURL)
	} else {
		inMemBroker := messaging.NewBroker(cfg.WorkerConcurrency)
		defer inMemBroker.Close()
		eventBroker = inMemBroker
		logger.Info("in-memory event broker initialized")
	}

	// --- Service ---
	deps := service.Deps{
		Projects: projects, Tasks: tasks, Users: users, Auth: authRepo,
		Memberships: memberships, Quotas: quotas, Cache: summaryCache,
		Publisher:            eventBroker,
		MaxProjectsPerUser:   cfg.MaxProjectsPerUser,
		MaxMembersPerProject: cfg.MaxMembersPerProject,
		MaxTasksPerProject:   cfg.MaxTasksPerProject,
		Config: service.AuthConfig{
			JWTSecret: cfg.JWTSecret, AccessTokenTTL: cfg.AccessTokenTTL,
			RefreshTokenTTL: cfg.RefreshTokenTTL, BCryptCost: cfg.BcryptCost,
		},
	}
	workhub, err := service.NewWorkHub(deps)
	if err != nil {
		return fmt.Errorf("service: %w", err)
	}

	// --- Worker (run in-process only if external BrokerURL is not set) ---
	var wg sync.WaitGroup
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer func() {
		workerCancel()
		wg.Wait()
	}()

	if cfg.BrokerURL == "" {
		retryCfg := retry.DefaultConfig()
		retryCfg.MaxAttempts = 3 // Standard default

		pool := worker.NewWorkerPool(cfg.WorkerConcurrency, cfg.WorkerQueueSize)

		notificationWorker := worker.NewNotificationWorker(
			eventBroker, deliveryStore,
			worker.LogNotifier{Logger: logger}, retryCfg, pool)
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := notificationWorker.Run(workerCtx)
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("in-process worker stopped with error", "error", err)
			} else {
				logger.Info("in-process worker stopped gracefully")
			}
		}()
	} else {
		logger.Info("standalone worker mode: skipping in-process worker (handled by external worker container)")
	}

	// --- HTTP ---
	var idempotencyStore cache.IdempotencyStore
	if redisClient != nil {
		idempotencyStore = cache.NewRedisIdempotencyStore(redisClient, 24*time.Hour)
	}
	api := handler.NewAPI(workhub, trustedProxies(cfg), idempotencyStore, logger)
	authHandler := middleware.Authenticate(cfg.JWTSecret)(api.Routes())

	// Add OTel HTTP middleware to trace all incoming requests
	otelHandler := otelhttp.NewHandler(authHandler, "workhub-api")

	finalHandler := middleware.LoadShedding(cfg.MaxInFlight, logger)(otelHandler)
	finalHandler = middleware.Chain(finalHandler, logger, cfg.RequestTimeout)
	finalHandler = middleware.SecurityHeaders("")(finalHandler)
	finalHandler = middleware.CORS(cfg.AllowedOrigins)(finalHandler)

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           finalHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	// --- Internal Profiling Server (pprof on isolated admin port 6060) ---
	if os.Getenv("ENABLE_PROFILING") == "true" || cfg.AppEnv != "production" {
		runtime.SetBlockProfileRate(1000)
		runtime.SetMutexProfileFraction(5)
		pprofServer := &http.Server{
			Addr:         "127.0.0.1:6060",
			Handler:      http.DefaultServeMux, // net/http/pprof mounts on DefaultServeMux
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 60 * time.Second,
		}
		go func() {
			logger.Info("pprof debug server listening", "address", pprofServer.Addr)
			if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Warn("pprof server stopped", "error", err)
			}
		}()
		defer func() {
			shutCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
			defer c()
			_ = pprofServer.Shutdown(shutCtx)
		}()
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", server.Addr)

		api.AppHealth().MarkReady(true)
		api.AppHealth().MarkStartupComplete(true)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		api.AppHealth().MarkReady(false)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		_ = server.Close()
	}
	logger.Info("server shut down")
	return nil
}
func setupLogger(level string) *slog.Logger {
	lvl := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindString {
				key := strings.ToLower(a.Key)
				if strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "authorization") {
					return slog.String(a.Key, "[REDACTED]")
				}
			}
			return a
		},
	}

	jsonHandler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(logger.ContextHandler{Handler: jsonHandler})
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func setupPool(cfg config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pgxCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}
	pgxCfg.MaxConns = 25
	pgxCfg.MinConns = 5
	pgxCfg.MaxConnLifetime = time.Hour
	pgxCfg.MaxConnIdleTime = 30 * time.Minute
	pgxCfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

func setupRedis(cfg config.Config) (*redis.Client, error) {
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func trustedProxies(cfg config.Config) map[string]struct{} {
	m := make(map[string]struct{})
	for _, ip := range cfg.TrustedProxies {
		m[strings.TrimSpace(ip)] = struct{}{}
	}
	return m
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"context"
	"fmt"
	"time"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/contrib/otelfiber"
	"workhub-service/internal/handler"
	"workhub-service/internal/infrastructure/cache"
	"workhub-service/internal/middleware"
)

// setupFiberServer demonstrates how the HTTP layer is wired with Fiber:
func setupFiberServer(cfg config.Config, api *handler.API, logger *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		BodyLimit:    10 * 1024 * 1024,
		DisableStartupMessage: true,
	})

	// 1. Tracing & Telemetry (OpenTelemetry)
	app.Use(otelfiber.NewMiddleware(otelfiber.WithServerName("workhub-api")))

	// 2. Core Middlewares
	app.Use(middleware.RecoveryFiber(logger))
	app.Use(middleware.RequestIDFiber())
	app.Use(middleware.RequestLogFiber(logger))
	app.Use(middleware.MetricsFiber())
	app.Use(middleware.LoadSheddingFiber(cfg.MaxInFlight, logger))
	app.Use(middleware.SecurityHeadersFiber(""))
	app.Use(middleware.CORSFiber(cfg.AllowedOrigins))
	app.Use(middleware.AuthenticateFiber(cfg.JWTSecret))

	// 3. Register Routes
	api.RegisterRoutesFiber(app)

	return app
}

// Running and Graceful Shutdown with Fiber:
//
// app := setupFiberServer(cfg, api, logger)
//
// go func() {
//     addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
//     logger.Info("fiber server listening", "address", addr)
//     if err := app.Listen(addr); err != nil {
//         logger.Error("fiber server stopped", "error", err)
//     }
// }()
//
// <-quit
// shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
// defer cancel()
// if err := app.ShutdownWithContext(shutCtx); err != nil {
//     logger.Error("fiber graceful shutdown error", "error", err)
// }
*/
