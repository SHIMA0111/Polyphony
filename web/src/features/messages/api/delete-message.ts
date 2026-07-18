import { apiRequest } from "@/lib/http-client"

/**
 * Calls `DELETE /api/proxy/rooms/:roomId/messages/:messageId` (Step 23's
 * soft-delete endpoint). Resolves with no value on success — the response is
 * `204 No Content`, which `apiRequest`/`apiFetch` already resolve as
 * `undefined` rather than attempting to parse a body.
 */
export function deleteMessage(roomId: string, messageId: string): Promise<void> {
  return apiRequest<void>(`/rooms/${roomId}/messages/${messageId}`, {
    method: "DELETE",
  })
}
