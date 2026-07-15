# Step 6: Infra tooling baseline: compose healthchecks, Taskfile, env staging, migration convention

## Meta
- **Type**: infra
- **Components**: infra, docs
- **Wave**: 1 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: None
- **Unlocks**: Step 10: Playwright E2E harness + seeded compose test stack + LLM stub | Step 11: Infra: Ory Kratos services in docker-compose | Step 12: Server: MinIO object storage + attachments + presigned upload endpoints | Step 31: Redis in compose + RedisHub MessageHub swap
- **Size**: 1 PR (a few hours for one AI agent)

## Context
The current `docker-compose.yml` and `Taskfile.yml` (repo root) work for the Phase-1 slice but are not ready to carry the next ~19 phases of work: `web` only waits for `api` to *start* (`depends_on: - api`, no health condition), `api` never waits for `llm-gateway`, and neither `api` nor `llm-gateway` declare a `healthcheck` even though both already expose a `GET /health` endpoint (`server/internal/interface/handler/health_handler.go`, `llm-gateway/src/adapters/inbound/rest/handlers.rs`). `task up` silently runs `migrate:generate` on every invocation (`deps: [migrate:generate]` in `Taskfile.yml`), which is surprising and will conflict once multiple PRs touch `schema.sql` in parallel per-step migrations. There is no lint step to catch destructive/unsafe migrations before `atlas migrate apply` runs against the real dev database, no fallback env file for engineers who run `go run ./cmd/api`, `cargo run`, or `bun run dev` directly on the host (where `db`/`llm-gateway` container hostnames don't resolve), and `.env.example` has no placeholders for the providers/services that land in later phases (Anthropic, Gemini, Stripe, MinIO, Redis, Kratos, Hydra, Dex). This step hardens all of that and writes down the migration- and compose-editing conventions so that every later infra-touching step (10, 11, 12, 31, 44, 49, 55) can extend the same two files additively without merge conflicts.

## Goal
After this PR, `docker compose up -d` brings up a fully health-gated dependency chain (`db` → `migrate` → `api` ⟷ `llm-gateway` → `web`), `task migrate:generate` is a deliberate, explicit action never triggered as a side effect of `task up`, `task migrate:lint` exists to catch unsafe schema changes before apply, `task rebuild` exists for a clean image rebuild + restart, host-run dev tasks (`task dev:server`, `task dev:gateway`, `task dev:web`) work against `localhost` via an optional `.env.local` overlay, `.env.example` carries commented placeholder blocks for every provider/service phases 8/9/12/15/16-17/31/44 will need, and `docs/infra-conventions.md` documents the one-migration-per-PR Atlas workflow plus the alphabetical/additive editing convention for `docker-compose.yml` and `Taskfile.yml` that all later infra steps must follow.

## Scope
- [x] `docker-compose.yml`: add a `healthcheck` block to the `api` service using the existing `GET /health` endpoint (`wget --spider`, since the runtime image is `alpine:3.21` with only `ca-certificates tzdata` installed — no `curl` — but busybox `wget` is present by default).
- [x] `docker-compose.yml`: add an equivalent `healthcheck` block to the `llm-gateway` service using its existing `GET /health` endpoint (same `wget --spider` pattern; its runtime image is also plain `alpine:3.21`).
- [x] `docker-compose.yml`: change `api`'s `depends_on` to a health-gated chain: keep `migrate: condition: service_completed_successfully`, and add `llm-gateway: condition: service_healthy`.
- [x] `docker-compose.yml`: change `web`'s `depends_on` from the short list form (`- api`) to the map form `api: condition: service_healthy`.
- [x] `docker-compose.yml`: leave `db`'s existing `healthcheck` (`pg_isready`) and `migrate`'s existing `depends_on: db: condition: service_healthy` untouched — they already follow the target convention.
- [x] `docker-compose.yml`: reorder the top-level `services:` block alphabetically (`api`, `db`, `llm-gateway`, `migrate`, `web`) and note in a comment above `services:` that this ordering is a project convention — establishes the "additive alphabetical service blocks" rule later steps (10/11/12/31/44/49/55) must follow when they insert `dex`, `hydra`, `kratos`, `minio`, `redis`, etc.
- [x] `Taskfile.yml`: add a top-level `dotenv: ['.env.local', '.env']` key (go-task gives precedence to *earlier* entries in the list, so `.env.local` must be listed first for its values to override `.env`; `.env.local` is optional and already covered by the existing `.env.*` gitignore pattern) so every task — Docker-based and host-run alike — has env vars available without manual `export`.
- [x] `Taskfile.yml`: remove `deps: [migrate:generate]` from the `up` task. `task up` becomes purely `docker compose up -d`; migration SQL generation becomes an explicit, opt-in step the developer runs deliberately (`task migrate:generate -- <name>`) when `server/schema.sql` has changed.
- [x] `Taskfile.yml`: add a `migrate:lint` task under the "Migration" section: `dir: server`, running `atlas migrate lint --env local --latest 1` (reuses the `local` env already declared in `server/atlas.hcl`, which defines `dev = "docker://postgres/17/dev?search_path=public"` and `migration.dir = "file://migrations"`) to catch destructive/unsafe changes in the most recently generated migration before it is committed or applied.
- [x] `Taskfile.yml`: add a `rebuild` task under the "Docker" section: `docker compose build --no-cache` followed by `docker compose up -d --force-recreate`, for when a clean image rebuild is needed (dependency bumps, Dockerfile changes) without wiping data volumes.
- [x] Create `.env.local.example` at the repo root: a template documented with the exact overrides needed to run `task dev:server` / `task dev:gateway` / `task dev:web` on the host instead of in Docker (`DATABASE_URL` pointed at `localhost:5432` instead of `db:5432`, `LLM_GATEWAY_URL` pointed at `localhost:8081` instead of `llm-gateway:8081`, `NEXT_PUBLIC_API_URL` pointed at `localhost:8080`), with a header comment explaining `cp .env.local.example .env.local` and that the real file is gitignored.
- [x] `.gitignore`: add `!.env.local.example` immediately after the existing `!.env.example` line so the new template is trackable despite the blanket `.env.*` ignore rule.
- [x] `.env.example`: append new commented section blocks (values only — no real secrets) for every provider/service later phases need, matching the existing `# === Section ===` header style already used for PostgreSQL/API/Gateway/Web:
  - `# === Anthropic (Phase 8) ===` → `ANTHROPIC_API_KEY=sk-ant-your-anthropic-api-key`
  - `# === Google Gemini (Phase 8) ===` → `GEMINI_API_KEY=your-gemini-api-key`
  - `# === Stripe (Phase 16-17, test mode) ===` → `STRIPE_SECRET_KEY=sk_test_your-stripe-secret-key`, `STRIPE_WEBHOOK_SECRET=whsec_your-stripe-webhook-secret`
  - `# === MinIO / S3-compatible storage (Phase 12) ===` → `MINIO_ROOT_USER=polyphony`, `MINIO_ROOT_PASSWORD=polyphony-minio`, `MINIO_ENDPOINT=http://minio:9000`, `MINIO_BUCKET=polyphony-attachments`
  - `# === Redis (Phase 10) ===` → `REDIS_URL=redis://redis:6379`
  - `# === Ory Kratos (Phase 9) ===` → `KRATOS_PUBLIC_URL=http://kratos:4433`, `KRATOS_ADMIN_URL=http://kratos:4434`
  - `# === Ory Hydra (Phase 15) ===` → `HYDRA_PUBLIC_URL=http://hydra:4444`, `HYDRA_ADMIN_URL=http://hydra:4445`
  - `# === Dex mock OIDC provider (local social-login testing) ===` → `DEX_ISSUER_URL=http://dex:5556/dex`, `DEX_CLIENT_ID=polyphony-web`, `DEX_CLIENT_SECRET=dex-client-secret-change-me`
  - These are placeholders only — no new compose services are added in this step (Kratos/Hydra/Dex/MinIO/Redis containers land in steps 11/12/31/44/55 respectively).
- [x] Create `docs/infra-conventions.md` documenting: (a) the one-Atlas-migration-per-PR rule with a descriptive migration name (`task migrate:generate -- add_room_invitations`, never the default `auto_<timestamp>` name in a real PR); (b) the post-rebase conflict-resolution recipe for `server/migrations/atlas.sum` — re-run `task migrate:generate` (or `atlas migrate hash --env local` if only the checksum file conflicts and no new schema change is needed) so the sum file is regenerated mechanically rather than hand-merged; (c) the additive/alphabetical convention for `docker-compose.yml` service blocks and `Taskfile.yml` task blocks established in this step, so parallel PRs adding new services/tasks in the same wave never edit the same lines; (d) a pointer to `.env.local.example` for host-run dev.
- [x] `README.md`: update the "Quick Start" section to mention `task migrate:generate` is now a manual step (not implied by `task up`) and add one line pointing to `docs/infra-conventions.md` for the full migration/compose-editing conventions.
- [x] Add a lightweight verification test: a `Taskfile.yml`-driven check is sufficient here (no new Go/Rust unit test target exists for compose config) — verification is covered by the local `docker compose config` validation and the live health-chain checks in the Verification section below.

## Out of scope
- Adding the actual `minio`, `redis`, `kratos`, `hydra`, `dex` services to `docker-compose.yml` — those are added by Steps 11, 12, 31, 44, 55 respectively, following the alphabetical/additive convention this step documents.
- Any change to the `/health` handler logic itself in `server/internal/interface/handler/health_handler.go` or `llm-gateway/src/adapters/inbound/rest/handlers.rs` (e.g. deep dependency checks against DB/gateway) — both already return a simple 200 liveness response, which is sufficient for compose healthchecks.
- Playwright / E2E harness and the LLM stub container (Step 10).
- Any Atlas schema.sql table changes (this step touches no DB schema).
- CI/CD pipeline changes (excluded entirely from this project's scope per binding decisions — Phases 21-23).
- Redis-backed MessageHub, WebSocket work, RBAC, or any feature-level change — this step is infra-only.

## Implementation notes
- **Files to modify**: `/Users/seigooshima/git/multi-user-ai/docker-compose.yml`, `/Users/seigooshima/git/multi-user-ai/Taskfile.yml`, `/Users/seigooshima/git/multi-user-ai/.env.example`, `/Users/seigooshima/git/multi-user-ai/.gitignore`, `/Users/seigooshima/git/multi-user-ai/README.md`.
- **Files to create**: `/Users/seigooshima/git/multi-user-ai/.env.local.example`, `/Users/seigooshima/git/multi-user-ai/docs/infra-conventions.md`.
- **Existing endpoints to reuse** (do not modify): `GET /health` on the API server is registered in `server/cmd/api/main.go` (`e.GET("/health", healthHandler.Health)`) and implemented in `server/internal/interface/handler/health_handler.go` (`return c.JSON(http.StatusOK, map[string]string{"status": "ok"})`). `GET /health` on the LLM Gateway is registered in `llm-gateway/src/adapters/inbound/rest/router.rs` (`.route("/health", get(health))`) and implemented in `llm-gateway/src/adapters/inbound/rest/handlers.rs`.
- **Healthcheck command choice**: both runtime images are `FROM alpine:3.21` with only `ca-certificates tzdata` added via `apk add` (see `server/Dockerfile` and `llm-gateway/Dockerfile`) — no `curl` binary. Use the busybox `wget` applet that ships with the base Alpine image: `["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:<port>/health"]`. Use `127.0.0.1`, not `localhost`: busybox `wget` resolves `localhost` to `::1` first, but both binaries bind IPv4-only (`0.0.0.0`), so a `localhost` healthcheck fails forever. Do not add `apk add curl` — that would be an unnecessary image-size regression when `wget` already works.
- **docker-compose.yml target shape** (services reordered alphabetically; only the diffs described below change from the current file):
  ```yaml
  services:
    api:
      build:
        context: ./server
      environment:
        DATABASE_URL: ${DATABASE_URL:-postgres://polyphony:polyphony@db:5432/polyphony?sslmode=disable}
        JWT_SECRET: ${JWT_SECRET:-change-me-in-production}
        PORT: ${PORT:-8080}
        CORS_ORIGINS: ${CORS_ORIGINS:-http://localhost:3000}
        LLM_GATEWAY_URL: ${LLM_GATEWAY_URL:-http://llm-gateway:8081}
      ports:
        - "8080:8080"
      healthcheck:
        test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:8080/health"]
        interval: 5s
        timeout: 3s
        retries: 5
        start_period: 5s
      depends_on:
        migrate:
          condition: service_completed_successfully
        llm-gateway:
          condition: service_healthy

    db:
      # unchanged: existing pg_isready healthcheck already matches the target convention

    llm-gateway:
      build:
        context: ./llm-gateway
      environment:
        LLM_GATEWAY_PORT: ${LLM_GATEWAY_PORT:-8081}
        OPENAI_API_KEY: ${OPENAI_API_KEY:-}
      ports:
        - "8081:8081"
      healthcheck:
        test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:8081/health"]
        interval: 5s
        timeout: 3s
        retries: 5
        start_period: 5s

    migrate:
      # unchanged: already depends_on db: condition: service_healthy

    web:
      build:
        context: ./web
        args:
          NEXT_PUBLIC_API_URL: ${NEXT_PUBLIC_API_URL:-http://localhost:8080}
          NEXT_PUBLIC_MOCK_API: ${NEXT_PUBLIC_MOCK_API:-false}
      ports:
        - "3000:3000"
      depends_on:
        api:
          condition: service_healthy
  ```
- **Taskfile.yml target shape** (additive; keep every existing task, only edit `up` and add three new tasks):
  ```yaml
  version: '3'

  dotenv: ['.env.local', '.env']

  tasks:
    up:
      desc: Start all services
      cmds:
        - docker compose up -d

    rebuild:
      desc: Rebuild all Docker images from scratch and restart the stack
      cmds:
        - docker compose build --no-cache
        - docker compose up -d --force-recreate

    migrate:lint:
      desc: Lint the latest migration for destructive/unsafe changes
      dir: server
      cmds:
        - atlas migrate lint --env local --latest 1
  ```
  `migrate:generate`, `migrate:apply`, `migrate:status`, `test*`, `lint*`, `fmt*`, and `dev:*` tasks stay exactly as they are today except that they now inherit the top-level `dotenv` list.
- **Atlas conventions**: `server/atlas.hcl` already declares the `local` env (`src = "file://schema.sql"`, `dev = "docker://postgres/17/dev?search_path=public"`, `migration.dir = "file://migrations"`) reused unchanged by both `migrate:generate`, `migrate:status`, and the new `migrate:lint` task. No `atlas.hcl` changes are needed in this step.
- **`.env.local.example` content**:
  ```
  # Overrides for running server/gateway/web directly on the host (not in Docker).
  # Copy to .env.local (already gitignored) and adjust if your local ports differ:
  #   cp .env.local.example .env.local

  DATABASE_URL=postgres://polyphony:polyphony@localhost:5432/polyphony?sslmode=disable
  LLM_GATEWAY_URL=http://localhost:8081
  NEXT_PUBLIC_API_URL=http://localhost:8080
  ```
- **Conventions to respect**: English-only comments/docs (CLAUDE.md), no code-level GoDoc/rustdoc changes needed since no Go/Rust source files are touched by this step. Keep `.env.example` section headers in the existing `# === Name ===` style already used in the file.
- **conflictNotes carried forward from the plan**: this step is the one that *establishes* the docker-compose.yml/Taskfile.yml additive-alphabetical convention that steps 10, 11, 12, 31, 44, 49, 55 rely on — those later steps must insert new service blocks in alphabetical position and new tasks without reordering or reformatting blocks this step lands, so their diffs stay disjoint from each other and from this step's.

## Verification
1. `docker compose config` — succeeds with no errors, confirming valid YAML and variable interpolation after all edits.
2. `docker compose up -d` (or `task up`) from repo root — all five services start; `docker compose ps` shows `api` and `llm-gateway` reach `healthy` status (not just `running`) within ~30s, and `web` does not start until `api` is `healthy` (observe via `docker compose ps` timestamps or `docker compose logs web` showing no early-connection-refused errors).
3. `curl -sf http://localhost:8080/health` → `{"status":"ok"}` with HTTP 200.
4. `curl -sf http://localhost:8081/health` → HTTP 200 (matches the existing gateway health response shape).
5. `docker inspect --format='{{json .State.Health}}' $(docker compose ps -q api)` and the same for `llm-gateway` — both report `"Status":"healthy"`.
6. `task up` alone (with no prior schema changes) does **not** invoke `atlas migrate diff` — confirm by checking `server/migrations/` has no new auto-generated file after running `task up` twice in a row.
7. `task migrate:lint` — runs `atlas migrate lint --env local --latest 1` against the existing migration history and exits 0 (no destructive changes flagged in the current baseline schema).
8. `task rebuild` — runs to completion (`docker compose build --no-cache` then `docker compose up -d --force-recreate`) and the stack reaches the same healthy state as step 2 afterward.
9. `cp .env.local.example .env.local`, edit nothing further, then run `task dev:server` (with a local Postgres reachable at `localhost:5432`, e.g. via `docker compose up -d db`) — the server starts using `DATABASE_URL` from `.env.local` (points at `localhost`, not `db`) without manual `export`.
10. `git status` after `cp .env.local.example .env.local` shows `.env.local` as untracked/ignored (not staged) while `.env.local.example` is tracked in `git ls-files`.
11. `docs/infra-conventions.md` exists and documents both the one-migration-per-PR + `atlas migrate hash` recovery recipe and the alphabetical/additive compose+Taskfile convention.
12. `task down:clean && task up` (or `docker compose down -v && docker compose up -d`) from a clean state — full stack still comes up healthy end-to-end, confirming no regression from the healthcheck/depends_on changes.

## Completion criteria
- [x] `docker-compose.yml` has `healthcheck` blocks on `api` and `llm-gateway`, a health-gated `depends_on` chain (`db` → `migrate` → `api` ⟷ `llm-gateway` → `web`), and services reordered alphabetically.
- [x] `Taskfile.yml` loads `.env`/`.env.local` via top-level `dotenv`, no longer auto-runs `migrate:generate` from `up`, and has new `migrate:lint` and `rebuild` tasks.
- [x] `.env.local.example` exists and is committed; `.gitignore` allows it via `!.env.local.example` while still ignoring the real `.env.local`.
- [x] `.env.example` has new placeholder sections for Anthropic, Gemini, Stripe, MinIO, Redis, Kratos, Hydra, and Dex.
- [x] `docs/infra-conventions.md` exists and documents the migration and compose/Taskfile editing conventions.
- [x] `README.md` Quick Start reflects the manual `migrate:generate` step and links to `docs/infra-conventions.md`.
- [ ] All verification checks pass. (Wave-1 integration review, 2026-07-16: checks 1, 3, 4, 6, 10, 11 pass. Check 2/5 FAIL as shipped: the `llm-gateway` healthcheck uses `http://localhost:8081/health`, busybox `wget` resolves `localhost` to `::1`, and the gateway binds IPv4-only (`0.0.0.0`), so the container never turns healthy and `api`/`web` never start; with the healthcheck URL changed to `127.0.0.1` the full chain comes up healthy and checks 2-5 pass. Check 7 FAILS: Atlas v0.38.1 gates `migrate lint` behind an Atlas Pro login. Check 9 FAILS: go-task gives precedence to *earlier* dotenv files, so `dotenv: ['.env', '.env.local']` means `.env.local` never overrides `.env` (`task dev:server` still resolved host `db`); reversing the order to `['.env.local', '.env']` makes the override work. Checks 8 and 12 were not run in the review (full `--no-cache` rebuild is slow; `down:clean` would wipe the developer's existing pgdata volume).)
