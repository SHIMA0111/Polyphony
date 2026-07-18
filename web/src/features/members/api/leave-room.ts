import { apiRequest } from "@/lib/http-client"

/**
 * Calls `DELETE /api/proxy/rooms/:roomId/members/:userId` — self-leave only
 * (the server rejects `409` if `userId` is the caller and they currently own
 * the room; ownership must be transferred first).
 */
export function leaveRoom(roomId: string, userId: string): Promise<void> {
  return apiRequest<void>(`/rooms/${roomId}/members/${userId}`, {
    method: "DELETE",
  })
}
