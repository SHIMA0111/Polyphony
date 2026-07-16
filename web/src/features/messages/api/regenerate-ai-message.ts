import { apiRequest } from "@/lib/http-client"
import type { Message } from "../types"

/** Calls `POST /api/proxy/rooms/:roomId/messages/:messageId/regenerate`. */
export function regenerateAIMessage(
  roomId: string,
  messageId: string,
  model?: string,
): Promise<Message> {
  return apiRequest<Message>(
    `/rooms/${roomId}/messages/${messageId}/regenerate`,
    {
      method: "POST",
      body: JSON.stringify({ model }),
    },
  )
}
