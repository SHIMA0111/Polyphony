import { apiRequest } from "@/lib/http-client"
import type { Invitation, RoomRole } from "../types"

/**
 * Request body for `POST /rooms/:roomId/invitations`. Omit
 * `invitee_username` to create a reusable link invitation; set it to create
 * a single-use invitation targeting an exact, existing username.
 */
export interface CreateInvitationInput {
  invitee_username?: string
  role: RoomRole
  expires_in_hours?: number
}

/** Calls `POST /api/proxy/rooms/:roomId/invitations`. */
export function createInvitation(
  roomId: string,
  input: CreateInvitationInput,
): Promise<Invitation> {
  return apiRequest<Invitation>(`/rooms/${roomId}/invitations`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}
