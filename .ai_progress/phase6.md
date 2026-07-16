# Phase 6: Multi-provider (Anthropic)

**Goal**: Use Anthropic models in addition to OpenAI.

**Delivered by**: `docs/tasks/step25.md` (Rust: Anthropic provider adapter), `docs/tasks/step24.md` (Server:
per-room AI provider/model settings). See those files for the full per-file implementation notes.

---

## Step 25: Anthropic provider adapter (Rust)

- [x] `llm-gateway/src/adapters/outbound/anthropic/` module (`mod.rs` + `request.rs` + `stream.rs`, mirroring the
      OpenAI adapter's submodule split)
- [x] Request mapping: system-message hoisting, `max_tokens` defaulting, role mapping, Anthropic-specific headers
      (`x-api-key`, `anthropic-version: 2023-06-01`, no `Authorization: Bearer`)
- [x] Error/usage mapping to domain `DomainError`, reusing Step 8's `http_retry::send_with_retry`/`RetryPolicy`
- [x] `ANTHROPIC_API_KEY` wired into config, DI (`main.rs` provider registration), `.env.example`,
      `docker-compose.yml`
- [x] Unit tests (system hoisting, role mapping, response mapping) + wiremock integration test
      (`llm-gateway/tests/anthropic_provider_test.rs`)
- [x] rustdoc on every new public item

## Step 24: Per-room AI provider/model settings (Go)

- [x] `schema.sql`: `rooms.ai_provider` / `rooms.ai_model` (both nullable `VARCHAR`); migration via
      `task migrate:generate -- add_room_ai_settings`
- [x] `domain/room.Room.AIProvider` / `.AIModel` (`*string`); repository read/write updated
- [x] `RoomUsecase.UpdateSettings` (`server/internal/usecase/room/settings.go`) — Admin+ only
      (`ActionManageRoom`), nil = unchanged, empty string = clear
- [x] `RoomHandler.UpdateSettings` (`server/internal/interface/handler/room_settings_handler.go`) — `PATCH
      /rooms/:roomId/settings`
- [x] `usecase/message/model_resolution.go` — `resolveModel`: request model > room `ai_model` > global default
      (`DEFAULT_AI_MODEL` env var, `Config.DefaultAIModel`)
- [x] `MessageUsecase.SendAIMessage`/`RegenerateAIMessage` updated to call `resolveModel` instead of the former
      hardcoded `defaultModel` constant
- [x] Unit tests: `settings_test.go` (RBAC matrix, nil-vs-empty-string semantics), `model_resolution_test.go`
      (three-tier precedence table)

## Verification run in this worktree (Step 60)

- [x] `cd llm-gateway && cargo build --all-targets && cargo clippy --all-targets -- -D warnings && cargo test`
- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [ ] Manual verification against a real Anthropic key / live compose stack — requires network credentials and the
      compose stack; skipped (post-merge integration review)
