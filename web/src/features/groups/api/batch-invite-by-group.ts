import { apiRequest } from "@/lib/http-client"
import type { RoomRole } from "@/features/members/types"
import type { BatchInviteByGroupResult } from "../types"

/**
 * The subset of `RoomRole` a batch-invite-by-group call may target — every
 * value except `"master"`. Previously this field was typed as the full
 * `RoomRole`, which let `"master"` pass the type checker even though the
 * server always 400s it; narrowing the type catches that mistake at compile
 * time instead of at the request.
 */
export type InvitationRole = Exclude<RoomRole, "master">

/**
 * Request body for `POST /rooms/:roomId/invitations/batch-by-group`.
 * `role` must be one of the four non-`master` `RoomRole` values; the server
 * 400s otherwise. `expires_in_hours` is optional and follows the same
 * `[1, 720]`-hour bounds as a single-username invitation.
 */
export interface BatchInviteByGroupInput {
  group_id: string
  role: InvitationRole
  expires_in_hours?: number
}

/**
 * Calls `POST /api/proxy/rooms/:roomId/invitations/batch-by-group` —
 * batch-invites every member of `input.group_id` into `roomId` at
 * `input.role`. The server 403s if the caller doesn't own the group or
 * lacks Admin+ in the room, and 400s on an invalid/`master` role; per-member
 * failures (already a room member, already has a pending invitation) do not
 * reject the call — they come back as `skipped` entries on the resolved
 * {@link BatchInviteByGroupResult}.
 */
export function batchInviteByGroup(
  roomId: string,
  input: BatchInviteByGroupInput,
): Promise<BatchInviteByGroupResult> {
  return apiRequest<BatchInviteByGroupResult>(
    `/rooms/${roomId}/invitations/batch-by-group`,
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  )
}
