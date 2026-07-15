// Package config loads and validates the server configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Default durations applied to the pgxpool connection pool when the
// corresponding environment variable is unset or fails to parse.
const (
	defaultDBMaxConnLifetime   = time.Hour
	defaultDBMaxConnIdleTime   = 30 * time.Minute
	defaultDBHealthCheckPeriod = time.Minute
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	// Port is the HTTP server listen port (default "8080").
	Port string
	// DatabaseURL is the PostgreSQL connection string (required).
	DatabaseURL string
	// JWTSecret is the HMAC secret used for signing and verifying JWT tokens (required).
	JWTSecret string
	// LLMGatewayURL is the base URL of the LLM Gateway service (default "http://localhost:8081").
	LLMGatewayURL string
	// CORSOrigins is a comma-separated list of allowed CORS origins (default "http://localhost:3000").
	CORSOrigins string
	// DBMaxConnLifetime is the maximum amount of time a pgxpool connection may
	// be reused before being closed (env DB_MAX_CONN_LIFETIME, default 1h).
	DBMaxConnLifetime time.Duration
	// DBMaxConnIdleTime is the maximum amount of time a pgxpool connection may
	// sit idle before being closed (env DB_MAX_CONN_IDLE_TIME, default 30m).
	DBMaxConnIdleTime time.Duration
	// DBHealthCheckPeriod is the interval at which pgxpool runs a background
	// health check on idle connections (env DB_HEALTH_CHECK_PERIOD, default 1m).
	DBHealthCheckPeriod time.Duration
}

// Load reads configuration from environment variables and returns a Config.
// DATABASE_URL and JWT_SECRET are required; PORT defaults to "8080" and LLM_GATEWAY_URL
// defaults to "http://localhost:8081". It returns an error if any required variable is missing.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	llmURL := os.Getenv("LLM_GATEWAY_URL")
	if llmURL == "" {
		llmURL = "http://localhost:8081"
	}

	corsOrigins := os.Getenv("CORS_ORIGINS")
	if corsOrigins == "" {
		corsOrigins = "http://localhost:3000"
	}

	dbMaxConnLifetime := parseDurationEnv("DB_MAX_CONN_LIFETIME", defaultDBMaxConnLifetime)
	dbMaxConnIdleTime := parseDurationEnv("DB_MAX_CONN_IDLE_TIME", defaultDBMaxConnIdleTime)
	dbHealthCheckPeriod := parseDurationEnv("DB_HEALTH_CHECK_PERIOD", defaultDBHealthCheckPeriod)

	return &Config{
		Port:                port,
		DatabaseURL:         dbURL,
		JWTSecret:           jwtSecret,
		LLMGatewayURL:       llmURL,
		CORSOrigins:         corsOrigins,
		DBMaxConnLifetime:   dbMaxConnLifetime,
		DBMaxConnIdleTime:   dbMaxConnIdleTime,
		DBHealthCheckPeriod: dbHealthCheckPeriod,
	}, nil
}

// parseDurationEnv reads the given environment variable and parses it as a
// time.Duration. If the variable is unset, it returns fallback. If the
// variable is set but fails to parse, it logs a warning via slog.Default()
// and returns fallback rather than propagating an error, since these
// settings are optional tuning knobs, not required configuration.
func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}

	d, err := time.ParseDuration(val)
	if err != nil {
		slog.Default().Warn("invalid duration for env var, using default",
			"env", key, "value", val, "default", fallback, "error", err)
		return fallback
	}

	return d
}
