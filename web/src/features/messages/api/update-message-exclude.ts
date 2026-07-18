import { apiRequest } from "@/lib/http-client"
import type { Message } from "../types"

/**
 * Calls `PATCH /api/proxy/rooms/:roomId/messages/:messageId` (Step 23's
 * `SetExcludeFromAI` endpoint) to toggle whether `messageId` is excluded
 * from future AI context assembly. Resolves with the full updated
 * `MessageResponse` on success, which callers patch into the query cache in
 * place of a full refetch.
 */
export function updateMessageExclude(
  roomId: string,
  messageId: string,
  exclude: boolean,
): Promise<Message> {
  return apiRequest<Message>(`/rooms/${roomId}/messages/${messageId}`, {
    method: "PATCH",
    body: JSON.stringify({ exclude_from_ai: exclude }),
  })
}
