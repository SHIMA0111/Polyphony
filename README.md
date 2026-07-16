# Polyphony

> [!WARNING]
> This project is under active development and not yet ready for production use.

Polyphony is a chat application with native LLM integration.

Users can chat with each other on Polyphony while also engaging with LLMs based on the conversation history. Multiple users and LLMs can participate in a single room simultaneously, and each room supports flexible access control through roles — Guest for chat-only access without AI, Reader for view-only access, and more.

A next-generation chat application that extends team thinking with LLMs.

[日本語](README-ja.md)

## Features

This repository implements the "Web complete version" of the plan in [`phases.md`](phases.md) — Phases 2 through
20 are fully built and E2E-tested (see [`phases.md`'s Phase Overview](phases.md#phase-overview) for the
phase-by-phase status and its [Deviations from this plan](phases.md#deviations-from-this-plan) section for the
handful of deliberate deviations from the original per-phase plan).

- **Multi-Provider AI Integration** — OpenAI, Anthropic, and Gemini, with per-room model selection, token/context
  metadata (context window, per-1M-token pricing, Vision support), and image/Vision multimodal input
- **Realtime Team Collaboration** — WebSocket-delivered chat rooms (Redis Pub/Sub-backed for multi-instance
  deployments) where users and AI coexist, with streaming (token-by-token) AI responses
- **Per-Room RBAC** — 5-tier roles, Reader → Guest → Member → Admin → Master, plus invitations, member/group
  management, and ownership transfer
- **Production-Grade Auth** — Ory Kratos-backed sessions (email/password + OIDC social login via a local `dex`
  mock provider), Ory Hydra as a first-party OAuth2/OIDC provider, Redis-backed rate limiting and session caching
- **AI Context Control** — per-message AI-exclusion, soft delete, a context cutoff datetime, private AI mode
  (responses visible only to the sender), and automatic history summarization when context overflows the model's
  window
- **Token Billing** — per-room token balance tracking, usage history, and Stripe (test mode) subscriptions/
  on-demand token purchases
- **Room Fork** — asynchronous batch-copy of a room's full history into a new, independent room

## Architecture

| Component | Tech Stack |
|-----------|-----------|
| Web Frontend | Next.js App Router, Bulletproof React |
| Mobile | Flutter, Riverpod |
| API Server | Go, Echo, WebSocket, Redis Pub/Sub |
| LLM Gateway | Rust, axum, Hexagonal Architecture |

**Data Layer**: PostgreSQL / Redis / S3 (MinIO locally) + CloudFront
**Auth**: Ory Kratos (sessions, OIDC social login) + Ory Hydra (OAuth2/OIDC provider), selectable via `AUTH_MODE`
(`simple_jwt` also supported)
**Infrastructure**: Docker Compose → AWS (ECS Fargate, Aurora, ElastiCache)

> [!NOTE]
> The **Mobile** row and the AWS/Terraform/CI-CD infrastructure migration correspond to `phases.md` Phases 21-25,
> which are **intentionally excluded from this repository's implemented scope** per the project's binding scope
> decision — there is no `mobile/` directory and no Terraform in this codebase. See
> [`phases.md`'s Phase Overview](phases.md#phase-overview) for the full phase-by-phase status.

## Directory Structure

```
server/            — Go API Server (Clean Architecture)
llm-gateway/       — Rust LLM Gateway (Ports & Adapters)
web/               — Next.js Web Frontend
web/e2e/           — Playwright E2E harness (smoke + feature specs + regression suites)
ory/               — Ory Kratos/Hydra/dex configuration (auth, OIDC, mock social login)
llm-stub/          — Canned OpenAI-compatible LLM stub used by the E2E test profile
server/migrations/ — DB Migrations (Atlas)
docker-compose.yml — Local development environment (+ isolated `test` profile for E2E)
Taskfile.yml       — Task runner (go-task)
phases.md          — Development Phase Plan (26 phases; Phases 2-20 implemented in this repo)
docs/tasks/        — Step-by-step build plan (steps 1-60) this codebase was implemented from
.ai_progress/      — Per-phase Implementation Checklists
```

`mobile/` (Flutter, Phases 24-25) is not present in this repository — see the note above.

## Development

### Prerequisites

- Rust 1.93+
- Go 1.24+
- Bun 1.2+
- Docker / Docker Compose
- PostgreSQL 17 / Redis 7
- Protocol Buffers compiler (`protoc`) 3.15+ (needed for local `cargo build`/`cargo run` in `llm-gateway/` outside Docker)
- [go-task](https://taskfile.dev/) (optional, for `task` commands)

### Quick Start (Docker Compose)

```bash
# Start all services (PostgreSQL, Redis, Go API, LLM Gateway, Web Frontend)
task up
# or: docker compose up -d

# View logs
task logs
```

> `task up` only starts the stack — it no longer runs `migrate:generate` implicitly. After changing
> `server/schema.sql`, generate the migration explicitly with `task migrate:generate -- <name>`. See
> [docs/infra-conventions.md](docs/infra-conventions.md) for the full migration and
> compose/Taskfile-editing conventions.

### Individual Services

```bash
# LLM Gateway
cd llm-gateway
cargo build && cargo test
OPENAI_API_KEY=sk-... cargo run
# → http://localhost:8081/health

# Go API Server
cd server
go run ./cmd/api
# → http://localhost:8080

# Web Frontend
cd web
bun install && bun run dev
# → http://localhost:3000
```

### Running the E2E suite (Playwright)

A separate, isolated Docker Compose stack (the `test` profile) runs on alternate host ports
(`5433`, `8090`, `8091`, `8092`, `3001`, plus dedicated `kratos-e2e`/`dex-e2e`/`hydra-e2e` ports for the
auth-dependent specs) alongside — and does not conflict with — the normal dev stack, with a deterministic
OpenAI-compatible LLM stub (`llm-stub/`) standing in for a real provider:

```bash
# Full clean-state gate: tear down anything left running, bring the stack up, run everything
task test:e2e:down   # ignore "no such service" if nothing was running
task test:e2e:up     # Postgres, migrations, API, LLM Gateway, LLM stub, Web, Kratos/Hydra/dex
task test:e2e        # runs the full Playwright suite; seeds fixtures automatically via globalSetup
task test:e2e:down   # tear down, including volumes
```

A fully green run reports every project passed with zero failed and zero flaky-after-retry specs in Playwright's
terminal summary line (e.g. `X passed (Ys)` across both the `chromium` and `rate-limiting` projects — see
`web/playwright.config.ts`'s comments for why the rate-limiting spec runs in its own, strictly-later project). The
suite covers the Step 10 smoke spec, every feature-area spec (rooms, members, groups, invitations, OAuth,
attachments/Vision, billing, room fork, private mode, streaming, AI context controls), and all four regression
suites under `web/e2e/regression/` (realtime AI core, advanced AI, billing/fork, and identity/collaboration).

`task test:e2e:seed` re-runs just the fixture seed script standalone (idempotent — safe to run
against an already-seeded stack). See `docs/tasks/step10.md` for the full harness design.

#### Running a single spec in isolation (faster iteration / deflaking)

Once `task test:e2e:up` has brought the stack up, run any single spec file (or glob) directly from `web/` instead
of the full suite:

```bash
cd web
bunx playwright test e2e/rooms.spec.ts
bunx playwright test e2e/regression/ai-send-model-select.spec.ts
bunx playwright test e2e/regression/rate-limiting.spec.ts --no-deps   # skips the chromium project dependency
```

`task test:e2e` is the **authoritative full-suite gate** — `task test`, `task test:server`, and `task test:gateway`
remain fast, compose-free unit-only tasks and intentionally do not depend on it (see `Taskfile.yml`).

#### Billing (Stripe) E2E coverage

The `test` Compose profile has no `STRIPE_*` configuration or webhook-forwarding service, so
`web/e2e/regression/billing.spec.ts`'s Stripe Checkout/webhook/subscription-cancel journey self-skips in a default
local run (everything reachable without live credentials — the plan catalog, empty states, error paths — is still
asserted). This is the accepted steady state, not a failure. See
[`web/e2e/regression/README.md`](web/e2e/regression/README.md) for the exact environment variables and
`stripe listen` setup needed to exercise the full Checkout journey locally.

## License

[AGPL-3.0](LICENSE.md)
