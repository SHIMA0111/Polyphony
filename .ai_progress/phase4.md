# Phase 4: Basic RBAC

**Goal**: 3-tier role-based access control with reader/member/master.

**Deviation (see `phases.md` → Deviations from this plan, item 1)**: Phase 4 and Phase 11 ("Full RBAC — 5-tier")
were implemented together in a single step, `docs/tasks/step13.md` ("Go 5-tier RBAC domain + authorization
middleware + ModelUsecase"), as one typed 5-tier `room.Role` enum from the start. There was never an intermediate
3-role-only state in the merged codebase. See `.ai_progress/phase11.md` for the pointer back to this file — the two
phase files describe the same merged work and should be read together.

---

## Step 13: 5-tier RBAC domain + authorization middleware + ModelUsecase

- [x] `server/internal/domain/room/role.go` — `type Role string` with `RoleReader < RoleGuest < RoleMember <
      RoleAdmin < RoleMaster`, an ordered ranking, and helper methods (`IsValid`, rank comparison)
- [x] `Action` enum (`ActionSendMessage`, `ActionInvokeAI`, `ActionManageMembers`, `ActionManageRoom`,
      `ActionDeleteRoom`) with `Role.Allows(Action)` encoding the full capability matrix: reader → none; guest →
      send only; member → send + invoke AI; admin → all except delete; master → all
- [x] `domainroom.RoomMember.Role` changed from `string` to the typed `Role`; every reference (repository,
      usecases, tests) updated to convert DB `VARCHAR` via `Role(stringValue)`
- [x] `server/internal/interface/middleware/rbac.go` — reusable `Authorize`/`RequireRole` authorization primitive
      built on `RoomRepository`
- [x] Shared membership+role lookup helper replacing the former duplicated `RoomUsecase.checkMembership` /
      `MessageUsecase.checkMembership`
- [x] `postgres.RoomRepository`: owner membership inserted with the typed `RoleMaster` constant;
      `GetMember`/`ListMembers`/`AddMember` scan/bind through `Role`
- [x] `schema.sql`: `CHECK` constraint on `room_members.role` restricting it to the five values; migration via
      `task migrate:generate -- add_room_members_role_check`
- [x] `server/internal/usecase/model/usecase.go` — `ModelUsecase` wrapping `ai.LLMGateway.ListModels` (room for
      future per-room filtering in Step 24)
- [x] `ModelHandler` updated to depend on `ModelUsecase` instead of `ai.LLMGateway` directly
- [x] `RoomResponse.Role` field (the requesting user's role in that room), surfaced via a `RoomWithRole` wrapper
      returned by `GetRoom`/`UpdateRoom`/`CreateRoom`/`ListRooms` (single-query `ListByUserIDWithRole`, not N+1)
- [x] Unit tests updated across `usecase/room`, `usecase/message`, `interface/handler` for the new `Role` type and
      DTO field; new tests cover the full reader/guest/member/admin/master capability matrix

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./...`
- [x] `cd server && go test ./...` (RBAC matrix tests all pass)
- [x] `cd server && go test -tags=integration ./...`
- [ ] `task migrate:status` / live compose migration apply — requires the compose `db` service; skipped (post-merge
      integration review)
