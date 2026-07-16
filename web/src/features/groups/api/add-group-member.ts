import { apiRequest } from "@/lib/http-client"
import type { GroupMember } from "../types"

/**
 * Calls `POST /api/proxy/groups/:groupId/members` with an exact username.
 * The server 404s on an unknown username and 409s if the user is already a
 * member — both surface as {@link import("@/lib/http-client").ApiRequestError}.
 */
export function addGroupMember(
  groupId: string,
  username: string,
): Promise<GroupMember> {
  return apiRequest<GroupMember>(`/groups/${groupId}/members`, {
    method: "POST",
    body: JSON.stringify({ username }),
  })
}
