import { apiRequest } from "@/lib/http-client"

/**
 * Calls `POST /api/proxy/invitations/:invitationId/reject`.
 *
 * Rejects with `ApiRequestError` (400 if the invitation is a reusable link
 * invitation — those have no meaningful "reject", see `InviteAcceptPage`).
 */
export function rejectInvitation(invitationId: string): Promise<void> {
  return apiRequest<void>(`/invitations/${invitationId}/reject`, {
    method: "POST",
  })
}
