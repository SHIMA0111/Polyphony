// Package middleware provides Echo middleware: JWT authentication and request-scoped structured logging.
package middleware

import (
	"math"
	"net/http"
	"strconv"

	"github.com/go-redis/redis_rate/v10"
	"github.com/labstack/echo/v4"
)

// RateLimitConfig configures RateLimit. Limiter and Limit together determine
// the token-bucket rate applied; KeyPrefix and KeyFunc together determine
// which bucket a given request is charged against.
type RateLimitConfig struct {
	// Limiter is the shared redis_rate.Limiter (built on the same *redis.Client
	// Step 31 already wired into container.go — see Container.RateLimiter) used
	// to check/consume tokens. It must be non-nil.
	Limiter *redis_rate.Limiter
	// Limit is the token-bucket rate (e.g. redis_rate.PerMinute(n)) enforced
	// for every key this middleware instance is applied to.
	Limit redis_rate.Limit
	// KeyPrefix namespaces this middleware instance's Redis keys from any
	// other RateLimit instance sharing the same Limiter/client (e.g. "auth"
	// vs "ai_invoke"), so two independently-configured limits never share a
	// bucket by accident.
	KeyPrefix string
	// KeyFunc extracts the per-request identity (e.g. client IP or
	// authenticated user ID) the rate limit is enforced against. See
	// PerIPKeyFunc and PerUserOrIPKeyFunc.
	KeyFunc func(c echo.Context) string
}

// RateLimit returns an Echo middleware enforcing a Redis-backed GCRA
// token-bucket rate limit (via github.com/go-redis/redis_rate/v10) per the
// given RateLimitConfig.
//
// On each request it computes key := cfg.KeyPrefix + ":" + cfg.KeyFunc(c) and
// calls cfg.Limiter.Allow(ctx, key, cfg.Limit). If the limit is exceeded
// (res.Allowed == 0), it sets the Retry-After response header to the number
// of whole seconds until the bucket next admits a request and returns HTTP
// 429 with the package-local errorResponse{Message: "rate limit exceeded"}
// body (the same shape JWTAuth's 401 responses use).
//
// If cfg.Limiter.Allow itself returns an error (e.g. Redis is unreachable),
// RateLimit fails open: it logs the error at Warn via GetLogger(c) and calls
// next(c) as if the request were allowed. This is a deliberate
// availability-over-strictness tradeoff — a Redis outage must not take down
// login or AI invocation, only remove the rate-limiting protection until
// Redis recovers, mirroring the whoami cache's identical fail-open
// philosophy (see interface/auth.CachedAuthService).
//
// If cfg.Limiter is nil (container.go never constructs one when no Redis
// client is configured at all — see Container.RedisClient's GoDoc), RateLimit
// also fails open by calling next(c) directly, without logging: unlike a
// transient Redis error, a nil Limiter reflects a static deployment choice
// (no Redis configured), so logging on every single request would just be
// noise rather than an actionable signal.
func RateLimit(cfg RateLimitConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if cfg.Limiter == nil {
				return next(c)
			}

			key := cfg.KeyPrefix + ":" + cfg.KeyFunc(c)

			res, err := cfg.Limiter.Allow(c.Request().Context(), key, cfg.Limit)
			if err != nil {
				GetLogger(c).Warn("rate limiter unavailable, failing open", "error", err, "key_prefix", cfg.KeyPrefix)
				return next(c)
			}

			if res.Allowed == 0 {
				// Round up to a whole second and clamp to a minimum of 1:
				// truncating toward zero (int(...)) would report
				// "Retry-After: 0" for any sub-second window, which tells
				// the client it may retry immediately even though it is
				// still rate-limited.
				retryAfterSeconds := int(math.Ceil(res.RetryAfter.Seconds()))
				if retryAfterSeconds < 1 {
					retryAfterSeconds = 1
				}
				c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
				return c.JSON(http.StatusTooManyRequests, errorResponse{Message: "rate limit exceeded"})
			}

			return next(c)
		}
	}
}

// PerIPKeyFunc is a RateLimitConfig.KeyFunc that keys the rate limit by the
// requester's IP (via echo.Context.RealIP). It is intended for
// unauthenticated routes (POST /auth/register, POST /auth/login) where no
// authenticated user ID is available yet.
func PerIPKeyFunc(c echo.Context) string {
	return c.RealIP()
}

// PerUserOrIPKeyFunc is a RateLimitConfig.KeyFunc that keys the rate limit by
// the authenticated user ID (via GetUserID) when present, falling back to
// the requester's IP (via echo.Context.RealIP) otherwise. It is intended for
// the AI-invoke routes, which always run behind JWTAuth and so always have a
// populated user ID in practice; the IP fallback only matters if this
// KeyFunc is ever reused ahead of JWTAuth, so a request is never left
// entirely unkeyed.
func PerUserOrIPKeyFunc(c echo.Context) string {
	if userID := GetUserID(c); userID != "" {
		return userID
	}
	return c.RealIP()
}
