# Phase 13: Group Management + Batch Invitations

**Goal**: Create user groups and invite entire groups to rooms.

**Delivered by**: `docs/tasks/step40.md` (Server: groups + batch invitations), `docs/tasks/step46.md` (Web: group
management UI). See those files for the full per-file implementation notes.

---

## Step 40: Server groups + batch invitations

- [x] `server/schema.sql`: `groups` / `group_members` tables; migration via `task migrate:generate --
      add_groups_and_group_members`
- [x] `server/internal/domain/group/entity.go` — `Group`, `GroupMember`, `GroupMemberWithUsername` (joined username)
- [x] `server/internal/domain/group/repository.go` — `GroupRepository` port (CRUD + member management,
      `ListMembers` single-query joined against `users`)
- [x] `postgres.GroupRepository` implementation following the `room_repository.go` pattern
- [x] `GroupUsecase` — create/get/list/update/delete (owner-only), add/list/remove member (by username),
      `BatchInviteToRoom` (reuses Step 21's `InvitationUsecase`, mixed success/skip results, RBAC-gated on the
      caller being Admin+ in the target room, group-ownership-gated on the caller owning the group)
- [x] `GroupHandler` + DTOs (`CreateGroupRequest`, `GroupResponse`, `AddGroupMemberRequest`,
      `BatchInviteByGroupRequest/Response`, reusing Step 21's `InvitationResponse`)
- [x] `POST /rooms/:roomId/invitations/batch-by-group` endpoint; DI wiring + `routes_group.go` registrar
- [x] Unit tests (owner/non-owner RBAC, duplicate member, batch invite success/mixed-skip/RBAC-short-circuit) +
      testcontainers integration tests for `GroupRepository`

## Step 46: Web group management UI

- [x] `web/src/features/groups/types.ts`, `api/` fetch modules, `hooks/` (TanStack Query, `["groups"]` /
      `["groups", groupId]` / `["groups", groupId, "members"]` keys, no collision with existing feature keys)
- [x] `GroupFormDialog` (create/edit), `GroupList` (`/groups` page), `GroupMemberRow`, `AddGroupMemberForm`,
      `GroupMembersPanel`, `GroupDetail` (`/groups/[groupId]` page), `GroupPicker` (menu, used by `InviteDialog`)
- [x] `app/(main)/groups/page.tsx`, `app/(main)/groups/[groupId]/page.tsx`; "Groups" nav entry added to the avatar
      menu in `app/(main)/layout.tsx`
- [x] `InviteDialog.tsx` extended with a third "Invite a group" section: `GroupPicker` + role select + optional
      expiry, calling `useBatchInviteByGroup`, rendering an invited/skipped summary inline

## Verification run in this worktree (Step 60)

- [x] `cd server && go build ./... && go vet ./... && go test ./... && go test -tags=integration ./...`
- [x] `cd web && bun install && bun run lint && bunx tsc --noEmit && bunx vitest run`
- [x] `web/e2e/groups.spec.ts` present and statically audited for fixture isolation (unique per-run room/user
      names) — not executed against the live compose stack in this run; see skippedComposeChecks
