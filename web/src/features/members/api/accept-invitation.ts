import { apiRequest } from "@/lib/http-client"
import type { RoomMembershipResponse } from "../types"

/**
 * Calls `POST /api/proxy/invitations/:invitationId/accept`.
 *
 * Rejects with `ApiRequestError` (403 wrong invitee, 409 already a member or
 * no longer pending, 410 expired) — callers render a short inline status
 * message instead of letting the rejection surface as an unhandled
 * exception.
 */
export function acceptInvitation(
  invitationId: string,
): Promise<RoomMembershipResponse> {
  return apiRequest<RoomMembershipResponse>(
    `/invitations/${invitationId}/accept`,
    { method: "POST" },
  )
}
