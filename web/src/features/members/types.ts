/**
 * The 5-tier per-room role, exactly as returned by the Go API (`Role`,
 * `server/internal/domain/room/role.go`) and carried on `RoomResponse.role`
 * / `MemberResponse.role` / `InvitationResponse.role`.
 *
 * Ordered lowest-to-highest privilege; see `lib/roles.ts` for the client-side
 * mirror of the server's `Role.Allows` matrix built on top of this order.
 */
export type RoomRole = "reader" | "guest" | "member" | "admin" | "master"

/**
 * A single room membership row, exactly as returned by the Go API's
 * `MemberResponse` (`server/internal/interface/handler/dto.go`).
 *
 * `username` is populated by the member-list endpoint (`GET
 * /rooms/:roomId/members`, which JOINs against `users`) but is `""` on
 * responses built from non-JOINed lookups such as the role-change endpoint
 * — see the username-display note in `docs/tasks/step37.md`'s Implementation
 * notes before rendering this field.
 */
export interface Member {
  id: string
  room_id: string
  user_id: string
  username: string
  role: RoomRole
  joined_at: string
}

/** Response body for `GET /rooms/:roomId/members`. */
export interface MemberListResponse {
  members: Member[]
}

/**
 * Lifecycle status of a `room_invitations` row, exactly as returned by the
 * Go API's `InvitationResponse.status`.
 */
export type InvitationStatus = "pending" | "accepted" | "rejected" | "revoked"

/**
 * A single room invitation, exactly as returned by the Go API's
 * `InvitationResponse` (`server/internal/interface/handler/dto.go`).
 *
 * `invitee_id` is `null` for a reusable link invitation (no specific
 * invitee) and set for a single-use, username-targeted invitation.
 */
export interface Invitation {
  id: string
  room_id: string
  inviter_id: string
  invitee_id: string | null
  invite_code: string
  role: RoomRole
  status: InvitationStatus
  expires_at: string
  created_at: string
}

/** Response body for a list of invitations (`GET /rooms/:roomId/invitations`, `GET /invitations`). */
export interface InvitationListResponse {
  invitations: Invitation[]
}

/**
 * Response body for `POST /invitations/:invitationId/accept`, exactly as
 * returned by the Go API's `RoomMembershipResponse`. Same shape as
 * {@link Member} but never carries `username` (the accept endpoint isn't
 * JOINed against `users`).
 */
export interface RoomMembershipResponse {
  id: string
  room_id: string
  user_id: string
  role: RoomRole
  joined_at: string
}
