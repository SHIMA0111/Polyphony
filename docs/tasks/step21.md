# Step 21: Server: room invitations

## Meta
- **Type**: feature
- **Components**: server
- **Wave**: 4 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 13: Go 5-tier RBAC domain + authorization middleware + ModelUsecase
- **Unlocks**: Step 37: Web members, invitations, and roles UI | Step 40: Server: groups + batch invitations
- **Size**: 1 PR (a few hours for one AI agent)

## Context
Phase 3 of `phases.md` ("Invitations + Member Management") requires that room owners/admins be able to bring new users into a room either by targeting a known username or by sharing a link, with the invitee accepting or rejecting before becoming a `room_members` row. Today `server/internal/usecase/room/usecase.go` only supports adding a member as a side effect of `CreateRoom` (the owner is inserted directly in `RoomRepository.Create`); there is no path for anyone else to join a room. This step adds that path as a self-contained domain/usecase/handler slice, reusing the typed `room.Role` and RBAC authorization primitives landed in Step 13 rather than re-deriving role-tier logic.

## Goal
After this PR, a room member with the Admin or Master role can create an invitation for a room, either targeted at an existing user by exact username or as a reusable shareable link code, with an assigned role and an expiry. The invited user (or, for link invitations, any authenticated user holding the code) can list their pending invitations and accept or reject them; accepting inserts a `room_members` row with the role recorded on the invitation. All new logic is covered by usecase unit tests (using Step 2's shared `testutil/mocks` package) and testcontainers-backed PostgreSQL repository integration tests (Step 2's shared helper).

## Scope
- [x] Add `room_invitations` table to `server/schema.sql` (see exact DDL in Implementation notes) and generate the Atlas migration with `task migrate:generate -- add_room_invitations`.
- [x] Add domain package `server/internal/domain/invitation/entity.go`: `Invitation` struct (`ID, RoomID, InviterID, InviteeID *string, InviteCode, Role, Status, ExpiresAt, CreatedAt`) plus a `Status` string type with constants `StatusPending`, `StatusAccepted`, `StatusRejected`, `StatusRevoked`.
- [x] Add `server/internal/domain/invitation/repository.go`: `InvitationRepository` interface with `Create`, `GetByID`, `GetByCode`, `GetPendingByRoomAndInvitee`, `ListByRoomID`, `ListPendingByInviteeID`, `UpdateStatus` (godoc on every method, documenting `domain.ErrNotFound` behavior per existing convention).
- [x] Extend `server/internal/domain/errors.go` with `ErrInvitationExpired`, `ErrInvitationNotPending`, `ErrAlreadyMember`, `ErrInvitationAlreadyExists` (godoc comments matching the existing style).
- [x] Implement `server/internal/interface/repository/postgres/invitation_repository.go` (`InvitationRepository` backed by `pgxpool.Pool`, following the exact patterns in `room_repository.go`: `pgx.ErrNoRows` → `domain.ErrNotFound`, plain SQL, no query builder).
- [x] Implement `server/internal/usecase/invitation/usecase.go` (`InvitationUsecase`) with:
  - `CreateInvitation(ctx, inviterID, roomID string, inviteeUsername *string, role room.Role, expiresInHours *int) (*invitation.Invitation, error)`
  - `ListRoomInvitations(ctx, userID, roomID string) ([]*invitation.Invitation, error)`
  - `ListMyInvitations(ctx, userID string) ([]*invitation.Invitation, error)`
  - `GetInvitationByCode(ctx, userID, code string) (*invitation.Invitation, error)`
  - `AcceptInvitation(ctx, userID, invitationID string) (*room.RoomMember, error)`
  - `RejectInvitation(ctx, userID, invitationID string) error`
  - Enforce RBAC: only room Admin+ (using the Step 13 role-comparison helper) may call `CreateInvitation`/`ListRoomInvitations`.
  - Enforce the privilege-escalation guard: the role assigned on an invitation must not exceed the inviter's own effective role in that room.
  - Enforce single-use semantics for username-targeted invitations (`InviteeID != nil`): accepting sets `status = accepted`; rejecting sets `status = rejected`; a second accept/reject on a non-pending invitation returns `domain.ErrInvitationNotPending`.
  - Treat link invitations (`InviteeID == nil`) as reusable until expiry or explicit revocation is added in a later step: accepting them does **not** change `status`, and `RejectInvitation` on a link invitation returns `domain.ErrForbidden`.
  - Generate a unique `invite_code` (see Implementation notes for the exact approach) and reject duplicate pending username invitations for the same `(room_id, invitee_id)` pair via `GetPendingByRoomAndInvitee`.
- [x] Implement `server/internal/interface/handler/invitation_handler.go` (`InvitationHandler`) wiring the six endpoints listed in Implementation notes, following the existing handler style in `room_handler.go` (`middleware.GetUserID`, `c.Bind`, explicit 400 validation, a `handleInvitationError` helper mirroring `handleRoomError`).
- [x] Add invitation DTOs to `server/internal/interface/handler/dto.go`: `CreateInvitationRequest`, `InvitationResponse`, `InvitationListResponse`, `RoomMembershipResponse` (godoc on each, following the existing `--- Section ---` comment grouping in that file).
- [x] Register the new repository/usecase/handler in the Step 1 DI container (`server/internal/app/container.go`) and add the six routes via a new registrar file `server/internal/app/routes_invitation.go` (exporting `registerInvitationRoutes(e *echo.Echo, c *Container)`, called from `router.go`, mirroring `routes_room.go`), on the same authenticated group the room/message registrars use.
- [x] Add unit tests for `InvitationUsecase` in `server/internal/usecase/invitation/usecase_test.go` using Step 2's shared mocks (`server/internal/testutil/mocks`) for `RoomRepository`/`UserRepository`, plus a new in-memory `InvitationRepository` mock (add it to the shared `mocks` package so Step 40 can reuse it). Cover: create by username, create as link, RBAC rejection (non-admin inviter), privilege-escalation rejection, accept (username + link), accept on expired invitation, accept twice on a username invitation, reject, reject-on-link-invitation rejection, duplicate pending invite rejection.
- [x] Add unit tests for `InvitationHandler` in `server/internal/interface/handler/invitation_handler_test.go`, following the style in `room_handler_test.go` / `message_handler_test.go` (reuse or extend the shared mocks in `server/internal/testutil/mocks` where practical).
- [x] Add testcontainers-backed integration tests for `InvitationRepository` in `server/internal/interface/repository/postgres/invitation_repository_test.go` (build-tagged `integration`, using Step 2's shared `server/internal/testutil/postgres` helper — which starts a `postgres:17` container and applies `server/migrations/*.sql` — rather than standing up a new container harness), covering `Create`/`GetByID`/`GetByCode`/`GetPendingByRoomAndInvitee`/`ListByRoomID`/`ListPendingByInviteeID`/`UpdateStatus` against a real PostgreSQL instance. Do not invent an ad hoc mocking substitute for this file.

## Out of scope
- Room member list endpoint (`GET /rooms/:roomId/members`) and member removal (`DELETE /rooms/:roomId/members/:userId`) — not part of this step's `keyScope`; a companion wave-4 step owns them.
- Ownership transfer endpoint (`PATCH /rooms/:roomId/owner`) — separate concern from invitations.
- Role change API / the RBAC middleware and `room.Role` type itself — landed in Step 13, only consumed here.
- Invitation revocation endpoint and any "usage count" tracking on link invitations — link invitations are simply left `pending`/reusable in this step; explicit revoke can be added later without a schema change (`status = revoked` already exists as a value).
- Group/batch invitations — Step 40.
- Any web/UI work for invitations — Step 37.
- WebSocket/event notifications on invitation create/accept — not part of this step; `domain/event.MessageHub` wiring for invitations, if desired, is a follow-up.

## Implementation notes

**Schema** — append to `server/schema.sql` (after the `messages` table / its index, matching the existing style of that file):

```sql
CREATE TABLE room_invitations (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    inviter_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    invitee_id UUID REFERENCES users(id) ON DELETE CASCADE,
    invite_code VARCHAR(64) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT room_invitations_invite_code_unique UNIQUE (invite_code)
);

CREATE INDEX idx_room_invitations_room_id ON room_invitations(room_id);
CREATE INDEX idx_room_invitations_invitee_id ON room_invitations(invitee_id) WHERE invitee_id IS NOT NULL;
```

Generate the migration with `task migrate:generate -- add_room_invitations` (runs `atlas migrate diff add_room_invitations --env local` per `server/atlas.hcl`, using the `docker://postgres/17/dev` dev database to diff). This produces a new file under `server/migrations/` plus an updated `server/migrations/atlas.sum` — do not hand-edit either. Per the project-wide schema convention, this migration only adds a brand-new table, so it cannot conflict with other wave-4 steps' schema edits.

**RBAC dependency (Step 13)** — Step 13 lands a typed `room.Role` enum (five tiers: Reader < Guest < Member < Admin < Master) in `server/internal/domain/room/`, a role-comparison helper (e.g. a method like `Role.AtLeast(other room.Role) bool` or equivalent), and an authorization primitive (either RBAC middleware in `server/internal/interface/middleware/` or a usecase-level helper alongside `RoomUsecase.checkMembership` in `server/internal/usecase/room/usecase.go`). Locate whatever Step 13 actually named these symbols as (do not guess at exact identifiers before reading that code) and reuse them directly in `InvitationUsecase` for:
1. Gating `CreateInvitation`/`ListRoomInvitations` to the inviter's role being Admin or Master in the target room (look up the caller's membership via `room.RoomRepository.GetMember`, matching the pattern already in `RoomUsecase.checkMembership` in `server/internal/usecase/room/usecase.go:93`).
2. The privilege-escalation guard: reject `CreateInvitation` with `domain.ErrForbidden` if the requested `role` outranks the inviter's own role.

Do not reimplement role-tier comparison logic from scratch in this step's package.

**Invite code generation** — no new dependency is needed; follow the existing use of `github.com/google/uuid` elsewhere in this codebase (e.g. `server/internal/usecase/room/usecase.go:29`) and derive the code as the UUID's hex form with dashes stripped (`strings.ReplaceAll(uuid.New().String(), "-", "")`), which is URL-safe and collision-resistant enough for this scope. Rely on the `room_invitations_invite_code_unique` DB constraint as the final backstop; on a unique-violation from `Create`, retry generation once, matching the level of defensiveness used elsewhere in this codebase (no elaborate retry loop).

**Expiry** — `CreateInvitationRequest.ExpiresInHours` is optional; default to 168 (7 days) when omitted, clamp to the range `[1, 720]` hours (reject out-of-range with HTTP 400). `expires_at = time.Now().Add(time.Duration(hours) * time.Hour)`. Expiry is checked lazily at accept time (`AcceptInvitation` returns `domain.ErrInvitationExpired` if `time.Now().After(inv.ExpiresAt)`); there is no background sweep job in this step — `ListMyInvitations`/`ListRoomInvitations` may still return already-expired-but-still-`pending` rows, and the caller (web UI, in a later step) is expected to render them as expired using `expires_at`.

**Endpoints** (register via the new `server/internal/app/routes_invitation.go` registrar on the same authenticated group the existing room/message registrars use):

| Method | Path | Handler | RBAC | Notes |
|---|---|---|---|---|
| POST | `/rooms/:roomId/invitations` | `Create` | inviter must be Admin+ in room | body `CreateInvitationRequest{invitee_username *string, role string, expires_in_hours *int}`; 201 + `InvitationResponse` |
| GET | `/rooms/:roomId/invitations` | `ListForRoom` | caller must be Admin+ in room | 200 + `InvitationListResponse` |
| GET | `/invitations` | `ListMine` | any authenticated user | pending invitations where `invitee_id = caller`; 200 + `InvitationListResponse` |
| GET | `/invitations/by-code/:code` | `GetByCode` | any authenticated user | resolves a link/username invitation by its code for preview; 200 + `InvitationResponse`, 404 if code unknown |
| POST | `/invitations/:invitationId/accept` | `Accept` | invitee (or any user for link invitations) | 200 + `RoomMembershipResponse`; 403 if targeted at a different user, 409 if already a member or not pending, 410 if expired |
| POST | `/invitations/:invitationId/reject` | `Reject` | invitee only (link invitations: 400) | 204 no content |

Error mapping in `handleInvitationError` (mirror `handleRoomError` in `server/internal/interface/handler/room_handler.go:155`): `domain.ErrNotFound` → 404, `domain.ErrForbidden` → 403, `domain.ErrAlreadyMember` → 409, `domain.ErrInvitationAlreadyExists` → 409, `domain.ErrInvitationNotPending` → 409, `domain.ErrInvitationExpired` → 410, default → 500.

**Conventions to follow**: GoDoc on every exported type/function/method (what/why/error behavior, matching the density already in `server/internal/domain/room/repository.go` and `server/internal/interface/handler/room_handler.go`); English-only identifiers/comments; `slog` is already global via `slog.SetDefault` in `main.go` — do not introduce a new logging pattern, just use `slog.Error`/`slog.Info` in the handler layer only if genuinely useful (existing handlers mostly don't log per-request, stay consistent). Domain layer (`internal/domain/invitation`) must not import `internal/infrastructure/*` or `internal/interface/*` packages, matching Clean Architecture rules in `CLAUDE.md`.

**Conflict notes**: This step only adds new `invitation_*` files and a new `room_invitations` table; it does not modify `room_members`, `messages`, or any file also touched by other wave-4 steps. The only shared files touched are `server/schema.sql` (additive table), `server/internal/app/container.go`/`router.go` (additive DI field/registrar-call lines; the routes themselves live in the new `routes_invitation.go`), and `server/internal/domain/errors.go` (additive error vars) — all append-only edits, consistent with the project-wide same-wave ownership convention.

## Verification
1. `cd server && go build ./...` — compiles cleanly.
2. `cd server && go vet ./...` — no issues.
3. `task test:server` (equivalently `cd server && go test ./...`) — all unit tests pass, including the new `internal/usecase/invitation` and `internal/interface/handler` tests.
4. `cd server && go test ./internal/interface/repository/postgres/... -run TestInvitation -v` — the testcontainers-backed `InvitationRepository` tests pass (requires a local Docker daemon, consistent with other testcontainers usage in this codebase).
5. `task migrate:generate -- add_room_invitations` followed by `git diff server/migrations` shows a single new migration file that only creates `room_invitations` (plus its indexes), and `server/migrations/atlas.sum` is regenerated (not hand-edited).
6. End-to-end smoke test via `task up` then, with two registered users `alice` (room Admin+) and `bob`:
   - `curl -s -X POST localhost:8080/rooms/$ROOM_ID/invitations -H "Authorization: Bearer $ALICE_TOKEN" -H "Content-Type: application/json" -d '{"invitee_username":"bob","role":"member"}'` → HTTP 201 with an `InvitationResponse` containing a non-empty `invite_code` and `status: "pending"`.
   - `curl -s localhost:8080/invitations -H "Authorization: Bearer $BOB_TOKEN"` → HTTP 200, the invitation created above appears.
   - `curl -s -X POST localhost:8080/invitations/$INVITATION_ID/accept -H "Authorization: Bearer $BOB_TOKEN"` → HTTP 200 with a `RoomMembershipResponse`; a repeat call → HTTP 409.
   - `curl -s localhost:8080/rooms/$ROOM_ID -H "Authorization: Bearer $BOB_TOKEN"` → HTTP 200 (bob can now access the room, confirming the `room_members` row was inserted).

## Completion criteria
- [x] `room_invitations` table exists in `server/schema.sql` with a generated Atlas migration checked in.
- [x] `invitation` domain package, `InvitationUsecase`, `InvitationRepository` (postgres), and `InvitationHandler` are implemented and wired into the `internal/app` container/registrar.
- [x] All six endpoints listed above are routed and behave per the RBAC/status rules in this document.
- [x] Privilege-escalation guard (role assigned ≤ inviter's role) and single-use-vs-reusable status semantics (username vs. link invitations) are implemented and unit-tested.
- [x] Unit tests (usecase + handler, hand-written mocks) and testcontainers integration tests (repository) are added and pass.
- [ ] All verification checks above pass. (1–5 pass in this worktree; 6 requires the full `task up` compose stack and is left for the post-merge integration review.)
