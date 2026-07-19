# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Polyphony — a multi-user AI chat application that combines ChatGPT-like AI chat with LINE/Slack-like multi-user chat rooms.

See `phases.md` for detailed development phase plans, architecture, and scope.

### Key Differentiators

- Multi-provider AI integration (OpenAI, Anthropic, Gemini, Ollama, vLLM)
- Team collaboration (users and AI coexist in shared chat rooms)
- Per-room RBAC (Reader → Guest → Member → Admin → Master)

## Architecture (4 Components)

1. **Web Frontend** — Next.js App Router + Bulletproof React architecture
2. **Mobile Frontend** — Flutter (iOS/Android), Riverpod state management
3. **Main API Server** — Go + Echo, Clean Architecture, WebSocket via Redis Pub/Sub
4. **LLM Gateway** — Rust, Hexagonal Architecture (Ports & Adapters), gRPC/REST

**Data layer**: Single PostgreSQL DB (messages table monthly partitioning is planned for Phase 23, out of this repo's current scope — today it is a plain table), Redis (sessions, Pub/Sub, rate limiting), S3 + CloudFront (images, presigned URLs)
**Auth**: Initially SimpleJWT (argon2+JWT) → Phase 9: Ory Kratos + Phase 15: Ory Hydra (OAuth2/OIDC)
**Infra**: Initially Docker Compose → Phase 21: AWS (ECS Fargate, Aurora Serverless v2, ElastiCache), Terraform, GitHub Actions CI/CD

## Directory Structure

See `phases.md` for detailed directory structures of each component.

- `server/` — Go API server (Clean Architecture: domain → usecase → interface/infrastructure)
- `llm-gateway/` — Rust LLM Gateway (Hexagonal: domain/ports/adapters)
- `web/` — Next.js Web Frontend (Bulletproof React: app/features/components/hooks/lib)
- `server/schema.sql` — DB schema definition (Atlas declarative management)
- `server/atlas.hcl` — Atlas configuration
- `phases.md` — Development phase plan (26 phases)
- `.ai_progress/` — Per-phase implementation checklists

## Progress Tracking

Create `.ai_progress/phaseX.md` when starting each phase, and track tasks with checklists.

- Create a checklist before starting implementation to clarify scope
- Check off items with `[x]` as tasks are completed
- Verify all items are checked when a phase is complete

## Development Conventions

- **Language**: All code, comments, and documentation in English (`README-ja.md` is the approved Japanese localization and the sole exception)
- **Go docstrings**: GoDoc format on all exported symbols. Describe what/why/constraints/error behavior
- **Rust docstrings**: rustdoc (`///`) on all public symbols with `# Arguments`, `# Returns`, `# Errors` sections
- **Clean Architecture**: Domain layer must NOT import infrastructure packages. All inter-layer communication via interfaces
- **CQRS-lite**: Separate read/write query paths, single DB
- **Event-driven**: Message sends and user actions emit events. WebSocket delivery and AI invocations are handled by event handlers
- **Testing**: Unit tests with mocked Repository/Gateway, testcontainers integration tests (PostgreSQL), Playwright E2E (web), Flutter integration tests, k6 load tests
- **DB migrations**: Atlas (declarative schema management + versioned migrations)
- **Structured logging**: Go uses `slog`, Rust uses `tracing`. JSON format
- **Tracing**: Request-ID propagation is implemented (Echo request ID flows into request-scoped `slog`/`tracing` fields on both servers); OpenTelemetry export is planned for Phase 23 (out of this repo's current scope) and not yet wired

## Key Domain Logic

**AI context building** (most complex part): When invoking AI in a room, filter messages (exclude flag, cutoff datetime, deleted, private visibility) → count tokens via LLM Gateway → if over limit, summarize older history with AI and cache in `message_context_summaries`. Implemented in Phase 5, 18.

**Room fork**: Async batch job copies messages to a new room (1000/batch), reassigns sequence numbers, `is_archived=true` until complete. Implemented in Phase 20.

**Token billing**: Room master pays. Three payment rails (Stripe, App Store, Google Play) converge into `token_balances`. Balance checked before AI requests. Implemented in Phase 16-17, 25.

## Interface Swap Points

All three application-level swaps below are fully implemented on both sides and selectable at runtime via env var; see `phases.md`'s "Interface Swap Points" table for the full description column.

| Abstraction | Initial Implementation | Swap Phase | Status |
|-------------|----------------------|------------|--------|
| `AuthService` | SimpleJWT (argon2+JWT) | Phase 9 (Kratos) | Done — `AUTH_MODE=simple_jwt\|kratos` |
| `MessageHub` | InProcessHub | Phase 10 (Redis) | Done — `MESSAGE_HUB_DRIVER=inprocess\|redis` |
| `LLMClient` | REST client | Phase 8 (gRPC) | Done — `LLM_GATEWAY_TRANSPORT=rest\|grpc` |
| Rate limiting | None | Phase 10 (Redis, swap-adjacent) | Done — Redis Token Bucket, no driver swap |
| Infrastructure | Docker Compose | Phase 21 (AWS) | Out of this repo's scope |

## UI Development Workflow

Use the `v0-design` skill to delegate design generation to v0 when creating new UI components or pages.
Small modifications to existing components or logic-only changes can be edited directly.

## Available MCP Servers

- `v0`: UI design generation (`v0:create_chat`, `v0:send_message`, `v0:get_chat`, `v0:find_chats`)
- `chakra-ui`: Chakra UI v3 component info, code examples, design tokens, migration (`@chakra-ui/react-mcp`)
- `playwright`: Browser automation and screenshots
- `context7`: Library documentation reference
- `serena`: Codebase analysis

## Project Structure

- Next.js App Router (under `web/` directory)
- Bulletproof React architecture: `web/src/app/`, `web/src/features/`, `web/src/components/`, etc.

## Project Rules (auto-loaded by path matching)

- `chakra-ui.md`: Chakra UI v3 component conventions (web/src/app/**, web/src/components/**, web/src/features/**/components/**)
