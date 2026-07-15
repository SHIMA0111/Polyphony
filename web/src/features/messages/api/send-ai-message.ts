import { apiRequest } from "@/lib/http-client"
import type { AIMessageResponse } from "../types"

/** Calls `POST /api/proxy/rooms/:roomId/messages/ai`. */
export function sendAIMessage(
  roomId: string,
  content: string,
  model?: string,
): Promise<AIMessageResponse> {
  return apiRequest<AIMessageResponse>(`/rooms/${roomId}/messages/ai`, {
    method: "POST",
    body: JSON.stringify({ content, model }),
  })
}
