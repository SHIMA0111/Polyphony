# Phase 10: Redis + Multi-instance

**Goal**: Introduce Redis for WebSocket Pub/Sub, rate limiting, and session caching.

**Delivered by**: `docs/tasks/step31.md` (Redis in compose + RedisHub MessageHub swap), `docs/tasks/step33.md`
(Server: Redis rate limiting + Kratos session cache). See those files for the full per-file implementation notes.

---

## Step 31: Redis in compose + RedisHub MessageHub swap

- [x] `github.com/redis/go-redis/v9` added; `server/internal/infrastructure/event/redis_hub.go` — `RedisHub`
      implementing `domain/event.MessageHub` (Step 7's interface), one Pub/Sub channel per room, per-user
      client-side filtering mirroring `InProcessHub`'s `TargetUserIDs` semantics
- [x] `Config.MessageHubDriver` (`inprocess` default | `redis`), `RedisURL` (required only when `redis` selected)
- [x] DI switch in `container.go`: `RedisHub` vs `InProcessHub` behind the same `event.MessageHub` field; shared
      `*redis.Client` kept on `Container` for Step 33 to reuse
- [x] `docker-compose.yml`: healthchecked `redis:7-alpine` service; `api`'s `REDIS_URL`/`MESSAGE_HUB_DRIVER`
      defaulted to `redis` in compose
- [x] `docker-compose.scale-test.yml` — dev-only override for multi-replica verification; `task verify:redis-hub`
      scripted smoke check (register → WS ticket → open WS on replica 1 → send via replica 2 → assert delivery)
- [x] Unit tests (`config_test.go`: `MESSAGE_HUB_DRIVER=redis` without `REDIS_URL` errors); testcontainers
      `redis_hub_integration_test.go` (two `RedisHub` instances, cross-instance delivery + per-user targeting)

## Step 33: Redis rate limiting + Kratos session cache

- [x] `domainauth.Revoker` optional capability interface (`Revoke(ctx, token) error`); `KratosAuthService.Revoke`
      hits Kratos's `self-service/logout/api`, idempotent on already-gone sessions
- [x] `server/internal/interface/auth/cached.go` — `CachedAuthService` wraps any `AuthService`, caches
      `ValidateToken` results in Redis for `WHOAMI_CACHE_TTL`
- [x] `AuthUsecase.Logout` type-asserts `Revoker`, no-ops for backends without server-side revocation
      (`SimpleJWTService`)
- [x] `middleware.GetToken` accessor; `AuthHandler.Logout` (`POST /auth/logout`)
- [x] `server/internal/interface/middleware/rate_limit.go` — Redis token-bucket (GCRA, `go-redis/redis_rate`)
      middleware; wired onto `POST /auth/register`/`/auth/login` (per-IP) and the AI-invoke routes (per-user)
- [x] `Config.RateLimitLoginPerMinute` (10), `RateLimitAIInvokePerMinute` (20), `WhoamiCacheTTL` (30s); wired into
      `docker-compose.yml`'s `api` and `api-e2e` (much looser E2E-profile defaults so Playwright never trips 429)
- [x] `.env.example`: `# === Rate limiting & session cache (Phase 10) ===` section

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...` (includes
      `redis_hub_integration_test.go` via testcontainers' Redis module)
- [ ] `task verify:redis-hub` (multi-replica WebSocket smoke check) — requires the compose stack with a real
      scaled `api` service; skipped (post-merge integration review)
