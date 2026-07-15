# Step 31: Redis in compose + RedisHub MessageHub swap

## Meta
- **Type**: feature
- **Components**: server, infra
- **Wave**: 4 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 6: Infra tooling baseline: compose healthchecks, Taskfile, env staging, migration convention; Step 15: Server WebSocket endpoint + realtime delivery
- **Unlocks**: Step 33: Server: Redis rate limiting + Kratos session cache; Step 57: E2E regression: realtime AI core — send, model select, context controls, rate limiting
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Step 15 lands the WebSocket endpoint and the `MessageHub` abstraction with an `InProcessHub` implementation, which only works correctly when a single API process holds all WebSocket connections — this is explicitly called out as excluded in `phases.md` Phase 2 ("Multi-instance support (Phase 10)"). Phase 10 of `phases.md` requires introducing Redis so message delivery, rate limiting, and session caching all survive running more than one API replica. This step delivers the `MessageHub` half of Phase 10: a Redis Pub/Sub-backed implementation that is a drop-in replacement for `InProcessHub`, proven locally by scaling the `api` service to two replicas and observing that a message published on one instance is delivered over a WebSocket connection held by another instance. Rate limiting and session caching (the remaining Phase 10 items) are deliberately left to Step 33, which depends on this step's Redis service and client wiring.

## Goal
After this PR, the API server can run with `MESSAGE_HUB_DRIVER=redis`, in which case `MessageHub` is backed by a Redis Pub/Sub implementation (`RedisHub`) that preserves the exact per-room and per-user delivery semantics of `InProcessHub`. `docker-compose.yml` gains a healthchecked `redis:7` service and the `api` service is wired with a `REDIS_URL`. The swap is a pure dependency-injection change in the Step 1 DI container (`server/internal/app/container.go`) driven by an env flag — no handler, usecase, or WebSocket-endpoint code changes. Running `docker compose up -d --scale api=2` (via a documented override for the static host port) demonstrates that a message sent through one API replica reaches a client whose WebSocket is held open by the other replica, proving cross-instance delivery. `InProcessHub` remains the default for a plain `go run ./cmd/api` without Redis, so single-instance local dev keeps working unchanged.

## Scope
- [x] Read Step 7's and Step 15's actual deliverables before writing any code: `server/internal/domain/event/hub.go` (the `MessageHub` interface, `RoomEvent`, `EventType`) and `server/internal/domain/event/inprocess_hub.go` (the `InProcessHub` implementation — Step 7 deliberately keeps it beside the interface since it has zero external deps), plus wherever Step 1's DI container (`server/internal/app/container.go`) wires the hub into `MessageUsecase` and the WebSocket handler. Mirror the exact interface method set — do not redefine or fork it.
- [x] Add `github.com/redis/go-redis/v9` to `server/go.mod` (`cd server && go get github.com/redis/go-redis/v9 && go mod tidy`).
- [x] Implement `server/internal/infrastructure/event/redis_hub.go` (new package — `RedisHub` cannot live in `domain/event` like `InProcessHub` does, because it imports the go-redis client and the domain layer must stay infrastructure-free per `CLAUDE.md`): a `RedisHub` struct that implements the `domain/event.MessageHub` interface from Step 7 using a single shared `*redis.Client`. Use one Redis Pub/Sub channel per room (e.g. `room:{roomID}`, matching Step 7's `Subscribe(ctx, roomID, userID)` room scoping), publish the serialized `RoomEvent` (JSON) as the channel payload, and preserve per-user targeting by filtering delivered events against the subscribing user's ID inside the Go subscriber goroutine (Redis Pub/Sub has no server-side per-consumer filtering, so the filter must happen client-side, exactly mirroring `InProcessHub`'s `TargetUserIDs` filtering from Step 7). Each `Subscribe` call opens its own `*redis.PubSub` on the room channel and fans messages into the `<-chan RoomEvent` + unsubscribe-func shape Step 7's interface defines; the unsubscribe func closes the pubsub connection and the channel exactly once. Add full GoDoc on `RedisHub` and every exported method, including the delivery/filtering behavior and error conditions (e.g. Redis connection loss).
- [x] Add `RedisURL` (required only when the redis driver is selected) and `MessageHubDriver` fields to `server/internal/infrastructure/config/config.go`, following the existing `Load()` pattern (see `LLMGatewayURL`/`CORSOrigins` for the "env var with default" style). `MESSAGE_HUB_DRIVER` defaults to `"inprocess"` when unset; accepted values are `"inprocess"` and `"redis"`. When `"redis"` is selected, `REDIS_URL` becomes required and `Load()` returns an error if missing (mirror the `DATABASE_URL`/`JWT_SECRET` required-var pattern).
- [x] Wire the swap in `server/internal/app/container.go` (Step 1's DI container): construct `redis.NewClient` from `REDIS_URL` and instantiate `infraevent.NewRedisHub(...)` when `cfg.MessageHubDriver == "redis"`, otherwise keep constructing `event.NewInProcessHub()` as Step 7 does today. Both branches must satisfy the same `event.MessageHub` interface field so the rest of the DI graph (the WebSocket handler, message usecase, etc.) is untouched — this should be a small `if`/`switch` block, not a restructuring of the container. Keep the shared `*redis.Client` on the `Container` so Step 33 can reuse it.
- [x] Add a healthchecked `redis:7-alpine` service to `docker-compose.yml`: standard `redis-cli ping` healthcheck, no host port publish required (internal-only, consumed via the compose network as `redis:6379`). Set `api`'s `REDIS_URL` to `redis://redis:6379/0` and `MESSAGE_HUB_DRIVER` to `redis` by default in compose, and make `api` `depends_on: redis: condition: service_healthy` in addition to its existing `migrate` dependency. Add both new env vars (with the same defaults) to `.env.example` under the `Go API Server` section. Follow the alphabetized/additive service-block convention established by Step 6 (insert the `redis` block in alphabetical position among the existing services) — do not reorder or reformat unrelated service blocks.
- [x] Add a dev-only compose override, `docker-compose.scale-test.yml`, that changes only the `api` service's `ports` entry from the fixed `"8080:8080"` host mapping to a bare container-port form (`"8080"`), letting Docker assign a distinct random host port per replica. This file is committed (it is needed for the objective verification below and for anyone reproducing it later) but is never used by `task up`/default `docker compose up` — only referenced explicitly with `-f`.
- [x] Add a `task verify:redis-hub` Taskfile target (or a plain shell script at `server/scripts/verify_redis_hub.sh` invoked by that target — pick whichever fits the existing `Taskfile.yml` style) that scripts the multi-instance smoke check described in Verification below: bring up `db`, `migrate`, `redis`, `llm-gateway`, scale `api` to 2 replicas using the override file, resolve each replica's host port via `docker compose port`, register/login a test user, obtain a WS ticket, open a WebSocket to replica 1, send a message via HTTP against replica 2, and assert the message arrives on the replica-1 WebSocket within a timeout. Exit non-zero on failure/timeout. (Script written and reviewed; not executed end-to-end in this worktree — see skippedComposeChecks.)
- [x] Unit tests for `RedisHub` construction/config validation (e.g. `Load()` rejects `MESSAGE_HUB_DRIVER=redis` without `REDIS_URL`) using the project's existing hand-written-mock test style (see `server/internal/infrastructure/config` — add a `config_test.go` if one does not already exist, or extend it).
- [x] Integration test `server/internal/infrastructure/event/redis_hub_integration_test.go` using `testcontainers-go`'s Redis module (mirror the testcontainers setup/build-tag/naming convention Step 2 established for the project's Postgres integration tests — same module version, same `//go:build integration`-style gating if one exists, same `TestMain`/lifecycle helpers if a shared harness exists under `server/internal/infrastructure` or `server/internal/interface/repository/postgres`). The test must: (1) start two independent `RedisHub` instances pointed at the same testcontainers Redis (simulating two API replicas), (2) subscribe instance A to a room, (3) publish an event through instance B, (4) assert instance A receives it within a short timeout, and (5) assert per-user targeting — an event with `TargetUserIDs` set is delivered only to a subscription for a listed user and not to a differently-targeted subscription on the same room.
- [x] Update the `README.md`/`README-ja.md` Quick Start comment line ("Start all services (PostgreSQL, Go API, LLM Gateway, Web Frontend)") to mention Redis, matching the additive/minor-touch convention for these shared docs.

## Out of scope
- Rate limiting middleware (Redis Token Bucket) — Step 33.
- Session cache for Kratos session verification — Step 33 (Kratos itself isn't wired until Step 20/30 in this plan's numbering; this step only stands up the Redis service and client wiring that Step 33 will reuse).
- Any change to the WebSocket handler's HTTP surface, the WS ticket issuance endpoint, or the `Event`/`Subscription` types themselves — those are frozen by Step 15; this step only swaps the `MessageHub` implementation behind the existing interface.
- Redis Cluster / HA configuration (explicitly excluded through Phase 21 per `phases.md`).
- Making `docker-compose.scale-test.yml`'s port behavior the default for `docker-compose.yml` — the main compose file keeps its fixed `8080:8080` mapping for normal single-instance local dev.
- Any change to `schema.sql` or Atlas migrations — this step has no DB changes.

## Implementation notes
- **Interface contract**: this step must not invent a new `MessageHub` shape. Read `server/internal/domain/event/` (Step 7's `hub.go` + `inprocess_hub.go`, as consumed by Step 15's WebSocket handler) first and implement `RedisHub` against the exact same interface, including the `RoomEvent`/`Subscribe(ctx, roomID, userID string) (<-chan RoomEvent, func())` shapes Step 7 defines. If the actual method names differ slightly from the sketch below, follow the real code — this doc's snippets are the expected shape, not the source of truth.
- **Expected shape** (for orientation only — this matches Step 7's spec; confirm against the merged code):
  ```go
  package event

  type EventType string

  type RoomEvent struct {
      Type          EventType
      RoomID        string
      Message       *message.Message
      TargetUserIDs []string // nil/empty = all subscribers of RoomID
      OccurredAt    time.Time
  }

  type MessageHub interface {
      // Publish is best-effort and never fails the caller (no error return).
      Publish(ctx context.Context, event RoomEvent)
      // Subscribe returns a receive channel plus an unsubscribe/cleanup func.
      Subscribe(ctx context.Context, roomID, userID string) (<-chan RoomEvent, func())
  }
  ```
- **RedisHub file**: `server/internal/infrastructure/event/redis_hub.go`, a new `infraevent` package importing `server/internal/domain/event` (the domain package keeps `InProcessHub`; the Redis implementation lives in infrastructure because of its go-redis dependency). Constructor `NewRedisHub(client *redis.Client) *RedisHub`. `Publish` does `client.Publish(ctx, "room:"+ev.RoomID, jsonBytes)` (best-effort: log on error, never fail the caller — matching Step 7's no-error `Publish` contract). `Subscribe` does `client.Subscribe(ctx, "room:"+roomID)`, spawns a goroutine reading `pubsub.Channel()`, unmarshals each payload into `RoomEvent`, and forwards to the output channel only if `len(ev.TargetUserIDs) == 0` or the list contains `userID`. The returned unsubscribe func closes the underlying `*redis.PubSub` and the output channel, idempotently/safe against double-close.
- **Config**: `server/internal/infrastructure/config/config.go` — add fields following the exact style already there:
  ```go
  // RedisURL is the Redis connection string, required when MessageHubDriver is "redis".
  RedisURL string
  // MessageHubDriver selects the MessageHub implementation: "inprocess" (default) or "redis".
  MessageHubDriver string
  ```
  and in `Load()`:
  ```go
  hubDriver := os.Getenv("MESSAGE_HUB_DRIVER")
  if hubDriver == "" {
      hubDriver = "inprocess"
  }
  redisURL := os.Getenv("REDIS_URL")
  if hubDriver == "redis" && redisURL == "" {
      return nil, fmt.Errorf("REDIS_URL is required when MESSAGE_HUB_DRIVER=redis")
  }
  ```
- **Container wiring**: in `server/internal/app/container.go`, near where Step 7 constructs the hub, add:
  ```go
  var hub event.MessageHub
  switch cfg.MessageHubDriver {
  case "redis":
      redisClient := redis.NewClient(&redis.Options{Addr: ...}) // or redis.ParseURL(cfg.RedisURL)
      hub = infraevent.NewRedisHub(redisClient)
  default:
      hub = infraevent.NewInProcessHub()
  }
  ```
  Use an `infraevent` import alias for the new infrastructure package alongside the existing `domain/event` import. Log the selected driver via `slog.Info("message hub driver selected", "driver", cfg.MessageHubDriver)` to match the existing structured-logging style.
- **docker-compose.yml**: add the `redis` block alphabetically (after `migrate`, before `web`, matching Step 6's alphabetized-block convention):
  ```yaml
  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5
  ```
  and extend `api`'s `environment`/`depends_on`:
  ```yaml
  environment:
    ...
    REDIS_URL: ${REDIS_URL:-redis://redis:6379/0}
    MESSAGE_HUB_DRIVER: ${MESSAGE_HUB_DRIVER:-redis}
  depends_on:
    migrate:
      condition: service_completed_successfully
    redis:
      condition: service_healthy
  ```
  Add matching `REDIS_URL=redis://redis:6379/0` and `MESSAGE_HUB_DRIVER=redis` lines to `.env.example` under `# === Go API Server ===`.
- **Scale-test override**: `docker-compose.scale-test.yml` at repo root:
  ```yaml
  services:
    api:
      ports:
        - "8080"
  ```
  This is the entire file — Compose merges it over the base file, replacing only the `ports` key so multiple `api` replicas can each get an auto-assigned host port instead of colliding on `8080`.
- **conflictNotes**: this PR only adds new, self-contained blocks to `docker-compose.yml` and `.env.example` (new `redis` service block, new `api` env/depends_on keys) — do not reformat or reorder existing service blocks beyond inserting the new one alphabetically. The `MessageHub` interface itself is frozen by Step 7 (and consumed by Step 15); this step must not modify `server/internal/domain/event/` type definitions, only add a new implementation under `server/internal/infrastructure/event/`.
- Follow GoDoc conventions from `CLAUDE.md` on every new exported symbol (`RedisHub`, `NewRedisHub`, new `Config` fields, etc.) — describe what/why/constraints/error behavior, matching the existing style in `server/internal/infrastructure/config/config.go` and `server/internal/interface/middleware/auth.go`.
- All new code, comments, and doc updates in English per `CLAUDE.md`.

## Verification
1. [x] `cd server && go build ./... && go vet ./...` — compiles clean with the new `redis` dependency and config fields.
2. [x] `cd server && go test ./...` — all existing unit tests plus the new `config_test.go`/`RedisHub` unit tests pass.
3. [x] `cd server && go test -tags=integration ./internal/infrastructure/event/...` (adjust the build tag to match the convention Step 2 established for Postgres integration tests) — the two-instance Redis Pub/Sub integration test passes, including the per-user targeting assertion.
4. [x] `docker compose up -d db migrate redis llm-gateway` then `docker compose ps` — `redis` shows `healthy`. (Verified in the wave-4 integration review.)
5. [x] `docker compose up -d api` (single instance, default compose) then `curl -s http://localhost:8080/health` returns 200, and `docker compose logs api | grep "message hub driver selected"` shows `driver=redis`. (Verified in the wave-4 integration review.)
6. [x] Multi-instance cross-delivery check via `task verify:redis-hub` against a real two-replica compose stack. (Wave-4 review round 2: passes end to end as committed — the round-1 fixes (`ports: !override` in `docker-compose.scale-test.yml`; `--wait` only on long-running services + `docker compose run --rm migrate` + api-scoped cleanup trap in `verify_redis_hub.sh`) hold up: two replicas came up on auto-assigned host ports, a message POSTed to replica 2 was delivered over replica 1's WebSocket via RedisHub, exit 0, and the cleanup removed only the api replicas, leaving the rest of the running dev stack untouched.)
7. [x] Restart with `MESSAGE_HUB_DRIVER=inprocess docker compose up -d api` ... confirming the default/no-Redis path still works. (Wave-4 review round 2: verified — api restarted healthy, `/health` returned 200, and the api log shows `message hub driver selected driver=inprocess`; restarting again without the override restored `driver=redis`.)
8. [x] `task test` and `task lint` at the repo root still pass end-to-end. (Verified in the wave-4 integration review: both pass — lint reports 0 errors, 4 pre-existing/new react-hooks warnings on the web side.)

## Completion criteria
- [x] `RedisHub` implements the Step 15 `MessageHub` interface with per-room channels and per-user (`TargetUserID`) filtering.
- [x] `MESSAGE_HUB_DRIVER` env flag selects between `InProcessHub` (default) and `RedisHub` with no other code path changes.
- [x] `docker-compose.yml` has a healthchecked `redis:7-alpine` service and `api` depends on it when the redis driver is the compose default; `.env.example` documents the new vars.
- [x] `docker-compose.scale-test.yml` override exists and enables `--scale api=2` without host-port collisions.
- [x] Unit tests (config validation, RedisHub construction) and the testcontainers-backed integration test (two-instance pub/sub + per-user targeting) are added and pass.
- [x] The multi-instance WebSocket delivery smoke check (`task verify:redis-hub` or equivalent script) passes against a real two-replica compose stack. (Wave-4 review round 2: executed and passed, exit 0 — see Verification item 6.)
- [x] All items in the Verification section pass. (Wave-4 review round 2: items 1-8 all verified passing.)
- [x] No changes to `schema.sql`, Atlas migrations, or the `event.MessageHub`/`Event` type definitions themselves.
