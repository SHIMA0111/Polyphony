// Package auth provides concrete implementations of the domain/auth
// AuthService port: SimpleJWTService (self-hosted argon2+JWT), KratosAuthService
// (backed by Ory Kratos's self-service flows), and CachedAuthService (a
// Redis-caching decorator that wraps either of the other two).
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
)

// whoamiCacheKeyPrefix namespaces CachedAuthService's Redis keys from any
// other use of the shared *redis.Client (e.g. Step 33's rate limiter, or
// Step 31's RedisHub Pub/Sub channels), so a `redis-cli KEYS` scan can
// distinguish them at a glance.
const whoamiCacheKeyPrefix = "whoami:"

// CachedAuthService wraps any domainauth.AuthService and adds a short-TTL
// Redis cache in front of its ValidateToken calls, so that once
// AUTH_MODE=kratos makes ValidateToken a real HTTP round trip to Kratos's
// /sessions/whoami endpoint (see KratosAuthService.ValidateToken), most
// authenticated requests no longer pay that network cost. Register and
// Login are not cached — they are not hot, repeated-per-request calls, so
// there is nothing to gain from caching them, and doing so would only add
// staleness risk for no benefit.
//
// CachedAuthService also implements domainauth.Revoker regardless of
// whether inner does: Revoke always purges the cache entry for the given
// token (so a stale cache hit can never outlive an explicit logout), and
// additionally delegates to inner's own Revoke if inner implements
// domainauth.Revoker (e.g. KratosAuthService, to revoke the underlying
// session too — see Revoke's GoDoc for why both steps matter).
type CachedAuthService struct {
	inner       domainauth.AuthService
	redisClient *redis.Client
	ttl         time.Duration
}

// NewCachedAuthService creates a CachedAuthService wrapping inner. ttl is the
// lifetime given to a cached ValidateToken result (see Config.WhoamiCacheTTL);
// redisClient must be a non-nil, already-configured client (typically the one
// shared with the rate limiter and, when MESSAGE_HUB_DRIVER=redis, RedisHub —
// see container.go, which never opens a second Redis connection pool for
// this purpose).
func NewCachedAuthService(inner domainauth.AuthService, redisClient *redis.Client, ttl time.Duration) *CachedAuthService {
	return &CachedAuthService{inner: inner, redisClient: redisClient, ttl: ttl}
}

// Register delegates to inner unchanged; see the type's GoDoc for why
// Register/Login are not cached.
func (c *CachedAuthService) Register(ctx context.Context, email, username, password string) (*domainauth.TokenPair, error) {
	return c.inner.Register(ctx, email, username, password)
}

// Login delegates to inner unchanged; see the type's GoDoc for why
// Register/Login are not cached.
func (c *CachedAuthService) Login(ctx context.Context, email, password string) (*domainauth.TokenPair, error) {
	return c.inner.Login(ctx, email, password)
}

// ValidateToken returns the cached domainauth.Claims for token if present in
// Redis, otherwise falls through to inner.ValidateToken and, on success,
// caches the result for ttl.
//
// The Redis key is "whoami:" followed by the hex-encoded SHA-256 digest of
// token, never the raw token itself — a bearer token or Kratos session
// cookie value is a bearer credential, and writing it as a plaintext Redis
// key would make it recoverable from `redis-cli KEYS`/`MONITOR`/slow-log
// output; hashing it makes the key one-way while still letting repeated
// calls with the same token hit the same cache entry.
//
// Any Redis error while reading the cache (including redis.Nil on a miss) is
// treated as a cache miss and falls through to inner — this includes
// transport-level errors (e.g. Redis unreachable), so a Redis outage
// degrades ValidateToken to "no caching" rather than failing the request
// (the same fail-open principle documented on middleware.RateLimit). Such
// fail-open occurrences are logged at Warn via slog.Default(), since
// CachedAuthService has no Echo context to pull a request-scoped logger
// from.
//
// A successful inner.ValidateToken result is cached with `SET key <json> EX
// ttl`; a failed one is never cached — caching a transient Kratos hiccup (or
// any other transient error) would pin a false rejection for the whole TTL
// window, turning a momentary blip into an extended outage for that token.
// The cache write itself is best-effort: a failure to write is logged and
// ignored, never surfaced as a ValidateToken error, since the request has
// already been correctly validated by inner at that point.
func (c *CachedAuthService) ValidateToken(ctx context.Context, token string) (*domainauth.Claims, error) {
	key := whoamiCacheKey(token)

	if c.redisClient != nil {
		cached, err := c.redisClient.Get(ctx, key).Result()
		if err == nil {
			var claims domainauth.Claims
			if unmarshalErr := json.Unmarshal([]byte(cached), &claims); unmarshalErr == nil {
				return &claims, nil
			}
			// A corrupt cache entry is treated the same as a miss below —
			// fall through to inner rather than failing the request over a
			// cache-format problem.
		} else if err != redis.Nil {
			slog.Default().Warn("whoami cache read failed, falling back to inner ValidateToken", "error", err)
		}
	}

	claims, err := c.inner.ValidateToken(ctx, token)
	if err != nil {
		// Never cache an error result — see GoDoc above.
		return nil, err
	}

	if c.redisClient != nil {
		if data, marshalErr := json.Marshal(claims); marshalErr == nil {
			if setErr := c.redisClient.Set(ctx, key, data, c.ttl).Err(); setErr != nil {
				slog.Default().Warn("whoami cache write failed", "error", setErr)
			}
		} else {
			slog.Default().Warn("failed to marshal claims for whoami cache", "error", marshalErr)
		}
	}

	return claims, nil
}

// Revoke deletes the cached entry for token (best-effort — a failure is
// logged and ignored, since the underlying session revocation below is the
// authoritative action) and, if inner implements domainauth.Revoker, also
// calls inner.Revoke and returns its error. If inner does not implement
// domainauth.Revoker (no backend currently wraps a non-Revoker in
// CachedAuthService, but the fallback is documented for completeness), it
// returns nil, mirroring AuthUsecase.Logout's own no-op contract.
func (c *CachedAuthService) Revoke(ctx context.Context, token string) error {
	if c.redisClient != nil {
		if err := c.redisClient.Del(ctx, whoamiCacheKey(token)).Err(); err != nil {
			slog.Default().Warn("whoami cache delete failed during revoke", "error", err)
		}
	}

	if revoker, ok := c.inner.(domainauth.Revoker); ok {
		return revoker.Revoke(ctx, token)
	}
	return nil
}

// whoamiCacheKey computes the Redis key ValidateToken/Revoke use for token —
// see ValidateToken's GoDoc for the key-hashing rationale.
func whoamiCacheKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return whoamiCacheKeyPrefix + hex.EncodeToString(sum[:])
}
