import { apiRequest } from "@/lib/http-client"

/** Calls `DELETE /api/proxy/groups/:groupId/members/:userId`. */
export function removeGroupMember(
  groupId: string,
  userId: string,
): Promise<void> {
  return apiRequest<void>(`/groups/${groupId}/members/${userId}`, {
    method: "DELETE",
  })
}
