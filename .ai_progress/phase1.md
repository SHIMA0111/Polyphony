# Phase 1: Project Skeleton + Minimal AI Chat

**Goal**: A minimal application where a single user can chat with AI from a browser

---

## LLM Gateway (Rust)

- [x] Initialize Cargo project (`llm-gateway/`) + add dependency crates
- [x] `config.rs` — Environment variable configuration loading
- [x] `domain/model.rs` — CompletionRequest, CompletionResponse, ModelInfo types
- [x] `domain/error.rs` — Domain error types
- [x] `domain/service.rs` — CompletionService (provider selection logic)
- [x] `ports/inbound/completion.rs` — CompletionUseCase trait
- [x] `ports/outbound/provider.rs` — LLMProvider trait
- [x] `ports/outbound/key_store.rs` — KeyStore trait
- [x] `adapters/outbound/openai.rs` — OpenAI API adapter
- [x] `adapters/outbound/env_key.rs` — API key retrieval from environment variables
- [x] `adapters/inbound/rest/` — REST API handlers (axum)
  - [x] `POST /completions` — Chat completion
  - [x] `GET /models` — Available model listing
  - [x] `GET /health` — Health check
- [x] `main.rs` — DI assembly + server startup
- [x] Unit tests (mock provider) — 10 passing
- [x] Manual verification (local testing with curl, etc.)

## Go API Server

- [x] Initialize Go module (`server/`)
- [x] `cmd/api/main.go` — Entry point, DI assembly
- [x] `internal/infrastructure/config/config.go` — Configuration loading
- [x] `internal/infrastructure/database/postgres.go` — DB connection pool
- [x] **Domain layer**
  - [x] `domain/user/entity.go` — User struct
  - [x] `domain/user/repository.go` — UserRepository interface
  - [x] `domain/auth/service.go` — AuthService interface
  - [x] `domain/room/entity.go` — Room, RoomMember structs
  - [x] `domain/room/repository.go` — RoomRepository interface
  - [x] `domain/message/entity.go` — Message, MessageType, MessageStatus structs
  - [x] `domain/message/repository.go` — MessageRepository interface
  - [x] `domain/ai/entity.go` — AIRequest, AIResponse structs
  - [x] `domain/ai/gateway.go` — LLMGateway interface
- [x] **Usecase layer**
  - [x] `usecase/auth/usecase.go` — Register, Login
  - [x] `usecase/room/usecase.go` — CreateRoom, GetRoom, ListRooms
  - [x] `usecase/message/usecase.go` — SendMessage, ListMessages (cursor pagination), SendAIMessage (saves status=failed placeholder on LLM failure)
  - [x] `usecase/message/usecase.go` — RegenerateAIMessage (always UPDATEs existing AI message, never creates new)
- [x] **Interface layer**
  - [x] `interface/handler/auth_handler.go` — POST /auth/register, POST /auth/login
  - [x] `interface/handler/room_handler.go` — CRUD API
  - [x] `interface/handler/message_handler.go` — Message send/list API
  - [x] `interface/handler/message_handler.go` — RegenerateAI handler (`POST /rooms/:roomId/messages/:messageId/regenerate`)
  - [x] `interface/middleware/auth.go` — JWT verification middleware
  - [x] `interface/repository/postgres/user_repository.go`
  - [x] `interface/repository/postgres/room_repository.go`
  - [x] `interface/repository/postgres/message_repository.go`
  - [x] `interface/gateway/llm_client.go` — LLM Gateway REST client
- [x] SimpleJWT implementation (argon2 + JWT issue/verify)
- [x] Unit tests (mock Repository/Gateway)
- [ ] Manual verification

## Web Frontend (Next.js)

- [ ] Initialize Next.js project (`web/`), Bulletproof React directory structure
- [ ] `src/lib/api.ts` — API client configuration
- [ ] `src/features/auth/` — Login & registration
  - [ ] `components/LoginForm.tsx`
  - [ ] `components/RegisterForm.tsx`
  - [ ] `api/actions.ts` — Server Actions
- [ ] `src/features/rooms/` — Room listing & creation
  - [ ] `components/RoomList.tsx`
  - [ ] `components/CreateRoomForm.tsx`
  - [ ] `api/actions.ts`
- [ ] `src/features/messages/` — Chat screen
  - [ ] `components/MessageList.tsx`
  - [ ] `components/MessageInput.tsx`
  - [ ] `api/actions.ts`
- [ ] `src/app/` — Routing
  - [ ] `(auth)/login/page.tsx`
  - [ ] `(auth)/register/page.tsx`
  - [ ] `(main)/rooms/page.tsx`
  - [ ] `(main)/rooms/[roomId]/page.tsx`
- [ ] Manual verification

## DB Migration

- [x] `migrations/` — golang-migrate initial migration
  - [x] `users` table
  - [x] `rooms` table (`owner_id` ON DELETE RESTRICT — cannot delete account without ownership transfer)
  - [x] `room_members` table (`user_id` ON DELETE RESTRICT — leave is handled explicitly by the app)
  - [x] `messages` table (`status` column: completed/failed, `sender_id` ON DELETE SET NULL)
  - [x] `room_sequences` table

## Infrastructure

- [ ] `docker-compose.yml` (PostgreSQL + Go API + Rust LLM Gateway + Next.js)
- [ ] Dockerfiles for each service
- [ ] `.env.example`

## Integration Test

- [ ] End-to-end flow via Docker Compose: register → login → create room → send message → get AI response
