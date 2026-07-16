# Phase 3: Invitations + Member Management

**Goal**: Room owners can invite users and view the member list.

**Delivered by**: `docs/tasks/step21.md` (Server: room invitations), `docs/tasks/step22.md` (Server: member
management, leave, role change, ownership transfer). Reconciled here (Step 60) — see those files for the full
per-file implementation notes; this checklist is the condensed, actually-merged view.

---

## Step 21: Room invitations

- [x] `server/schema.sql`: `room_invitations` table (`invite_code` unique, `status` pending/accepted/rejected/revoked,
      `expires_at`) + indexes; Atlas migration generated via `task migrate:generate -- add_room_invitations`
- [x] `server/internal/domain/invitation/entity.go` — `Invitation` struct + `Status` constants
- [x] `server/internal/domain/invitation/repository.go` — `InvitationRepository` port (`Create`, `GetByID`,
      `GetByCode`, `GetPendingByRoomAndInvitee`, `ListByRoomID`, `ListPendingByInviteeID`, `UpdateStatus`)
- [x] `server/internal/domain/errors.go` — `ErrInvitationExpired`, `ErrInvitationNotPending`, `ErrAlreadyMember`,
      `ErrInvitationAlreadyExists`
- [x] `server/internal/interface/repository/postgres/invitation_repository.go` — Postgres-backed implementation
- [x] `server/internal/usecase/invitation/usecase.go` — `InvitationUsecase`: create (username or link, RBAC-gated to
      Admin+, privilege-escalation guard), list (room + mine), get-by-code, accept, reject; single-use semantics for
      username invitations, reusable-until-expiry semantics for link invitations
- [x] `server/internal/interface/handler/invitation_handler.go` + DTOs — six endpoints (`POST/GET
      /rooms/:roomId/invitations`, `GET /invitations`, `GET /invitations/by-code/:code`, `POST
      /invitations/:invitationId/accept`, `POST /invitations/:invitationId/reject`)
- [x] DI wiring in `container.go` + `server/internal/app/routes_invitation.go` registrar
- [x] Unit tests (`usecase/invitation`, `interface/handler`) and testcontainers integration tests
      (`invitation_repository_integration_test.go`)

## Step 22: Member management, leave, role change, ownership transfer

- [x] `domainroom.ErrOwnerRoleProtected` — returned when the owner tries to leave without transferring, or someone
      tries to change the owner's role directly
- [x] `RoomRepository.UpdateMemberRole` / `TransferOwnership` (atomic: new owner → `master`, old owner → `admin`)
- [x] `RoomUsecase.ListMembers` (any member, `Username` populated via `JOIN users`), `LeaveRoom` (self-leave only,
      owner-protected), `ChangeMemberRole` (Admin+ only, cannot grant `master` via this path), `TransferOwnership`
      (owner-only, target must already be a member, idempotent no-op if unchanged)
- [x] `MemberResponse`/`MemberListResponse`/`ChangeMemberRoleRequest`/`TransferOwnershipRequest` DTOs
- [x] `RoomHandler.ListMembers` / `Leave` / `ChangeRole` / `TransferOwnership` — `GET /rooms/:roomId/members`,
      `DELETE /rooms/:roomId/members/:userId`, `PATCH /rooms/:roomId/members/:userId/role`, `PATCH
      /rooms/:roomId/owner`
- [x] Routes registered additively in `routes_room.go`; `RequireRole(ActionManageMembers)` middleware on the
      role-change route only
- [x] Unit tests (usecase RBAC-denial matrix, handler status codes) and Postgres integration tests for
      `UpdateMemberRole`/`TransferOwnership` atomicity

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./...`
- [x] `cd server && go test ./...` (all packages, including `usecase/invitation`, `usecase/room`, `interface/handler`)
- [x] `cd server && go test -tags=integration ./...` (Docker testcontainers, includes invitation/room repository tests)
- [ ] End-to-end curl walkthrough (`task up` + live HTTP calls) — requires the fixed-port compose stack; skipped
      per this run's constraints (post-merge integration review)
