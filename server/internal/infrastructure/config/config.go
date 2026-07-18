// Package config loads and validates the server configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default durations applied to the pgxpool connection pool when the
// corresponding environment variable is unset or fails to parse.
const (
	defaultDBMaxConnLifetime   = time.Hour
	defaultDBMaxConnIdleTime   = 30 * time.Minute
	defaultDBHealthCheckPeriod = time.Minute
)

// Defaults applied to the gRPC LLM Gateway client's transport-selection and
// retry/backoff tuning knobs when the corresponding environment variable is
// unset or fails to parse.
const (
	defaultLLMGatewayTransport       = "rest"
	defaultLLMGatewayGRPCAddr        = "llm-gateway:50051"
	defaultLLMGatewayGRPCMaxRetries  = 3
	defaultLLMGatewayGRPCBaseBackoff = 100 * time.Millisecond
)

// defaultDefaultAIModel is the deployment-wide fallback model string used
// when DEFAULT_AI_MODEL is unset. It matches the literal that used to be
// hardcoded as usecase/message's package-level `defaultModel` constant
// before that constant moved into Config so it's configurable per
// deployment.
const defaultDefaultAIModel = "gpt-5-mini"

// Defaults applied to the Redis-backed rate limiter and Kratos whoami-cache
// tuning knobs (Step 33) when the corresponding environment variable is
// unset or fails to parse. These are operational tuning knobs, not required
// credentials, so an invalid value falls back to the default with a logged
// warning rather than failing Load.
const (
	defaultRateLimitLoginPerMinute    = 10
	defaultRateLimitAIInvokePerMinute = 20
	defaultWhoamiCacheTTL             = 30 * time.Second
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

	// S3Endpoint is the S3-compatible object storage endpoint used when
	// presigning URLs. It must be reachable by whichever caller (typically a
	// browser) will actually use the resulting presigned URL, which is why
	// the default is a host-reachable http://localhost:9000 rather than the
	// container-internal MinIO service address (env S3_ENDPOINT).
	S3Endpoint string
	// S3Region is the region used for SigV4 signing (env S3_REGION, default
	// "us-east-1"). MinIO ignores the region's real-world meaning but still
	// requires one to be set for signing.
	S3Region string
	// S3Bucket is the bucket attachments are stored in (env S3_BUCKET,
	// default "polyphony-attachments").
	S3Bucket string
	// S3AccessKey is the access key used for static credentials (env
	// S3_ACCESS_KEY, default "minioadmin").
	S3AccessKey string
	// S3SecretKey is the secret key used for static credentials (env
	// S3_SECRET_KEY, default "minioadmin").
	S3SecretKey string
	// S3ForcePathStyle selects path-style bucket addressing (required by
	// MinIO, which does not support virtual-hosted-style addressing) rather
	// than virtual-hosted-style (env S3_FORCE_PATH_STYLE, default true).
	S3ForcePathStyle bool
	// WSTicketSecret is the HMAC secret used to sign and verify short-lived
	// WebSocket upgrade tickets (see internal/interface/wsticket). It is read
	// from the optional WS_TICKET_SECRET env var; if unset, it defaults to
	// JWTSecret, which keeps local/dev setup zero-config while still
	// allowing an independent secret in environments that want one.
	WSTicketSecret string
	// AuthMode selects which domainauth.AuthService implementation
	// container.go wires up: "simple_jwt" (default) for SimpleJWTService, or
	// "kratos" for KratosAuthService (env AUTH_MODE). Load returns an error
	// for any other non-empty value.
	AuthMode string
	// KratosPublicURL is Ory Kratos's public API base URL, used for
	// self-service registration/login flows and /sessions/whoami (env
	// KRATOS_PUBLIC_URL, default "http://localhost:4433"). Only required
	// when AuthMode is "kratos".
	KratosPublicURL string
	// KratosAdminURL is Ory Kratos's admin API base URL, used for identity
	// management (env KRATOS_ADMIN_URL, default "http://localhost:4434").
	KratosAdminURL string
	// KratosCookieName is the name of the session cookie Ory Kratos issues,
	// read by the auth middleware as a fallback when no Authorization
	// header is present (env KRATOS_COOKIE_NAME, default "ory_kratos_session").
	KratosCookieName string

	// LLMGatewayTransport selects which ai.LLMGateway implementation
	// container.go wires up: "rest" (default) for the existing
	// gateway.LLMClient, or "grpc" for gateway.GRPCClient (env
	// LLM_GATEWAY_TRANSPORT). This is the Phase 8 swap point noted in
	// CLAUDE.md's Interface Swap Points table. Any value other than "rest"
	// or "grpc" falls back to "rest" with a logged warning, rather than
	// failing Load, since this is an optional transport-selection knob.
	LLMGatewayTransport string
	// LLMGatewayGRPCAddr is the dial target used by gateway.NewGRPCClient
	// when LLMGatewayTransport is "grpc" (env LLM_GATEWAY_GRPC_ADDR,
	// default "llm-gateway:50051").
	LLMGatewayGRPCAddr string
	// LLMGatewayGRPCMaxRetries is the maximum number of attempts
	// gateway.GRPCClient makes for a single RPC before giving up on
	// transient failures (env LLM_GATEWAY_GRPC_MAX_RETRIES, default 3).
	// Falls back to the default if unset or unparseable as an int.
	LLMGatewayGRPCMaxRetries int
	// LLMGatewayGRPCBaseBackoff is the initial delay in gateway.GRPCClient's
	// exponential backoff schedule between retry attempts (env
	// LLM_GATEWAY_GRPC_BASE_BACKOFF, default 100ms). Falls back to the
	// default if unset or unparseable as a time.Duration.
	LLMGatewayGRPCBaseBackoff time.Duration

	// RedisURL is the Redis connection string (env REDIS_URL), required only
	// when MessageHubDriver is "redis". It is passed to redis.ParseURL by
	// container.go to build the shared *redis.Client used by RedisHub (and,
	// in a later step, rate limiting/session caching).
	RedisURL string
	// MessageHubDriver selects the event.MessageHub implementation
	// container.go wires up: "inprocess" (default) for InProcessHub, a
	// single-process, dependency-free implementation suitable for local dev
	// without Redis, or "redis" for RedisHub, which fans events out via
	// Redis Pub/Sub so multiple API replicas share message delivery (env
	// MESSAGE_HUB_DRIVER). Load returns an error for any other non-empty
	// value, and for "redis" without REDIS_URL also set.
	MessageHubDriver string

	// DefaultAIModel is the deployment-wide fallback model string used by
	// usecase/message.resolveModel whenever an AI request omits an explicit
	// model and the target room has no configured
	// domainroom.Room.AIModel (see PATCH /rooms/:roomId/settings). Read
	// from env DEFAULT_AI_MODEL, defaulting to "gpt-5-mini" when unset.
	DefaultAIModel string

	// RateLimitLoginPerMinute is the per-client-IP token-bucket rate limit
	// applied to POST /auth/register and POST /auth/login (env
	// RATE_LIMIT_LOGIN_PER_MINUTE, default 10). Falls back to the default if
	// unset or unparseable as an int.
	RateLimitLoginPerMinute int
	// RateLimitAIInvokePerMinute is the per-authenticated-user token-bucket
	// rate limit applied to POST /rooms/:roomId/messages/ai and POST
	// /rooms/:roomId/messages/:messageId/regenerate (env
	// RATE_LIMIT_AI_INVOKE_PER_MINUTE, default 20). Falls back to the
	// default if unset or unparseable as an int.
	RateLimitAIInvokePerMinute int
	// WhoamiCacheTTL is the lifetime given to a cached KratosAuthService
	// ValidateToken result by interface/auth.CachedAuthService (env
	// WHOAMI_CACHE_TTL, default 30s). Falls back to the default if unset or
	// unparseable as a time.Duration. Unused when AuthMode is "simple_jwt",
	// since SimpleJWTService is never wrapped by CachedAuthService.
	WhoamiCacheTTL time.Duration
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

	corsOriginsRaw := os.Getenv("CORS_ORIGINS")
	if corsOriginsRaw == "" {
		corsOriginsRaw = "http://localhost:3000"
	}
	corsOrigins, err := parseCORSOrigins(corsOriginsRaw)
	if err != nil {
		return nil, err
	}

	dbMaxConnLifetime := parseDurationEnv("DB_MAX_CONN_LIFETIME", defaultDBMaxConnLifetime)
	dbMaxConnIdleTime := parseDurationEnv("DB_MAX_CONN_IDLE_TIME", defaultDBMaxConnIdleTime)
	dbHealthCheckPeriod := parseDurationEnv("DB_HEALTH_CHECK_PERIOD", defaultDBHealthCheckPeriod)

	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "http://localhost:9000"
	}
	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		s3Region = "us-east-1"
	}
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		s3Bucket = "polyphony-attachments"
	}
	s3AccessKey := os.Getenv("S3_ACCESS_KEY")
	if s3AccessKey == "" {
		s3AccessKey = "minioadmin"
	}
	s3SecretKey := os.Getenv("S3_SECRET_KEY")
	if s3SecretKey == "" {
		s3SecretKey = "minioadmin"
	}
	s3ForcePathStyle := parseBoolEnv("S3_FORCE_PATH_STYLE", true)

	wsTicketSecret := os.Getenv("WS_TICKET_SECRET")
	if wsTicketSecret == "" {
		wsTicketSecret = jwtSecret
	}

	authMode := os.Getenv("AUTH_MODE")
	if authMode == "" {
		authMode = "simple_jwt"
	}
	if authMode != "simple_jwt" && authMode != "kratos" {
		return nil, fmt.Errorf("AUTH_MODE must be %q or %q, got %q", "simple_jwt", "kratos", authMode)
	}

	kratosPublicURL := os.Getenv("KRATOS_PUBLIC_URL")
	if kratosPublicURL == "" {
		kratosPublicURL = "http://localhost:4433"
	}

	kratosAdminURL := os.Getenv("KRATOS_ADMIN_URL")
	if kratosAdminURL == "" {
		kratosAdminURL = "http://localhost:4434"
	}

	kratosCookieName := os.Getenv("KRATOS_COOKIE_NAME")
	if kratosCookieName == "" {
		kratosCookieName = "ory_kratos_session"
	}

	llmGatewayTransport := os.Getenv("LLM_GATEWAY_TRANSPORT")
	if llmGatewayTransport == "" {
		llmGatewayTransport = defaultLLMGatewayTransport
	}
	if llmGatewayTransport != "rest" && llmGatewayTransport != "grpc" {
		slog.Default().Warn("invalid LLM_GATEWAY_TRANSPORT, using default",
			"value", llmGatewayTransport, "default", defaultLLMGatewayTransport)
		llmGatewayTransport = defaultLLMGatewayTransport
	}

	llmGatewayGRPCAddr := os.Getenv("LLM_GATEWAY_GRPC_ADDR")
	if llmGatewayGRPCAddr == "" {
		llmGatewayGRPCAddr = defaultLLMGatewayGRPCAddr
	}

	llmGatewayGRPCMaxRetries := defaultLLMGatewayGRPCMaxRetries
	if v := os.Getenv("LLM_GATEWAY_GRPC_MAX_RETRIES"); v != "" {
		n, err := strconv.Atoi(v)
		switch {
		case err != nil:
			slog.Default().Warn("invalid LLM_GATEWAY_GRPC_MAX_RETRIES, using default",
				"value", v, "default", defaultLLMGatewayGRPCMaxRetries, "error", err)
		case n < 0:
			// A negative retry count parses successfully but is nonsensical
			// (GRPCClient.callWithRetry would then treat it the same as "at
			// least 1 attempt" via its own clamp, silently ignoring the
			// caller's intent) -- treat it like a parse failure rather than
			// passing it through.
			slog.Default().Warn("invalid LLM_GATEWAY_GRPC_MAX_RETRIES, using default",
				"value", v, "default", defaultLLMGatewayGRPCMaxRetries, "error", "must not be negative")
		default:
			llmGatewayGRPCMaxRetries = n
		}
	}

	llmGatewayGRPCBaseBackoff := parseDurationEnv("LLM_GATEWAY_GRPC_BASE_BACKOFF", defaultLLMGatewayGRPCBaseBackoff)

	hubDriver := os.Getenv("MESSAGE_HUB_DRIVER")
	if hubDriver == "" {
		hubDriver = "inprocess"
	}
	if hubDriver != "inprocess" && hubDriver != "redis" {
		return nil, fmt.Errorf("MESSAGE_HUB_DRIVER must be %q or %q, got %q", "inprocess", "redis", hubDriver)
	}

	redisURL := os.Getenv("REDIS_URL")
	if hubDriver == "redis" && redisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required when MESSAGE_HUB_DRIVER=redis")
	}

	defaultAIModel := os.Getenv("DEFAULT_AI_MODEL")
	if defaultAIModel == "" {
		defaultAIModel = defaultDefaultAIModel
	}

	rateLimitLoginPerMinute := defaultRateLimitLoginPerMinute
	if v := os.Getenv("RATE_LIMIT_LOGIN_PER_MINUTE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Default().Warn("invalid RATE_LIMIT_LOGIN_PER_MINUTE, using default",
				"value", v, "default", defaultRateLimitLoginPerMinute, "error", err)
		} else {
			rateLimitLoginPerMinute = n
		}
	}

	rateLimitAIInvokePerMinute := defaultRateLimitAIInvokePerMinute
	if v := os.Getenv("RATE_LIMIT_AI_INVOKE_PER_MINUTE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Default().Warn("invalid RATE_LIMIT_AI_INVOKE_PER_MINUTE, using default",
				"value", v, "default", defaultRateLimitAIInvokePerMinute, "error", err)
		} else {
			rateLimitAIInvokePerMinute = n
		}
	}

	whoamiCacheTTL := parseDurationEnv("WHOAMI_CACHE_TTL", defaultWhoamiCacheTTL)

	return &Config{
		Port:                port,
		DatabaseURL:         dbURL,
		JWTSecret:           jwtSecret,
		LLMGatewayURL:       llmURL,
		CORSOrigins:         corsOrigins,
		DBMaxConnLifetime:   dbMaxConnLifetime,
		DBMaxConnIdleTime:   dbMaxConnIdleTime,
		DBHealthCheckPeriod: dbHealthCheckPeriod,
		S3Endpoint:          s3Endpoint,
		S3Region:            s3Region,
		S3Bucket:            s3Bucket,
		S3AccessKey:         s3AccessKey,
		S3SecretKey:         s3SecretKey,
		S3ForcePathStyle:    s3ForcePathStyle,
		WSTicketSecret:      wsTicketSecret,
		AuthMode:            authMode,
		KratosPublicURL:     kratosPublicURL,
		KratosAdminURL:      kratosAdminURL,
		KratosCookieName:    kratosCookieName,

		LLMGatewayTransport:       llmGatewayTransport,
		LLMGatewayGRPCAddr:        llmGatewayGRPCAddr,
		LLMGatewayGRPCMaxRetries:  llmGatewayGRPCMaxRetries,
		LLMGatewayGRPCBaseBackoff: llmGatewayGRPCBaseBackoff,

		RedisURL:         redisURL,
		MessageHubDriver: hubDriver,

		DefaultAIModel: defaultAIModel,

		RateLimitLoginPerMinute:    rateLimitLoginPerMinute,
		RateLimitAIInvokePerMinute: rateLimitAIInvokePerMinute,
		WhoamiCacheTTL:             whoamiCacheTTL,
	}, nil
}

// parseCORSOrigins splits raw (a comma-separated list, matching the
// CORS_ORIGINS format app/router.go splits on "," when building its
// AllowOrigins list) into origins, trimming surrounding whitespace from each
// entry and dropping any that are empty after trimming (e.g. from a trailing
// comma, or accidental double commas). The result is joined back into a
// comma-separated string with no surrounding whitespace, which
// app/router.go's own strings.Split(..., ",") then splits back into a clean
// origin list — so a value like " https://a.com , https://b.com " round-trips
// to "https://a.com,https://b.com".
//
// It returns an error if, after trimming, any entry is a literal "*":
// app/router.go always sets AllowCredentials: true on the CORS middleware
// (required so the browser can send/receive the Kratos session cookie), and
// browsers refuse to honor Access-Control-Allow-Origin: * together with
// Access-Control-Allow-Credentials: true — so a "*" here would not just be
// an overly permissive origin list, it would silently break every
// credentialed cross-origin request. Fail fast at startup rather than as a
// hard-to-diagnose CORS error in the browser.
func parseCORSOrigins(raw string) (string, error) {
	parts := strings.Split(raw, ",")
	trimmed := make([]string, 0, len(parts))
	for _, origin := range parts {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			return "", fmt.Errorf(`CORS_ORIGINS must not contain "*" when credentials are enabled, got %q`, raw)
		}
		trimmed = append(trimmed, origin)
	}
	return strings.Join(trimmed, ","), nil
}

// parseDurationEnv reads the given environment variable and parses it as a
// time.Duration. If the variable is unset, it returns fallback. If the
// variable is set but fails to parse, or parses to a non-positive duration
// (which is not a meaningful pool tuning value), it logs a warning via
// slog.Default() and returns fallback rather than propagating an error,
// since these settings are optional tuning knobs, not required configuration.
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

	if d <= 0 {
		slog.Default().Warn("non-positive duration for env var, using default",
			"env", key, "value", val, "default", fallback)
		return fallback
	}

	return d
}

// parseBoolEnv reads the given environment variable and parses it with
// strconv.ParseBool (accepting "1", "t", "T", "TRUE", "true", "True", "0",
// "f", "F", "FALSE", "false", "False"). If the variable is unset, it returns
// fallback. If the variable is set but fails to parse, it logs a warning via
// slog.Default() and returns fallback rather than propagating an error,
// matching this file's convention for optional tuning knobs (see
// parseDurationEnv) rather than a required, startup-failing configuration.
func parseBoolEnv(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}

	b, err := strconv.ParseBool(val)
	if err != nil {
		slog.Default().Warn("invalid boolean for env var, using default",
			"env", key, "value", val, "default", fallback, "error", err)
		return fallback
	}

	return b
}
