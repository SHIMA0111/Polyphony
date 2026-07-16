# Phase 5: AI Context Control

**Goal**: Set per-message AI exclusion flags and estimate token counts.

**Delivered by**: `docs/tasks/step23.md` (Server: AI context control — soft delete, exclude flag, cutoff, context
builder). See that file for the full per-file implementation notes.

---

## Step 23: AI context control

- [x] `schema.sql`: `messages.is_deleted` / `messages.exclude_from_ai` (both `BOOLEAN NOT NULL DEFAULT false`),
      `rooms.ai_context_cutoff_at` (nullable `TIMESTAMPTZ`); migration via `task migrate:generate --
      add_ai_context_control_fields`
- [x] `domain/message.Message`: `IsDeleted`, `ExcludeFromAI` fields; `domain/room.Room`: `AIContextCutoffAt *time.Time`
- [x] `MessageRepository.Delete` documented as soft-delete (sets `is_deleted = true`, excluded from
      `ListByRoom`/`ListByRoomUpTo`/AI context, still fetchable via `GetByID`); new `UpdateExcludeFromAI` method
- [x] `postgres.MessageRepository` / `postgres.RoomRepository` updated for the new columns
- [x] `domain/ai/context_builder.go` — `ContextBuilder.Build` filters messages by exclude flag, cutoff datetime,
      deleted, and (later, Step 41) private visibility, before token counting
- [x] `RoomUsecase.UpdateAIContextCutoff` — Admin+ only (`ActionManageRoom`), reuses `Update`
- [x] `MessageUsecase`: exclude-toggle + soft-delete usecase methods (owner-or-admin rule on delete)
- [x] DTOs: `UpdateRoomAIContextCutoffRequest`, exclude/delete request/response shapes on `dto.go`
- [x] New routes registered additively in `routes_message.go` / `routes_room.go`: `PATCH
      /rooms/:roomId/ai-context-cutoff`, exclude-toggle and delete endpoints on messages
- [x] Unit tests for `ContextBuilder.Build`'s exact filter list, exclude-toggle, soft-delete (owner-or-admin),
      cutoff enforcement; testcontainers integration tests for the new columns

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./...`
- [x] `cd server && go test ./...`
- [x] `cd server && go test -tags=integration ./...`
- [ ] End-to-end curl walkthrough via `task up` — requires the compose stack; skipped (post-merge integration
      review)
