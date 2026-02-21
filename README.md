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
- [go-task](https://taskfile.dev/) (optional, for `task` commands)

### Quick Start (Docker Compose)

```bash
# Start all services (PostgreSQL, Go API, LLM Gateway, Web Frontend)
task up
# or: docker compose up -d

# View logs
task logs
```

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

## License

[AGPL-3.0](LICENSE.md)
