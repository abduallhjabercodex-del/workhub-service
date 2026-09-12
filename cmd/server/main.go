package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"workhub-service/internal/cache"
	"workhub-service/internal/config"
	"workhub-service/internal/handler"
	"workhub-service/internal/messaging"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
	"workhub-service/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var projects repository.ProjectRepository
	var tasks repository.TaskRepository
	var closeStore func()
	if cfg.DatabaseURL == "" {
		logger.Warn("DATABASE_URL is empty; using in-memory repository")
		memory := repository.NewMemoryRepository()
		projects, tasks = memory, memory
		closeStore = func() {}
	} else {
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Error("create database pool", "error", err)
			os.Exit(1)
		}
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err != nil {
			pool.Close()
			logger.Error("database is not reachable", "error", err)
			os.Exit(1)
		}
		store := repository.NewPostgresRepository(pool, cfg.DefaultOwnerID)
		projects, tasks = store, store
		closeStore = store.Close
		logger.Info("using PostgreSQL repository", "defaultOwnerID", cfg.DefaultOwnerID)
	}
	defer closeStore()
	workhub := service.NewWorkHub(projects, tasks)
	broker := messaging.NewBroker(3)
	deliveryStore := worker.NewMemoryDeliveryStore()
	notificationWorker := worker.NewNotificationWorker(broker, deliveryStore, worker.LogNotifier{Logger: logger}, 3)
	workhub.SetEventPublisher(broker)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go func() {
		if err := notificationWorker.Run(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("notification worker stopped", "error", err)
		}
	}()
	if cfg.RedisURL != "" {
		redisOptions, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			logger.Error("invalid REDIS_URL", "error", err)
			os.Exit(1)
		}
		redisClient := redis.NewClient(redisOptions)
		redisCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := redisClient.Ping(redisCtx).Err(); err != nil {
			logger.Warn("redis unavailable; continuing without cache", "error", err)
			redisClient.Close()
		} else {
			workhub.SetSummaryCache(cache.NewProjectSummaryCache(redisClient, cfg.RedisCacheTTL, logger))
			logger.Info("project summary cache enabled", "ttl", cfg.RedisCacheTTL.String())
		}
		cancel()
	}
	api := handler.NewAPI(workhub)
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	logger.Info("server listening", "address", addr)
	server := &http.Server{Addr: addr, Handler: middleware.Chain(api.Routes(), logger, cfg.RequestTimeout), ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
