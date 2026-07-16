# Phase 20: Room Fork

**Goal** (`phases.md` Phase 20): Copy a room's conversation to a new room to create a branch. Delivered across two
steps: Step 32 (server) and Step 52 (web room fork UI — reconciled into this file at Step 60; it had not
previously been tracked here).

Step 32 tracks `docs/tasks/step32.md`'s scope: schema for `forked_from_room_id`/
`is_archived`/`room_fork_jobs`, bulk-copy repository primitives, the
goroutine-based fork worker, the fork/fork-job-status endpoints, and the
archived-room guard on new message posts.

## Scope

- [x] `server/schema.sql`: `rooms.forked_from_room_id`/`rooms.is_archived`
      columns + new `room_fork_jobs` table + `idx_room_fork_jobs_new_room`
      index. Migration generated via `task migrate:generate -- add_room_fork`
      (`server/migrations/20260716023744_add_room_fork.sql` +
      `server/migrations/atlas.sum`).
- [x] `server/internal/domain/room/entity.go`: `ForkedFromRoomID`/
      `IsArchived` added to `Room`.
- [x] `server/internal/domain/room/repository.go`: `SetArchived` added to
      `RoomRepository`.
- [x] `server/internal/interface/repository/postgres/room_repository.go`:
      `Create`/`GetByID`/`ListByUserID`/`ListByUserIDWithRole`/`Update`
      updated; `SetArchived` implemented.
- [x] `server/internal/domain/message/repository.go`: `CountByRoom`/
      `ListByRoomAfter`/`CreateBatch` added to `MessageRepository`.
- [x] `server/internal/interface/repository/postgres/message_repository.go`:
      the three methods implemented, reusing `messageColumns`/`scanMessage`.
- [x] `server/internal/domain/roomfork/entity.go` (new package): `Status`
      enum + `Job` struct.
- [x] `server/internal/domain/roomfork/repository.go` (new package):
      `ForkJobRepository` port.
- [x] `server/internal/interface/repository/postgres/room_fork_repository.go`
      (new file): `RoomForkRepository` implementing `ForkJobRepository`.
- [x] `server/internal/usecase/room/fork.go` (new file): `RoomUsecase`
      constructor extended to 3 args (`roomRepo`, `msgRepo`,
      `forkJobRepo`); `ForkRoom`, `GetForkJobStatus`, unexported
      `runForkJob` (batch copy + `in_response_to_message_id` remap across
      batch boundaries).
- [x] `server/internal/domain/errors.go`: `ErrArchivedRoom` added.
- [x] `server/internal/usecase/message/usecase.go`: archived-room guard in
      `SendMessage`/`SendAIMessage` (only change in this file for this
      step).
- [x] `server/internal/interface/handler/message_handler.go`:
      `handleMessageError` maps `ErrArchivedRoom` -> HTTP 409.
- [x] `server/internal/interface/handler/dto.go`: `RoomResponse` extended
      with `ForkedFromRoomID`/`IsArchived`; `ForkJobResponse`/
      `RoomForkResponse` added.
- [x] `server/internal/interface/handler/room_fork_handler.go` (new file):
      `RoomHandler.Fork` / `RoomHandler.GetForkJobStatus`.
- [x] `server/internal/app/container.go` /
      `server/internal/app/routes_room.go`: `forkJobRepo` wired,
      `NewRoomUsecase` call site updated, two additive routes
      (`POST /rooms/:roomId/fork`, `GET /rooms/:roomId/fork-jobs/:jobId`).
- [x] `server/internal/testutil/mocks/`: `RoomRepo.SetArchived`,
      `MessageRepo.CountByRoom`/`ListByRoomAfter`/`CreateBatch`, new
      `ForkJobRepo` mock.
- [x] Tests: `usecase/room/fork_test.go` (forbidden, successful
      `ForkRoom`, `GetForkJobStatus` membership rules, synchronous
      multi-batch `runForkJob` boundary-remap + defensive-nil-remap unit
      tests), `usecase/message/usecase_test.go` (archived-room guard
      cases), `interface/handler/room_fork_handler_test.go` (202/403/200/
      403), postgres integration tests (`room_repository_integration_test.go`
      `SetArchived`/`ForkedFromRoomID` extensions,
      `message_repository_integration_test.go` `CountByRoom`/
      `ListByRoomAfter`/`CreateBatch` extensions,
      `room_fork_repository_integration_test.go` new,
      `room_fork_integration_test.go` new — 1500-message, 2-batch,
      boundary-remap end-to-end test via the real async `ForkRoom` path).
- [x] GoDoc on all new/changed exported symbols; doc comment on the
      unexported `runForkJob`.
- [x] This progress file.

## Verification

- [x] `go build ./...`
- [x] `go vet ./...` / `gofmt -l .` (clean) / `golangci-lint run ./...`
      (0 issues)
- [x] `go test ./...` (all unit tests pass, no Docker required)
- [x] `task migrate:generate -- add_room_fork` (ran against Atlas's own
      Docker dev-database, not the compose stack)
- [x] `go test -tags=integration ./internal/interface/repository/postgres/...`
      (room/message/fork repository round-trips + the >1-batch end-to-end
      fork test, all against a Docker testcontainers Postgres)
- [ ] `task up` + curl smoke test against the running compose stack
      (skipped — requires the full Docker Compose stack on fixed ports;
      left for the post-merge integration review)

## Step 52: Web room fork UI

- [x] `web/src/features/rooms/types.ts`: `is_archived`/`forked_from_room_id` on `Room`; `ForkJob`/`RoomForkResponse`
      types mirroring Step 32's DTOs exactly
- [x] `forkRoom(roomId)` / `getForkJobStatus(roomId, jobId)` added to the rooms feature's existing typed API client
      module (no second parallel client introduced)
- [x] `useForkJobPolling` — polls fork-job status on a short interval, stopping once `status` is terminal
      (`completed`/`failed`)
- [x] `RoomSettingsDrawer.tsx`: "Fork room" section (admin/master only, same `canManage` gate as other admin
      actions), inline progress view driven by `useForkJobPolling`, "Open forked room" link on completion,
      "Try again" on failure
- [x] `ChatRoom.tsx`: archived-room read-only banner (no `MessageInput` rendered) when `room.is_archived`; clears
      automatically on next mount/room-id-change re-fetch, no extra background poll on every open room
- [x] Component tests: fork section visibility by role, click-to-fork → progress view, terminal completed/failed
      states
- [x] `web/e2e/room-fork.spec.ts` / `web/e2e/regression/room-fork.spec.ts`

### Verification run in this worktree (Step 60, Step 52)

- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run`
- [ ] `web/e2e/room-fork.spec.ts` against the live compose stack — requires the full E2E stack; skipped (post-merge
      integration review)
