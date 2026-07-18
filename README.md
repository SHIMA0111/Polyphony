# Polyphony

> [!WARNING]
> This project is under active development and not yet ready for production use.

Polyphony is a chat application with native LLM integration.

Users can chat with each other on Polyphony while also engaging with LLMs based on the conversation history. Multiple users and LLMs can participate in a single room simultaneously, and each room supports flexible access control through roles — Guest for chat-only access without AI, Reader for view-only access, and more.

A next-generation chat application that extends team thinking with LLMs.

[日本語](README-ja.md)

## Features

- **Multi-Provider AI Integration** — OpenAI, Anthropic, Gemini, Ollama, vLLM
- **Team Collaboration** — Chat rooms where users and AI coexist
- **Per-Room RBAC** — Reader → Guest → Member → Admin → Master

## Architecture

| Component | Tech Stack |
|-----------|-----------|
| Web Frontend | Next.js App Router, Bulletproof React |
| Mobile | Flutter, Riverpod |
| API Server | Go, Echo, WebSocket, Redis Pub/Sub |
| LLM Gateway | Rust, axum, Hexagonal Architecture |

**Data Layer**: PostgreSQL / Redis / S3 + CloudFront
**Infrastructure**: Docker Compose → AWS (ECS Fargate, Aurora, ElastiCache)

## Directory Structure

```
server/            — Go API Server (Clean Architecture)
llm-gateway/       — Rust LLM Gateway (Ports & Adapters)
web/               — Next.js Web Frontend
mobile/            — Flutter Mobile App
server/migrations/ — DB Migrations (Atlas)
docker-compose.yml — Local development environment
Taskfile.yml       — Task runner (go-task)
phases.md          — Development Phase Plan (26 phases)
.ai_progress/      — Per-phase Implementation Checklists
```

## Development

### Prerequisites

- Rust 1.93+
- Go 1.24+
- Bun 1.2+
- Flutter 3+
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

### End-to-End Tests (Playwright)

A separate, isolated Docker Compose stack (the `test` profile) runs on alternate host ports
(`5433`, `8090`, `8091`, `8092`, `3001`) alongside — and does not conflict with — the normal dev
stack, with a deterministic OpenAI-compatible LLM stub (`llm-stub/`) standing in for a real
provider:

```bash
# Bring up the isolated E2E stack (Postgres, migrations, API, LLM Gateway, LLM stub, Web)
task test:e2e:up

# Run Playwright specs (seeds a fixture user/room automatically via globalSetup)
task test:e2e

# Tear the E2E stack down, including its volumes
task test:e2e:down
```

`task test:e2e:seed` re-runs just the fixture seed script standalone (idempotent — safe to run
against an already-seeded stack). See `docs/tasks/step10.md` for the full harness design.

## License

[AGPL-3.0](LICENSE.md)
