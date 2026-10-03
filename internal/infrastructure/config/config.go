package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                       string
	Host                         string
	Port                         int
	RequestTimeout               time.Duration
	DatabaseURL                  string
	DefaultOwnerID               int64
	RedisURL                     string
	RedisCacheTTL                time.Duration
	RedisCircuitBreakerThreshold int
	RedisCircuitBreakerCooldown  time.Duration
	JWTSecret                    string
	AccessTokenTTL               time.Duration
	RefreshTokenTTL              time.Duration
	BcryptCost                   int
	LogLevel                     string
	WorkerConcurrency            int
	WorkerQueueSize              int
	MaxInFlight                  int
	MaxProjectsPerUser           int
	MaxMembersPerProject         int
	MaxTasksPerProject           int
	ProjectCreationRateLimit     int
	AllowedOrigins               []string
	TrustedProxies               []string
	BrokerURL                    string
	WorkerHealthPort             int

	// HTTP Server Timeouts & Limits
	ReadHeaderTimeout   time.Duration
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	MaxHeaderBytes      int

	// Database Connection Pool
	DBMaxConns          int
	DBMinConns          int
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthCheckPeriod time.Duration

	// Auth Rate Limit
	AuthRateLimit       int
}

func Load() (Config, error) {
	// 1) APP_ENV — whitelist
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "" {
		appEnv = "development"
	}
	switch appEnv {
	case "development", "staging", "production":
	default:
		return Config{}, fmt.Errorf("APP_ENV must be one of: development, staging, production")
	}
	isProd := appEnv == "production"

	// 2) Host — default 0.0.0.0 in production
	defaultHost := "0.0.0.0"
	if !isProd {
		defaultHost = "127.0.0.1"
	}
	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		host = defaultHost
	}

	// 3) PORT
	port, err := envInt("PORT", 8080, 1, 65535)
	if err != nil {
		return Config{}, err
	}

	// 4) REQUEST_TIMEOUT — range between 1s and 2m
	timeout, err := envDuration("REQUEST_TIMEOUT", 5*time.Second, time.Second, 2*time.Minute)
	if err != nil {
		return Config{}, err
	}

	// 5) DATABASE_URL — required in production
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if isProd && databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required in production")
	}

	// 6) DEFAULT_OWNER_ID — disallowed in production
	var ownerID int64
	if raw := strings.TrimSpace(os.Getenv("DEFAULT_OWNER_ID")); raw != "" {
		if isProd {
			return Config{}, fmt.Errorf("DEFAULT_OWNER_ID must not be set in production")
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			return Config{}, fmt.Errorf("DEFAULT_OWNER_ID must be a positive integer")
		}
		ownerID = parsed
	} else {
		ownerID = 1
	}

	// 7) REDIS_URL — required in production
	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if isProd && redisURL == "" {
		return Config{}, fmt.Errorf("REDIS_URL is required in production")
	}

	// 8) REDIS_CACHE_TTL — range between 1s and 1h
	cacheTTL, err := envDuration("REDIS_CACHE_TTL", 30*time.Second, time.Second, time.Hour)
	if err != nil {
		return Config{}, err
	}

	// 8.1) REDIS_CIRCUIT_BREAKER_THRESHOLD — default 5
	cbThreshold, err := envInt("REDIS_CIRCUIT_BREAKER_THRESHOLD", 5, 1, 100)
	if err != nil {
		return Config{}, err
	}

	// 8.2) REDIS_CIRCUIT_BREAKER_COOLDOWN — default 10s
	cbCooldown, err := envDuration("REDIS_CIRCUIT_BREAKER_COOLDOWN", 10*time.Second, time.Second, 10*time.Minute)
	if err != nil {
		return Config{}, err
	}

	// 9) JWT_SECRET — mandatory in production, length >= 32
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	defaultSecret := "workhub-dev-secret-key-32bytes-long-change-in-prod!"
	if jwtSecret == "" {
		if isProd {
			return Config{}, fmt.Errorf("JWT_SECRET is required in production")
		}
		jwtSecret = defaultSecret
	}
	if jwtSecret == defaultSecret && isProd {
		return Config{}, fmt.Errorf("JWT_SECRET must not use the default development key in production")
	}
	if len(jwtSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 bytes long")
	}

	// 10) ACCESS_TOKEN_TTL — range between 1m and 1h
	accessTTL, err := envDuration("ACCESS_TOKEN_TTL", 15*time.Minute, time.Minute, time.Hour)
	if err != nil {
		return Config{}, err
	}

	// 11) REFRESH_TOKEN_TTL — range between 1h and 90d
	refreshTTL, err := envDuration("REFRESH_TOKEN_TTL", 168*time.Hour, time.Hour, 90*24*time.Hour)
	if err != nil {
		return Config{}, err
	}

	// 12) BCRYPT_COST — range between 10 and 14
	bcryptCost, err := envInt("BCRYPT_COST", 12, 10, 14)
	if err != nil {
		return Config{}, err
	}

	// 13) LOG_LEVEL — whitelist
	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}
	if isProd && logLevel == "debug" {
		return Config{}, fmt.Errorf("LOG_LEVEL=debug is not allowed in production")
	}

	// 14) WORKER_CONCURRENCY, WORKER_QUEUE_SIZE & MAX_IN_FLIGHT
	workerConcurrency, err := envInt("WORKER_CONCURRENCY", 3, 1, 50)
	if err != nil {
		return Config{}, err
	}
	workerQueueSize, err := envInt("WORKER_QUEUE_SIZE", 1000, 1, 100000)
	if err != nil {
		return Config{}, err
	}
	maxInFlight, err := envInt("MAX_IN_FLIGHT", 1000, 10, 100000)
	if err != nil {
		return Config{}, err
	}

	// 15) Quotas
	maxProjects, err := envInt("MAX_PROJECTS_PER_USER", 50, -1, 10000)
	if err != nil {
		return Config{}, err
	}
	maxMembers, err := envInt("MAX_MEMBERS_PER_PROJECT", 100, -1, 10000)
	if err != nil {
		return Config{}, err
	}
	maxTasks, err := envInt("MAX_TASKS_PER_PROJECT", 1000, -1, 50000)
	if err != nil {
		return Config{}, err
	}

	// 16) Project Creation Rate Limit
	projectCreationLimit, err := envInt("PROJECT_CREATION_RATE_LIMIT", 10, 1, 1000)
	if err != nil {
		return Config{}, err
	}

	// 17) ALLOWED_ORIGINS — explicit whitelist
	allowedOrigins := parseCSV(os.Getenv("ALLOWED_ORIGINS"))
	if len(allowedOrigins) == 0 {
		if isProd {
			return Config{}, fmt.Errorf("ALLOWED_ORIGINS is required in production")
		}
		// In development: allow localhost defaults
		allowedOrigins = []string{
			"http://localhost:3000",
			"http://localhost:8080",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:8080",
		}
	}
	// Explicitly prohibit wildcard in production
	for _, o := range allowedOrigins {
		if o == "*" && isProd {
			return Config{}, fmt.Errorf("ALLOWED_ORIGINS=* is not allowed in production")
		}
	}

	// 18) TRUSTED_PROXIES
	trustedProxies := parseCSV(os.Getenv("TRUSTED_PROXIES"))

	// 19) BROKER_URL — optional in dev (falls back to in-memory or RedisURL)
	brokerURL := strings.TrimSpace(os.Getenv("BROKER_URL"))

	// 20) WORKER_HEALTH_PORT — default 9091
	workerHealthPort, err := envInt("WORKER_HEALTH_PORT", 9091, 1, 65535)
	if err != nil {
		return Config{}, err
	}

	// 21) HTTP Timeouts & Limits
	readHeaderTimeout, err := envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second, time.Second, time.Minute)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := envDuration("HTTP_READ_TIMEOUT", 15*time.Second, time.Second, 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := envDuration("HTTP_WRITE_TIMEOUT", 30*time.Second, time.Second, 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := envDuration("HTTP_IDLE_TIMEOUT", 60*time.Second, time.Second, 10*time.Minute)
	if err != nil {
		return Config{}, err
	}
	maxHeaderBytes, err := envInt("HTTP_MAX_HEADER_BYTES", 1<<20, 1024, 10<<20)
	if err != nil {
		return Config{}, err
	}

	// 22) Database Pool Settings
	dbMaxConns, err := envInt("DB_MAX_CONNS", 25, 1, 500)
	if err != nil {
		return Config{}, err
	}
	dbMinConns, err := envInt("DB_MIN_CONNS", 5, 0, 100)
	if err != nil {
		return Config{}, err
	}
	dbMaxConnLifetime, err := envDuration("DB_MAX_CONN_LIFETIME", time.Hour, time.Minute, 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	dbMaxConnIdleTime, err := envDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute, time.Minute, 12*time.Hour)
	if err != nil {
		return Config{}, err
	}
	dbHealthCheckPeriod, err := envDuration("DB_HEALTH_CHECK_PERIOD", time.Minute, 5*time.Second, time.Hour)
	if err != nil {
		return Config{}, err
	}

	// 23) Auth Rate Limit
	authRateLimit, err := envInt("AUTH_RATE_LIMIT", 5, 1, 1000)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:                       appEnv,
		Host:                         host,
		Port:                         port,
		RequestTimeout:               timeout,
		DatabaseURL:                  databaseURL,
		DefaultOwnerID:               ownerID,
		RedisURL:                     redisURL,
		RedisCacheTTL:                cacheTTL,
		RedisCircuitBreakerThreshold: cbThreshold,
		RedisCircuitBreakerCooldown:  cbCooldown,
		JWTSecret:                    jwtSecret,
		AccessTokenTTL:               accessTTL,
		RefreshTokenTTL:              refreshTTL,
		BcryptCost:                   bcryptCost,
		LogLevel:                     logLevel,
		WorkerConcurrency:            workerConcurrency,
		WorkerQueueSize:              workerQueueSize,
		MaxInFlight:                  maxInFlight,
		MaxProjectsPerUser:           maxProjects,
		MaxMembersPerProject:         maxMembers,
		MaxTasksPerProject:           maxTasks,
		ProjectCreationRateLimit:     projectCreationLimit,
		AllowedOrigins:               allowedOrigins,
		TrustedProxies:               trustedProxies,
		BrokerURL:                    brokerURL,
		WorkerHealthPort:             workerHealthPort,
		ReadHeaderTimeout:            readHeaderTimeout,
		ReadTimeout:                  readTimeout,
		WriteTimeout:                 writeTimeout,
		IdleTimeout:                  idleTimeout,
		MaxHeaderBytes:               maxHeaderBytes,
		DBMaxConns:                   dbMaxConns,
		DBMinConns:                   dbMinConns,
		DBMaxConnLifetime:            dbMaxConnLifetime,
		DBMaxConnIdleTime:            dbMaxConnIdleTime,
		DBHealthCheckPeriod:          dbHealthCheckPeriod,
		AuthRateLimit:                authRateLimit,
	}, nil
}

// envInt parses an environment variable as integer with range validation.
func envInt(key string, def, min, max int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("%s must be between %d and %d", key, min, max)
	}
	return parsed, nil
}

// envDuration parses an environment variable as duration with range validation.
func envDuration(key string, def, min, max time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration (e.g., 15m, 1h)", key)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("%s must be between %s and %s", key, min, max)
	}
	return parsed, nil
}

// parseCSV splits a comma-separated string, trims whitespace, and omits empty entries.
func parseCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
