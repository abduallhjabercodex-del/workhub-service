package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Host           string
	Port           int
	RequestTimeout time.Duration
	DatabaseURL    string
	DefaultOwnerID int64
	RedisURL       string
	RedisCacheTTL  time.Duration
}

func Load() (Config, error) {
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 8080
	if raw := os.Getenv("PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("PORT must be between 1 and 65535")
		}
		port = parsed
	}
	timeout := 5 * time.Second
	if raw := os.Getenv("REQUEST_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("REQUEST_TIMEOUT must be a positive duration")
		}
		timeout = parsed
	}
	ownerID := int64(1)
	if raw := os.Getenv("DEFAULT_OWNER_ID"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			return Config{}, fmt.Errorf("DEFAULT_OWNER_ID must be a positive integer")
		}
		ownerID = parsed
	}
	cacheTTL := 30 * time.Second
	if raw := os.Getenv("REDIS_CACHE_TTL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("REDIS_CACHE_TTL must be a positive duration")
		}
		cacheTTL = parsed
	}
	return Config{
		Host: host, Port: port, RequestTimeout: timeout,
		DatabaseURL: os.Getenv("DATABASE_URL"), DefaultOwnerID: ownerID,
		RedisURL: os.Getenv("REDIS_URL"), RedisCacheTTL: cacheTTL,
	}, nil
}
