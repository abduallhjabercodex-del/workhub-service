package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"workhub-service/internal/infrastructure/config"
	"workhub-service/internal/infrastructure/logger"
	"workhub-service/internal/infrastructure/messaging"
	"workhub-service/internal/infrastructure/retry"
	"workhub-service/internal/worker"

	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := setupLogger(cfg.LogLevel)
	log.Info("workhub worker starting",
		"env", cfg.AppEnv,
		"concurrency", cfg.WorkerConcurrency,
		"queue_size", cfg.WorkerQueueSize,
		"health_port", cfg.WorkerHealthPort,
	)

	// --- Redis (Delivery Store) ---
	var deliveryStore worker.DeliveryStore = worker.NewMemoryDeliveryStore()
	var redisClient *redis.Client
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("parse redis url: %w", err)
		}
		redisClient = redis.NewClient(opts)
		pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelPing()
		if err := redisClient.Ping(pingCtx).Err(); err != nil {
			if cfg.AppEnv == "production" {
				return fmt.Errorf("redis ping failed: %w", err)
			}
			log.Warn("redis unavailable for delivery store, using memory", "error", err)
		} else {
			deliveryStore = worker.NewRedisDeliveryStore(redisClient, 24*time.Hour)
			defer redisClient.Close()
			log.Info("redis delivery store initialized", "url", cfg.RedisURL)
		}
	}

	// --- Broker ---
	var eventBroker messaging.EventBroker
	brokerURL := cfg.BrokerURL
	if brokerURL == "" && cfg.RedisURL != "" {
		brokerURL = cfg.RedisURL
	}

	if brokerURL != "" {
		brokerOpts, err := redis.ParseURL(brokerURL)
		if err != nil {
			return fmt.Errorf("parse broker url: %w", err)
		}
		brokerClient := redis.NewClient(brokerOpts)
		pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelPing()
		if err := brokerClient.Ping(pingCtx).Err(); err != nil {
			return fmt.Errorf("ping broker failed: %w", err)
		}
		defer brokerClient.Close()
		redisBroker := messaging.NewRedisBroker(brokerClient)
		defer redisBroker.Close()
		eventBroker = redisBroker
		log.Info("distributed redis broker connected", "broker_url", brokerURL)
	} else {
		inMemBroker := messaging.NewBroker(cfg.WorkerConcurrency)
		defer inMemBroker.Close()
		eventBroker = inMemBroker
		log.Info("using in-memory event broker")
	}

	// --- Worker Setup ---
	retryCfg := retry.DefaultConfig()
	retryCfg.MaxAttempts = 3

	pool := worker.NewWorkerPool(cfg.WorkerConcurrency, cfg.WorkerQueueSize)
	notificationWorker := worker.NewNotificationWorker(
		eventBroker,
		deliveryStore,
		worker.LogNotifier{Logger: log},
		retryCfg,
		pool,
	)

	// --- Healthcheck HTTP Server (:9091/healthz) ---
	var isHealthy atomic.Bool
	isHealthy.Store(true)

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !isHealthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "shutting_down",
				"time":   time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	healthServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WorkerHealthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	go func() {
		log.Info("worker health server listening", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("health server failed", "error", err)
		}
	}()

	// --- Run Worker Goroutine ---
	var wg sync.WaitGroup
	workerCtx, workerCancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := notificationWorker.Run(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("notification worker exited with error", "error", err)
		} else {
			log.Info("notification worker loop stopped gracefully")
		}
	}()

	// --- Signal Listening ---
	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	<-sigCtx.Done()
	log.Info("worker shutdown signal received (SIGINT/SIGTERM), initiating graceful shutdown")
	isHealthy.Store(false)

	// 1. Cancel worker context so it stops picking up new tasks
	workerCancel()

	// 2. Wait for current in-flight worker tasks with timeout (30 seconds)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	workerDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(workerDone)
	}()

	select {
	case <-workerDone:
		log.Info("all background worker tasks completed gracefully")
	case <-shutdownCtx.Done():
		log.Warn("shutdown timeout reached, forcing worker exit")
	}

	// 3. Stop health server
	healthShutdownCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer healthCancel()
	_ = healthServer.Shutdown(healthShutdownCtx)

	log.Info("worker process shutdown complete")
	return nil
}

func setupLogger(level string) *slog.Logger {
	lvl := parseLogLevel(level)
	opts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindString {
				key := strings.ToLower(a.Key)
				if strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "secret") {
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
