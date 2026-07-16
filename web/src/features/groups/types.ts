import type { Invitation } from "@/features/members/types"

/**
 * A single personal group, exactly as returned by the Go API's
 * `GroupResponse` (`server/internal/interface/handler/dto.go`).
 *
 * Groups are strictly personal and flat (Step 40's server-side scope): only
 * `owner_id` may manage a group's membership or delete it, and there is no
 * per-member role or nested-group concept.
 */
export interface Group {
  id: string
  owner_id: string
  name: string
  description: string
  created_at: string
  updated_at: string
}

/** Response body for `GET /groups`. */
export interface GroupListResponse {
  groups: Group[]
}

/**
 * A single group membership row, exactly as returned by the Go API's
 * `GroupMemberResponse` (`GroupMemberWithUsername` serialized).
 *
 * Unlike `@/features/members/types`' room `Member`, `username` here is
 * always resolved (Step 40's `ListMembers`/`AddMember` queries always JOIN
 * against `users`) — never fall back to the raw `user_id` when rendering
 * this field.
 */
export interface GroupMember {
  id: string
  group_id: string
  user_id: string
  username: string
  added_at: string
}

/** Response body for `GET /groups/:groupId/members`. */
export interface GroupMemberListResponse {
  members: GroupMember[]
}

/**
 * One group member who was not invited by a batch-invite-by-group call,
 * along with the reason (e.g. already a room member, already has a pending
 * invitation), exactly as returned by the Go API's `BatchInviteSkipResponse`.
 */
export interface BatchInviteSkip {
  user_id: string
  username: string
  reason: string
}

/**
 * Response body for `POST /rooms/:roomId/invitations/batch-by-group`,
 * exactly as returned by the Go API's `BatchInviteByGroupResponse`.
 *
 * `invited` reuses the same `Invitation` shape `@/features/members/types`
 * already defines (Step 40's `BatchInviteByGroupResponse.invited` is a plain
 * `[]InvitationResponse`) rather than redefining it here.
 */
export interface BatchInviteByGroupResult {
  invited: Invitation[]
  skipped: BatchInviteSkip[]
}
