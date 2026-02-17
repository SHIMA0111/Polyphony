# Development Phase Plan

## Application Overview

A multi-user AI chat application. Combines ChatGPT-like AI chat with LINE/Slack-like multi-user chat rooms.

### Differentiators

- **Multi-provider AI integration** — OpenAI, Anthropic, Gemini, Ollama, vLLM
- **Team collaboration** — Chat rooms where users and AI coexist
- **Per-room RBAC** — Reader → Guest → Member → Admin → Master

### Components (4)

1. **Web Frontend** — Next.js App Router + Bulletproof React architecture
2. **Mobile Frontend** — Flutter (iOS/Android), Riverpod state management
3. **Main API Server** — Go + Echo, Clean Architecture, WebSocket via Redis Pub/Sub
4. **LLM Gateway** — Rust, Hexagonal Architecture (Ports & Adapters), gRPC/REST

### Data Layer & Infrastructure

- **DB**: Single PostgreSQL (messages with monthly partitioning)
- **Cache/Pub/Sub**: Redis (sessions, Pub/Sub, rate limiting)
- **Storage**: S3 + CloudFront (images, presigned URLs)
- **Auth**: Initially SimpleJWT (argon2+JWT) → Phase 9 Ory Kratos → Phase 15 Ory Hydra
- **Infra**: Initially Docker Compose → Phase 21 AWS (ECS Fargate, Aurora, ElastiCache)

## Approach: 1 Phase = 1 Theme

This plan incrementally builds the application through 26 phases, each focused on a single theme.

### Core Principles

- **Small increments**: Each phase is scoped to complete within 1–2 weeks. Each phase builds on the previous one
- **Swappable via interfaces**: Start with simple implementations; swap at interface boundaries later. Don't aim for production quality from the start
- **Always keep it working**: The application must be functional at the end of every phase

---

## Go API Server Architecture: Clean Architecture

The Go API server follows Clean Architecture with strict dependency direction: domain → usecase → interface/infrastructure.

```
server/
├── cmd/
│   └── api/
│       └── main.go                    # Entry point, DI assembly
├── internal/
│   ├── domain/                        # Domain layer (no external dependencies)
│   │   ├── user/
│   │   │   ├── entity.go              # User struct
│   │   │   ├── repository.go          # UserRepository interface
│   │   │   └── service.go             # Domain logic
│   │   ├── room/
│   │   │   ├── entity.go              # Room, RoomMember, RoomRole
│   │   │   ├── repository.go          # RoomRepository interface
│   │   │   └── service.go
│   │   ├── message/
│   │   │   ├── entity.go              # Message, MessageType
│   │   │   ├── repository.go
│   │   │   └── service.go
│   │   ├── auth/
│   │   │   └── service.go             # AuthService interface (★ swap point)
│   │   └── ai/
│   │       ├── entity.go              # AIRequest, AIResponse
│   │       ├── context_builder.go     # Context building logic
│   │       └── gateway.go             # LLMGateway interface (★ swap point)
│   ├── usecase/                       # Usecase layer
│   │   ├── auth/usecase.go            # Register, Login
│   │   ├── room/usecase.go            # CreateRoom, JoinRoom
│   │   ├── message/usecase.go         # SendMessage, SendAIMessage
│   │   └── ai/usecase.go             # InvokeAI, BuildContext
│   ├── interface/                     # Adapter layer (outer)
│   │   ├── handler/                   # Echo HTTP handlers
│   │   │   ├── auth_handler.go
│   │   │   ├── room_handler.go
│   │   │   ├── message_handler.go
│   │   │   └── ws_handler.go          # WebSocket (added in Phase 2)
│   │   ├── middleware/
│   │   │   ├── auth.go                # JWT verification → swap to Kratos session in Phase 9
│   │   │   └── rbac.go                # Added in Phase 4
│   │   ├── repository/
│   │   │   └── postgres/              # PostgreSQL implementations (all tables)
│   │   └── gateway/
│   │       └── llm_client.go          # LLM Gateway REST/gRPC client
│   └── infrastructure/
│       ├── database/postgres.go       # Connection pool
│       ├── websocket/                 # Added in Phase 2
│       │   ├── hub.go                 # MessageHub interface + InProcessHub
│       │   ├── client.go
│       │   └── event.go
│       └── config/config.go
├── migrations/                        # Atlas
├── proto/                             # Added in Phase 8
├── atlas.hcl
├── schema.sql
└── go.mod
```

**Principles**:
- `domain/` has no dependencies beyond the standard library
- All Repository/Service/Gateway are defined as interfaces in `domain/`; implementations live in `interface/` or `infrastructure/`
- The usecase layer only references domain interfaces (no knowledge of concrete types)
- DI assembly happens in `cmd/api/main.go`

---

## Frontend Architecture: Bulletproof React

The Web Frontend follows the Bulletproof React architecture.

```
web/
├── src/
│   ├── app/              # Next.js App Router (routing layer only)
│   │   ├── layout.tsx    # Root layout
│   │   ├── (auth)/       # Auth route group
│   │   │   ├── login/page.tsx
│   │   │   └── register/page.tsx
│   │   └── (main)/       # Main app route group
│   │       ├── layout.tsx
│   │       ├── rooms/page.tsx
│   │       └── rooms/[roomId]/page.tsx
│   ├── components/       # Shared components
│   ├── config/           # Global configuration
│   ├── features/         # Feature modules (★ core)
│   │   ├── auth/         # Auth (api/, components/, hooks/, types/)
│   │   ├── rooms/        # Room management
│   │   ├── messages/     # Messages
│   │   ├── ai/           # AI invocation
│   │   └── ...
│   ├── hooks/            # Shared hooks
│   ├── lib/              # Pre-configured library wrappers
│   ├── stores/           # Global state
│   ├── testing/          # Test utilities
│   ├── types/            # Shared type definitions
│   └── utils/            # Shared utilities
└── public/               # Static files
```

**Coexistence rules with Next.js App Router**:
- The `app/` directory is for routing and layouts only. No business logic
- Each `page.tsx` is a thin wrapper that imports and renders the corresponding `features/` component
- Server Components/Server Actions are defined in `api/` directories within `features/`

**Principles**:
- Direct imports between features are forbidden (shared parts go through `components/`, `hooks/`, `lib/`)
- Dependency direction is one-way: shared → features → app
- No barrel files (index.ts) for tree-shaking optimization

---

## LLM Gateway Architecture: Hexagonal Architecture (Ports & Adapters)

The LLM Gateway (Rust) adopts Hexagonal Architecture to prevent singleton file bloat.

```
llm-gateway/src/
├── domain/                # Domain layer (pure business logic)
│   ├── model.rs           # CompletionRequest, CompletionResponse, ModelInfo, etc.
│   ├── error.rs           # Domain error types
│   └── service.rs         # LLM invocation domain service
├── ports/                 # Port definitions (traits)
│   ├── inbound/           # Driving ports (outside → inside)
│   │   └── completion.rs  # CompletionUseCase trait
│   └── outbound/          # Driven ports (inside → outside)
│       ├── provider.rs    # LLMProvider trait (per AI provider)
│       └── key_store.rs   # API key retrieval trait
├── adapters/              # Adapter implementations
│   ├── inbound/           # Driving adapters
│   │   ├── grpc/          # gRPC server (added in Phase 8)
│   │   └── rest/          # REST API handlers (axum/actix-web)
│   └── outbound/          # Driven adapters
│       ├── openai.rs      # OpenAI API adapter
│       ├── anthropic.rs   # Anthropic API adapter (Phase 6)
│       ├── gemini.rs      # Gemini API adapter (Phase 7)
│       └── env_key.rs     # API key retrieval from environment variables
├── config.rs              # Application configuration
└── main.rs                # Entry point (DI assembly)
```

**Principles**:
- `domain/` and `ports/` have no external crate dependencies (only `serde`, etc. allowed)
- Adding a provider = add a file in `adapters/outbound/` + register DI in `main.rs`
- Inbound adapters (REST/gRPC) only call traits from `ports/inbound/`

---

## Phase Overview

| # | Theme | Goal |
|---|-------|------|
| 1 | Project skeleton + minimal AI chat | A single user can chat with AI |
| 2 | WebSocket + real-time | Messages displayed in real-time |
| 3 | Invitations + member management | Users can be invited to rooms |
| 4 | Basic RBAC | 3 roles: reader/member/master |
| 5 | AI context control | Per-message AI exclusion, token estimation |
| 6 | Multi-provider (Anthropic) | Second AI provider |
| 7 | Multi-provider (Gemini) + model selection UI | Users can choose models |
| 8 | gRPC + LLM Gateway hardening | Retries, health checks, model metadata |
| 9 | Ory Kratos adoption | Migrate to production auth |
| 10 | Redis + multi-instance | WebSocket Pub/Sub, rate limiting |
| 11 | Full RBAC | 5-tier permissions with guest/admin |
| 12 | Image upload + Vision | S3, multimodal AI |
| 13 | Group management + batch invitations | Invite by group |
| 14 | Private AI mode | AI responses visible only to sender |
| 15 | OAuth social login | Google/GitHub integration |
| 16 | Token balance management | Usage tracking, balance checks |
| 17 | Stripe billing | Subscriptions + on-demand purchases |
| 18 | Context summarization | Summarize old history with AI → cache |
| 19 | Streaming AI responses | Tokens displayed incrementally |
| 20 | Room fork | Async batch copy |
| 21 | AWS infrastructure (Terraform) | VPC, ECS, Aurora, ElastiCache |
| 22 | CI/CD + staging | GitHub Actions, ECR, ECS deploy |
| 23 | Monitoring + production ops | CloudWatch, WAF, partitioning, k6 |
| 24 | Flutter mobile app | iOS/Android support |
| 25 | Mobile billing + push notifications | App Store/Google Play integration |
| 26+ | Enterprise (future) | SSO/SAML, admin dashboard, Ollama/vLLM |

---

## Interface Swap Points

The following abstractions keep the initial implementation simple while allowing production-quality swaps in later phases.

| Abstraction | Initial Implementation | Swap Timing | Description |
|-------------|----------------------|-------------|-------------|
| `AuthService` | SimpleJWT (argon2+JWT) | Phase 9 (Kratos) | Auth & session management. Phase 1: argon2 password hashing, JWT token issuance. Phase 9: swap to Ory Kratos session verification |
| `MessageHub` | InProcessHub | Phase 10 (Redis) | WebSocket message delivery. Phase 2: in-process hub. Phase 10: swap to Redis Pub/Sub for multi-instance support |
| `LLMClient` | REST client | Phase 8 (gRPC) | LLM Gateway communication. Phase 1: simple REST client. Phase 8: swap to gRPC client |
| Infra | Docker Compose | Phase 21 (AWS) | Runtime environment. Docker Compose during development. Phase 21: migrate to AWS via Terraform (ECS Fargate, Aurora, ElastiCache) |

---

## Phase Details

### Phase 1: Project Skeleton + Minimal AI Chat

**Goal**: A minimal application where a single user can chat with AI from a browser

#### Go API

- Clean Architecture skeleton (domain/usecase/interface/infrastructure)
- `AuthService` interface + SimpleJWT implementation (argon2 + JWT)
- User registration & login API
- Room CRUD API
- Message persistence + cursor pagination
- AI invocation (LLM Gateway REST client → synchronous response → DB save. On LLM failure, saves a `status=failed` placeholder AI message)
- AI response regeneration API (`POST /rooms/:roomId/messages/:messageId/regenerate` — always UPDATEs the AI message immediately following the specified human message. No create path. Preserves sequence/created_at to maintain ordering)

#### LLM Gateway (Rust)

- Hexagonal Architecture skeleton
- `domain/` — CompletionRequest/Response types, domain errors
- `ports/outbound/provider.rs` — `LLMProvider` trait definition
- `ports/inbound/completion.rs` — `CompletionUseCase` trait definition
- `adapters/outbound/openai.rs` — OpenAI adapter
- `adapters/inbound/rest/` — REST API handlers (`/completions`, `/models`, `/health`)
- `main.rs` — DI assembly

#### Web Frontend (Next.js)

- Next.js App Router + Bulletproof React directory structure
- SSR (Server Components) + Server Actions
- `features/auth/` — Login & registration pages
- `features/rooms/` — Room listing
- `features/messages/` — Chat screen (polling or manual reload, no real-time)

#### DB Changes

- `users` — Basic user info, password hash
- `rooms` — Room info
- `room_members` — Room membership
- `messages` — Message body, sender, type (human/ai), status (completed/failed)
- `room_sequences` — Per-room message sequence counter

#### Infrastructure

- Docker Compose (PostgreSQL + Go API + Rust LLM Gateway + Next.js)

#### Excluded

- WebSocket (Phase 2)
- RBAC (Phase 4)
- Multi-provider (Phase 6–7)
- Images (Phase 12)
- Billing (Phase 16–17)
- Redis (Phase 10)
- S3 (Phase 12)

---

### Phase 2: WebSocket + Real-time

**Goal**: Messages are delivered to all members in real-time

#### Scope

- Go API: WebSocket endpoint, `MessageHub` interface + `InProcessHub` implementation
- Web Frontend: WebSocket connection, real-time message receive & display
- Event-driven: Message send → Hub → broadcast to connected clients

#### DB Changes

None

#### Excluded

- Redis Pub/Sub (Phase 10)
- Multi-instance support (Phase 10)

---

### Phase 3: Invitations + Member Management

**Goal**: Room owners can invite users and view the member list

#### Scope

- Go API: Invitation API, member list API, invitation accept/reject
- Go API: Ownership transfer API (`PATCH /rooms/:roomId/owner`) — current owner transfers ownership to another member. Also required for billing entity (`token_balances`) migration
- Go API: Member leave API (`DELETE /rooms/:roomId/members/:userId`) — `room_members` uses ON DELETE RESTRICT, so leave must be handled explicitly by the app
- Web Frontend: Member management screen, invitation form, ownership transfer UI
- Invitation via link or username search

#### DB Changes

- `room_invitations` — Invitation records

#### Excluded

- Role management (Phase 4)
- Group batch invitations (Phase 13)

---

### Phase 4: Basic RBAC

**Goal**: 3-tier role-based access control with reader/member/master

#### Scope

- Go API: RBAC middleware, role change API
- Room creator automatically becomes master
- reader: View messages only (cannot send)
- member: Send messages + invoke AI
- master: Manage members, change room settings
- Web Frontend: Role display, master admin UI

#### DB Changes

- Add role column to `room_members` table (existing data defaults to member)

#### Excluded

- guest/admin roles (Phase 11)

---

### Phase 5: AI Context Control

**Goal**: Set per-message AI exclusion flags and estimate token counts

#### Scope

- Go API: AI exclusion flag on messages, context building logic (considers exclusion flag, deleted messages, cutoff datetime)
- LLM Gateway: Token estimation endpoint
- Web Frontend: AI exclusion toggle per message, estimated token count display

#### DB Changes

- Add `exclude_from_ai` column to `messages` table
- Add `ai_context_cutoff_at` column to `rooms` table

#### Excluded

- History summarization (Phase 18)
- Private AI mode (Phase 14)

---

### Phase 6: Multi-provider (Anthropic)

**Goal**: Use Anthropic models in addition to OpenAI

#### Scope

- LLM Gateway: `adapters/outbound/anthropic.rs` — Anthropic API adapter
- Go API: Provider/model selection in room settings
- Web Frontend: Model settings UI (within room settings)

#### DB Changes

- Add `ai_provider`, `ai_model` columns to `rooms` table

#### Excluded

- Gemini (Phase 7)
- Rich model selection UI (Phase 7)

---

### Phase 7: Multi-provider (Gemini) + Model Selection UI

**Goal**: 3-provider support, intuitive model selection UI

#### Scope

- LLM Gateway: `adapters/outbound/gemini.rs` — Gemini API adapter
- LLM Gateway: `/models` endpoint returns available model list
- Web Frontend: Model selection dropdown (provider + model name)

#### DB Changes

None

#### Excluded

- Ollama/vLLM (Phase 26+)

---

### Phase 8: gRPC + LLM Gateway Hardening

**Goal**: Switch Go API ↔ LLM Gateway communication to gRPC, add retries and health checks

#### Scope

- LLM Gateway: Add gRPC server (`adapters/inbound/grpc/`)
- Go API: Swap to gRPC client (`LLMClient` interface implementation swap)
- Proto definitions (`server/proto/`)
- Retry policy (exponential backoff)
- Health check endpoint
- Model metadata (token limits, pricing info)

#### DB Changes

None

#### Excluded

- Streaming (Phase 19)

---

### Phase 9: Ory Kratos Adoption

**Goal**: Migrate auth from SimpleJWT to Ory Kratos

#### Scope

- Add Ory Kratos container (Docker Compose)
- Go API: Swap `AuthService` implementation to Kratos session verification
- Middleware: JWT header verification → Kratos session cookie verification
- Web Frontend: Adjust login/registration flow for Kratos UI
- Existing user data migration script

#### DB Changes

- Kratos-managed DB schema
- Add `kratos_identity_id` column to `users` table

#### Excluded

- OAuth/social login (Phase 15)
- Ory Hydra (Phase 15)

---

### Phase 10: Redis + Multi-instance

**Goal**: Introduce Redis for WebSocket Pub/Sub, rate limiting, and session caching

#### Scope

- Add Redis container (Docker Compose)
- Go API: Swap `MessageHub` implementation to Redis Pub/Sub
- Rate limiting middleware (Redis Token Bucket)
- Session cache (cache Kratos session verification results)

#### DB Changes

None

#### Excluded

- Redis Cluster configuration (Phase 21)

---

### Phase 11: Full RBAC

**Goal**: 5-tier permissions with reader/guest/member/admin/master

#### Scope

- Go API: guest role (can send messages, cannot invoke AI), admin role (can manage members, cannot delete room)
- RBAC middleware update
- Web Frontend: Updated role selection UI, permission-based UI display control

#### DB Changes

- Add new values to role column in `room_members` table

#### Excluded

None

---

### Phase 12: Image Upload + Vision

**Goal**: Upload images and have AI perform image recognition

#### Scope

- S3 (MinIO for local) + presigned URLs
- Go API: Image upload API, presigned URL issuance
- LLM Gateway: Multimodal request support (Vision API)
- Web Frontend: Image upload UI, image preview

#### DB Changes

- `message_attachments` — File metadata (S3 key, MIME type, size)

#### Excluded

- CloudFront (Phase 21)
- Other file formats (video, PDF, etc.)

---

### Phase 13: Group Management + Batch Invitations

**Goal**: Create user groups and invite entire groups to rooms

#### Scope

- Go API: Group CRUD, group member management, group-based invitations
- Web Frontend: Group management screen, group invitation UI

#### DB Changes

- `groups` — Group info
- `group_members` — Group membership

#### Excluded

None

---

### Phase 14: Private AI Mode

**Goal**: Option to make AI responses visible only to the sender

#### Scope

- Go API: `visibility` flag on messages (public/private)
- Private mode specification during AI requests
- Private responses delivered via WebSocket only to the sender
- Web Frontend: Private mode toggle during AI requests

#### DB Changes

- Add `visibility` column to `messages` table

#### Excluded

None

---

### Phase 15: OAuth Social Login

**Goal**: Login with Google/GitHub accounts

#### Scope

- Introduce Ory Hydra (OAuth2/OIDC provider)
- Kratos config: Add Google/GitHub OIDC providers
- Web Frontend: Social login buttons

#### DB Changes

- Kratos-managed (external provider links)

#### Excluded

- SSO/SAML (Phase 26+)

---

### Phase 16: Token Balance Management

**Goal**: Track AI token usage and manage balances

#### Scope

- Go API: Token consumption recording, balance check (before AI requests), insufficient balance error
- Room master holds the balance (per-room billing model)
- Web Frontend: Balance display, usage history

#### DB Changes

- `token_balances` — Per-user balance
- `token_transactions` — Token consumption & charge history

#### Excluded

- Payment integration (Phase 17)
- Automated initial balance provisioning

---

### Phase 17: Stripe Billing

**Goal**: Purchase subscriptions and on-demand tokens via Stripe

#### Scope

- Go API: Stripe Webhook processing, subscription management, one-time purchases
- Subscription plans (monthly token allocation)
- Web Frontend: Plan selection, Stripe Checkout integration, billing history

#### DB Changes

- `subscriptions` — Subscription info
- `payment_history` — Payment records

#### Excluded

- App Store/Google Play billing (Phase 25)

---

### Phase 18: Context Summarization

**Goal**: Summarize long conversation history with AI to fit within the context window

#### Scope

- Go API: Detect token limit exceeded during context building
- On overflow: Summarize old messages with AI → cache in `message_context_summaries`
- Insert summary at the beginning of context, only include recent individual messages
- Web Frontend: Indicate when summaries are being used

#### DB Changes

- `message_context_summaries` — Summary cache (room, covered period, summary text, token count)

#### Excluded

None

---

### Phase 19: Streaming AI Responses

**Goal**: AI responses are displayed incrementally, token by token

#### Scope

- LLM Gateway: Server-Sent Events (SSE) or gRPC streaming support
- Go API: Forward streaming responses to clients via WebSocket
- Web Frontend: Incremental token display, typing indicator

#### DB Changes

None (existing message save flow after streaming completes)

#### Excluded

None

---

### Phase 20: Room Fork

**Goal**: Copy a room's conversation to a new room to create a branch

#### Scope

- Go API: Async batch job (copy 1000 messages/batch)
- Sequence renumbering
- Block new posts with `is_archived=true` until copy completes
- Web Frontend: Fork button, progress display

#### DB Changes

- Add `forked_from_room_id`, `is_archived` columns to `rooms` table
- `room_fork_jobs` — Fork job progress

#### Excluded

None

---

### Phase 21: AWS Infrastructure (Terraform)

**Goal**: Build production environment on AWS

#### Scope

- Terraform: VPC, subnets, security groups
- ECS Fargate (Go API, LLM Gateway, Next.js, Kratos)
- Aurora Serverless v2 (PostgreSQL compatible)
- ElastiCache (Redis)
- S3 + CloudFront (image delivery)
- ALB + ACM (HTTPS)

#### DB Changes

None

#### Excluded

- CI/CD (Phase 22)
- Monitoring (Phase 23)

---

### Phase 22: CI/CD + Staging

**Goal**: Automatic deployment to staging on merge to main branch

#### Scope

- GitHub Actions: Test → Build → ECR push → ECS deploy
- Staging environment (Terraform workspace isolation)
- Automated DB migration
- E2E tests (Playwright) run on staging

#### DB Changes

None

#### Excluded

- Production deployment flow (Phase 23)

---

### Phase 23: Monitoring + Production Operations

**Goal**: Monitoring, security, and performance measures required for production

#### Scope

- CloudWatch Logs + Metrics + Alarms
- AWS WAF (rate limiting, SQLi/XSS protection)
- PostgreSQL messages table monthly partitioning
- k6 load testing
- OpenTelemetry traces (X-Ray integration)
- Production deployment flow (with approval gate)

#### DB Changes

- Partitioning configuration for `messages` table

#### Excluded

None

---

### Phase 24: Flutter Mobile App

**Goal**: Use equivalent features on iOS/Android native apps as on the web

#### Scope

- Flutter: Riverpod state management
- Auth (Kratos sessions)
- Room list, chat screen, WebSocket connection
- Image upload (camera/gallery)

#### DB Changes

None

#### Excluded

- App Store/Google Play billing (Phase 25)
- Push notifications (Phase 25)

---

### Phase 25: Mobile Billing + Push Notifications

**Goal**: In-app purchases and push notification support

#### Scope

- Flutter: App Store/Google Play billing integration (RevenueCat, etc.)
- Go API: App Store Server Notifications / Google Play RTDN Webhook
- Balance reflection in `token_balances` (shared across Stripe/App Store/Google Play)
- Push notifications via Firebase Cloud Messaging (FCM)
- Go API: Notification send logic (new message, invitation, AI response complete)

#### DB Changes

- `device_tokens` — FCM device tokens
- Add payment_rail (stripe/app_store/google_play) to `payment_history` table

#### Excluded

None

---

### Phase 26+: Enterprise (Future)

**Goal**: Features for large-scale organizations

#### Scope (Candidates)

- SSO/SAML support (Ory Hydra extension)
- Admin dashboard (usage stats, user management, audit logs)
- Ollama/vLLM support (on-premises LLM)
- Message export
- Data retention policies

#### DB Changes

TBD

#### Excluded

TBD
