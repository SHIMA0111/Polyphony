import { apiRequest } from "@/lib/http-client"
import type { Message } from "../types"

/** Calls `POST /api/proxy/rooms/:roomId/messages`. */
export function sendMessage(roomId: string, content: string): Promise<Message> {
  return apiRequest<Message>(`/rooms/${roomId}/messages`, {
    method: "POST",
    body: JSON.stringify({ content }),
  })
}
