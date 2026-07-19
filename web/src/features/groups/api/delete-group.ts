import { apiRequest } from "@/lib/http-client"

/** Calls `DELETE /api/proxy/groups/:groupId`. */
export function deleteGroup(groupId: string): Promise<void> {
  return apiRequest<void>(`/groups/${groupId}`, { method: "DELETE" })
}
