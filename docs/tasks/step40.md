# Step 40: Server: groups + batch invitations

## Meta
- **Type**: feature
- **Components**: server
- **Wave**: 5 (parallel group — all steps of the same wave can run concurrently)
- **Depends on**: Step 21: Server: room invitations
- **Unlocks**: Step 46: Web group management UI
- **Size**: 1 PR (a few hours for one AI agent)

## Context
`phases.md` Phase 13 ("Group Management + Batch Invitations") lets a user maintain personal address-book-style groups of other users and invite an entire group to a room in a single call, instead of repeating Step 21's single-target `CreateInvitation` once per person. Step 21 already ships `InvitationUsecase.CreateInvitation` (username-targeted invitations, RBAC-gated to room Admin+, with the privilege-escalation guard) and the `room_invitations` table; this step does not re-derive any of that — it adds a new `group`/`group_members` domain slice and a thin usecase that fans a single batch request out into repeated calls to the existing `InvitationUsecase.CreateInvitation`, collecting per-member results instead of failing the whole batch on the first duplicate or already-a-member case.

## Goal
After this PR, any authenticated user can create, list, update, and delete personal groups they own, and manage each group's membership (add/remove by exact username, list members with usernames resolved). A room Admin+ can call a new batch-invitation endpoint with a `group_id` they own and a target role; the server creates one `room_invitations` row per group member (reusing `InvitationUsecase.CreateInvitation` unchanged), returning both the successfully created invitations and a per-member skip list (with a reason) for members who are already room members, already have a pending invitation, or otherwise fail — a single RBAC failure (caller lacks Admin+ in the room) is returned as one top-level error rather than N per-member skips. All new logic is covered by usecase unit tests (using `server/internal/testutil/mocks`, per Step 2's shared-mock convention) and testcontainers-backed PostgreSQL repository integration tests.

## Scope
- [x] Add `groups` and `group_members` tables to `server/schema.sql` (see exact DDL in Implementation notes) and generate the Atlas migration with `task migrate:generate -- add_groups_and_group_members`.
- [x] Add domain package `server/internal/domain/group/entity.go`: `Group` struct (`ID, OwnerID, Name, Description, CreatedAt, UpdatedAt`), `GroupMember` struct (`ID, GroupID, UserID, AddedAt`), and `GroupMemberWithUsername` struct (embeds `GroupMember` plus `Username string`) so the repository can return group membership already joined with the owning user's username in one query.
- [x] Add `server/internal/domain/group/repository.go`: `GroupRepository` interface with `Create`, `GetByID`, `ListByOwnerID`, `Update`, `Delete`, `AddMember`, `GetMember`, `ListMembers` (returns `[]*GroupMemberWithUsername`), `RemoveMember` (godoc on every method, documenting `domain.ErrNotFound` behavior per the existing convention in `server/internal/domain/room/repository.go`).
- [x] Implement `server/internal/interface/repository/postgres/group_repository.go` (`GroupRepository` backed by `pgxpool.Pool`, following the exact patterns in `room_repository.go`: `pgx.ErrNoRows` → `domain.ErrNotFound`, plain SQL, no query builder; `ListMembers` uses a single `JOIN` against `users` to resolve usernames, matching the single-query preference established by Step 13's `ListByUserIDWithRole`).
- [x] Implement `server/internal/usecase/group/usecase.go` (`GroupUsecase`) with:
  - `CreateGroup(ctx, ownerID, name, description string) (*group.Group, error)`
  - `GetGroup(ctx, callerID, groupID string) (*group.Group, error)` — owner-only, `domain.ErrForbidden` otherwise.
  - `ListGroups(ctx, ownerID string) ([]*group.Group, error)`
  - `UpdateGroup(ctx, callerID, groupID, name, description string) (*group.Group, error)` — owner-only.
  - `DeleteGroup(ctx, callerID, groupID string) error` — owner-only; cascades to `group_members` via `ON DELETE CASCADE`.
  - `AddMember(ctx, callerID, groupID, username string) (*group.GroupMemberWithUsername, error)` — owner-only; resolves `username` via `user.UserRepository.GetByUsername` (`domain.ErrNotFound` if unknown); `domain.ErrAlreadyMember` if the user is already in the group.
  - `ListMembers(ctx, callerID, groupID string) ([]*group.GroupMemberWithUsername, error)` — owner-only.
  - `RemoveMember(ctx, callerID, groupID, userID string) error` — owner-only.
  - `BatchInviteToRoom(ctx, callerID, roomID, groupID string, role room.Role, expiresInHours *int) (*BatchInviteResult, error)` — verifies `callerID` owns `groupID` (`domain.ErrForbidden` otherwise), loads the group's members via `ListMembers`, then calls the existing `invitationusecase.InvitationUsecase.CreateInvitation(ctx, callerID, roomID, &member.Username, role, expiresInHours)` once per member. If the very first call returns `domain.ErrForbidden` (caller lacks Admin+ in the room — the same RBAC check applies identically to every member since it depends only on `callerID`/`roomID`), return that error immediately without iterating further. Any other per-member error (`domain.ErrAlreadyMember`, `domain.ErrInvitationAlreadyExists`, etc.) is recorded as a skip (with the error message as the reason) and iteration continues. Do not reimplement `CreateInvitation`'s RBAC/privilege-escalation/duplicate-detection logic in this method.
  - Define `BatchInviteResult` (`Invited []*invitation.Invitation`, `Skipped []BatchInviteSkip`) and `BatchInviteSkip` (`UserID, Username, Reason string`) in the same package.
- [x] Implement `server/internal/interface/handler/group_handler.go` (`GroupHandler`) wiring the endpoints listed in Implementation notes, following the existing handler style in `room_handler.go` (`middleware.GetUserID`, `c.Bind`, explicit 400 validation, a `handleGroupError` helper mirroring `handleRoomError`).
- [x] Implement the batch-invitation handler method (`BatchInviteByGroup`) on `GroupHandler` for `POST /rooms/:roomId/invitations/batch-by-group`.
- [x] Add group DTOs to `server/internal/interface/handler/dto.go`: `CreateGroupRequest`, `UpdateGroupRequest`, `GroupResponse`, `GroupListResponse`, `AddGroupMemberRequest`, `GroupMemberResponse`, `GroupMemberListResponse`, `BatchInviteByGroupRequest`, `BatchInviteSkipResponse`, `BatchInviteByGroupResponse` (godoc on each, following the existing `--- Section ---` comment grouping in that file; `BatchInviteByGroupResponse.Invited` reuses the `InvitationResponse` type Step 21 added to this file).
- [x] Register the new repository/usecase/handler in the DI container (`server/internal/app/container.go` if Step 1's app-container refactor is in place, otherwise wherever the equivalent wiring currently lives — locate the actual current call site before editing rather than assuming) and add a new route registrar `server/internal/app/routes_group.go` (or the equivalent location for route registration) exporting the group CRUD/member routes plus the batch-invitation route under the existing authenticated group.
- [x] Add unit tests for `GroupUsecase` in `server/internal/usecase/group/usecase_test.go`, using `server/internal/testutil/mocks` (per Step 2's shared-mock convention — extend that package with a `GroupRepo` fake if one is not already present, and reuse its existing `RoomRepo`/`UserRepo`/invitation-repository fakes to construct a real `invitationusecase.InvitationUsecase` for `BatchInviteToRoom` tests). Cover: create/get/list/update/delete group (owner vs. non-owner `domain.ErrForbidden`), add member by username (success, unknown username, duplicate member), list members (usernames resolved), remove member, batch invite success (all members invited), batch invite with a mix of successes and skips (one member already a room member, one with a duplicate pending invite), batch invite short-circuiting on RBAC failure (caller not Admin+ in the room), batch invite on a group the caller doesn't own (`domain.ErrForbidden`, zero `CreateInvitation` calls made).
- [x] Add unit tests for `GroupHandler` in `server/internal/interface/handler/group_handler_test.go`, following the style in `room_handler_test.go` (reuse or extend the shared mocks in `mock_test.go`/`testutil/mocks` where practical).
- [x] Add testcontainers-backed integration tests for `GroupRepository` in `server/internal/interface/repository/postgres/group_repository_test.go` (using the `server/internal/testutil/postgres` helper from Step 2), covering `Create`/`GetByID`/`ListByOwnerID`/`Update`/`Delete`/`AddMember`/`GetMember`/`ListMembers` (verifying the username join)/`RemoveMember` against a real PostgreSQL instance.

## Out of scope
- Web/UI for group management or batch invitations — Step 46.
- Shared/team groups spanning multiple owners, or nested groups — groups in this step are strictly personal (single `owner_id`), matching `phases.md` Phase 13's scope.
- Any change to `InvitationUsecase.CreateInvitation`'s signature, RBAC rules, or privilege-escalation guard — this step calls it as-is, once per group member.
- Invitation revocation, "usage count" tracking, or link-style (non-username) invitations — unchanged from Step 21.
- Notifications on group invite/batch-invite (e.g. via `domain/event.MessageHub`) — not part of this step.
- Any role or permission concept on `group_members` itself — group membership is a flat list; per-room role assignment for invited users is chosen once, per batch call, via the `role` field on the batch-invite request, not stored per group member.
- Rate limiting or throttling of batch invitations — no limit on group size is enforced beyond what the database and existing `CreateInvitation` logic already impose.

## Implementation notes

**Schema** — append to `server/schema.sql` (after the `room_invitations` table added by Step 21, matching the existing style of that file):

```sql
CREATE TABLE groups (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE group_members (
    id UUID PRIMARY KEY,
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(group_id, user_id)
);

CREATE INDEX idx_groups_owner_id ON groups(owner_id);
CREATE INDEX idx_group_members_group_id ON group_members(group_id);
```

Generate the migration with `task migrate:generate -- add_groups_and_group_members` (runs `atlas migrate diff add_groups_and_group_members --env local` per `server/atlas.hcl`). This produces a new file under `server/migrations/` plus an updated `server/migrations/atlas.sum` — do not hand-edit either. Per the project-wide schema convention, this migration only creates two brand-new tables, so it cannot conflict with any other wave-5 step's schema edits (wave-5 additions 40/49 each create only their own new, disjoint tables).

**Reusing Step 21's `InvitationUsecase`** — do not copy or re-derive `CreateInvitation`'s RBAC check (inviter must be Admin+ in the target room), privilege-escalation guard, or duplicate-pending-invite detection (`GetPendingByRoomAndInvitee`). `GroupUsecase.BatchInviteToRoom` takes a constructed `*invitationusecase.InvitationUsecase` as a dependency (via its constructor) and calls its existing `CreateInvitation(ctx, inviterID, roomID, inviteeUsername *string, role room.Role, expiresInHours *int) (*invitation.Invitation, error)` method once per group member, exactly as a human operator would call the single-invite endpoint repeatedly. If Step 21 named or shaped `CreateInvitation` differently than assumed here, adapt to whatever it actually exports — read `server/internal/usecase/invitation/usecase.go` before wiring this dependency rather than guessing at the exact signature.

**Errors** — reuse `domain.ErrAlreadyMember` (added by Step 21 in `server/internal/domain/errors.go`) for the "user is already a member of this group" case in `AddMember`; it is a generic enough name to cover both room and group membership duplication, so do not add a second near-identical error constant.

**Endpoints** (register in the authenticated route group, alongside the existing room/invitation routes):

| Method | Path | Handler | Authorization | Notes |
|---|---|---|---|---|
| POST | `/groups` | `Create` | any authenticated user | body `CreateGroupRequest{name, description}`; 201 + `GroupResponse` |
| GET | `/groups` | `List` | any authenticated user | groups owned by the caller; 200 + `GroupListResponse` |
| GET | `/groups/:groupId` | `Get` | group owner only | 200 + `GroupResponse`; 403 if not owner, 404 if not found |
| PUT | `/groups/:groupId` | `Update` | group owner only | body `UpdateGroupRequest{name, description}`; 200 + `GroupResponse` |
| DELETE | `/groups/:groupId` | `Delete` | group owner only | 204 no content |
| POST | `/groups/:groupId/members` | `AddMember` | group owner only | body `AddGroupMemberRequest{username}`; 201 + `GroupMemberResponse`; 404 if username unknown, 409 if already a member |
| GET | `/groups/:groupId/members` | `ListMembers` | group owner only | 200 + `GroupMemberListResponse` |
| DELETE | `/groups/:groupId/members/:userId` | `RemoveMember` | group owner only | 204 no content |
| POST | `/rooms/:roomId/invitations/batch-by-group` | `BatchInviteByGroup` | caller must own the group **and** be Admin+ in the room | body `BatchInviteByGroupRequest{group_id, role, expires_in_hours *int}`; 200 + `BatchInviteByGroupResponse{invited, skipped}`; 400 if `role` is invalid/`master`; 403 if caller doesn't own the group or lacks Admin+ in the room |

Validate `role` at the handler layer the same way Step 22's `ChangeRole` does: `domainroom.Role(req.Role).IsValid()` and `role != domainroom.RoleMaster`, returning HTTP 400 otherwise — `master` may never be granted through an invitation path.

Error mapping in `handleGroupError` (mirror `handleRoomError` in `server/internal/interface/handler/room_handler.go:155`): `domain.ErrNotFound` → 404, `domain.ErrForbidden` → 403, `domain.ErrAlreadyMember` → 409, default → 500. `BatchInviteByGroup` reuses this same mapper for the single top-level error case (e.g. RBAC failure); per-member skip reasons are returned inline in the 200 response body, not as HTTP error statuses.

**Conventions to follow**: GoDoc on every exported type/function/method (what/why/error behavior, matching the density in `server/internal/domain/room/repository.go` and `server/internal/interface/handler/room_handler.go`); English-only identifiers/comments; domain layer (`internal/domain/group`) must not import `internal/infrastructure/*` or `internal/interface/*` packages, matching Clean Architecture rules in `CLAUDE.md`. Follow the shared-mock convention from Step 2 (`server/internal/testutil/mocks`) rather than hand-writing a new mock repository type in a test file.

**Conflict notes**: This step only adds new `group_*` files, a new route registrar, and new `groups`/`group_members` tables; it does not modify `room_members`, `messages`, or `room_invitations`. The only shared files touched are `server/schema.sql` (additive tables), the DI container/route-registration entry point (additive lines), and — if `BatchInviteToRoom`'s constructor needs a reference to the already-built `InvitationUsecase` — the container wiring that threads it through (still an additive line, not a change to `InvitationUsecase`'s own file). This matches the plan-wide convention that wave-5 additions 40/49 create only new, disjoint tables and files.

## Verification
1. [x] `cd server && go build ./...` — compiles cleanly.
2. [x] `cd server && go vet ./...` — no issues.
3. [x] `task test:server` (equivalently `cd server && go test ./...`) — all unit tests pass, including the new `internal/usecase/group` and `internal/interface/handler` tests.
4. [x] `cd server && go test -tags=integration ./internal/interface/repository/postgres/... -run TestGroup -v` — the testcontainers-backed `GroupRepository` tests pass (requires a local Docker daemon).
5. [x] `task migrate:generate -- add_groups_and_group_members` followed by `git diff server/migrations` shows a single new migration file that only creates `groups`, `group_members`, and their indexes, and `server/migrations/atlas.sum` is regenerated (not hand-edited).
6. [ ] End-to-end smoke test via `task up` then, with two registered users `alice` (room Admin+, group owner) and `bob`/`carol` (existing users, not yet room members):
   - `curl -s -X POST localhost:8080/groups -H "Authorization: Bearer $ALICE_TOKEN" -H "Content-Type: application/json" -d '{"name":"Team","description":"my team"}'` → HTTP 201 with a `GroupResponse`; capture `$GROUP_ID`.
   - `curl -s -X POST localhost:8080/groups/$GROUP_ID/members -H "Authorization: Bearer $ALICE_TOKEN" -H "Content-Type: application/json" -d '{"username":"bob"}'` → HTTP 201; repeat for `carol` → HTTP 201.
   - `curl -s localhost:8080/groups/$GROUP_ID/members -H "Authorization: Bearer $ALICE_TOKEN"` → HTTP 200, both `bob` and `carol` present with usernames resolved.
   - `curl -s -X POST localhost:8080/rooms/$ROOM_ID/invitations/batch-by-group -H "Authorization: Bearer $ALICE_TOKEN" -H "Content-Type: application/json" -d '{"group_id":"'$GROUP_ID'","role":"member"}'` → HTTP 200 with `BatchInviteByGroupResponse.invited` containing two entries (bob, carol) and an empty `skipped` list.
   - Repeating the same batch-invite call → HTTP 200 with `invited` empty and `skipped` containing both `bob` and `carol` with a duplicate-pending-invite reason (proving per-member skip behavior instead of a whole-batch failure).
   - `curl -s -X POST localhost:8080/rooms/$ROOM_ID/invitations/batch-by-group -H "Authorization: Bearer $BOB_TOKEN" -H "Content-Type: application/json" -d '{"group_id":"'$GROUP_ID'","role":"member"}'` (bob does not own the group) → HTTP 403.

## Completion criteria
- [x] `groups` and `group_members` tables exist in `server/schema.sql` with a generated Atlas migration checked in.
- [x] `group` domain package, `GroupUsecase`, `GroupRepository` (postgres), and `GroupHandler` are implemented and wired into the DI container/routes.
- [x] All group CRUD/membership endpoints and the batch-invitation endpoint are routed and behave per the authorization/status rules in this document.
- [x] `GroupUsecase.BatchInviteToRoom` reuses `InvitationUsecase.CreateInvitation` unchanged, returns per-member skip results instead of failing the whole batch on a duplicate/already-member case, and short-circuits on RBAC failure.
- [x] Unit tests (usecase + handler, using `server/internal/testutil/mocks`) and testcontainers integration tests (repository) are added and pass.
- [x] All verification checks above pass (except item 6, which requires the full compose stack — see skippedComposeChecks).
