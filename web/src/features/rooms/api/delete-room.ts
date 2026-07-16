import { apiRequest } from "@/lib/http-client"

/** Calls `DELETE /api/proxy/rooms/:roomId`. */
export function deleteRoom(roomId: string): Promise<void> {
  return apiRequest<void>(`/rooms/${roomId}`, { method: "DELETE" })
}
