# Polyphony

> [!WARNING]
> This project is under active development and not yet ready for production use.

*In music, polyphony is the simultaneous combination of independent voices that harmonize as a whole.*

Polyphony brings this concept to AI chat — a platform where humans and AI each contribute their own independent voice, collaborating in shared rooms to create something greater than the sum of its parts.

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
server/        — Go API Server (Clean Architecture)
llm-gateway/   — Rust LLM Gateway (Ports & Adapters)
web/           — Next.js Web Frontend
mobile/        — Flutter Mobile App
migrations/    — DB Migrations (golang-migrate)
phases.md      — Development Phase Plan (26 phases)
.ai_progress/  — Per-phase Implementation Checklists
```

## Development

### Prerequisites

- Rust 1.93+
- Go 1.24+
- Node.js 22+
- Flutter 3+
- Docker / Docker Compose
- PostgreSQL 17 / Redis 7

### LLM Gateway

```bash
cd llm-gateway
cargo build
cargo test

# Start server
OPENAI_API_KEY=sk-... cargo run
# → http://localhost:8081/health
```

## License

[AGPL-3.0](LICENSE.md)
