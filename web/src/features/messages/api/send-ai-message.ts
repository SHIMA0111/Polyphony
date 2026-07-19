import { apiRequest } from "@/lib/http-client"
import type { AIMessageResponse } from "../types"

/**
 * Calls `POST /api/proxy/rooms/:roomId/messages/ai`: the non-streaming
 * send-with-AI path. Waits for the full AI completion before resolving —
 * `ai_message.status` is `"completed"` or `"failed"`, never `"streaming"`.
 *
 * Kept alongside {@link sendAIMessageStream} (rather than replaced by it) as
 * the documented fallback: `StreamAI` (the streaming endpoint) rejects
 * `private: true` with HTTP 400 (private AI mode is not yet supported for
 * streaming — see `server/internal/interface/handler/message_handler.go`),
 * so any private-mode send must keep using this function.
 *
 * `isPrivate` maps directly onto the request body's `private` field (Step
 * 41's `SendAIMessageRequest.Private`, `server/internal/interface/handler/
 * dto.go`); omitted/`false` sends `private: false`, matching the field's
 * own server-side default.
 */
export function sendAIMessage(
  roomId: string,
  content: string,
  model?: string,
  isPrivate?: boolean,
): Promise<AIMessageResponse> {
  return apiRequest<AIMessageResponse>(`/rooms/${roomId}/messages/ai`, {
    method: "POST",
    body: JSON.stringify({ content, model, private: isPrivate ?? false }),
  })
}

/**
 * Calls `POST /api/proxy/rooms/:roomId/messages/ai/stream` (Step 51's
 * streaming endpoint). Resolves as soon as the server has synchronously
 * persisted the human message and an AI placeholder — `ai_message.status`
 * is `"streaming"` on the happy path (or `"failed"` if the LLM Gateway
 * rejected the request synchronously, e.g. an unknown model), *never*
 * `"completed"`: the actual response text arrives afterward as a sequence
 * of `token_chunk` WebSocket frames followed by a final `message_updated`
 * frame (see `../types/ws-events.ts` and `../lib/merge-message-event.ts`),
 * not in this response body.
 *
 * Never pass `private: true` through this path — see {@link sendAIMessage}'s
 * docstring for why private sends must use that function instead.
 */
export function sendAIMessageStream(
  roomId: string,
  content: string,
  model?: string,
): Promise<AIMessageResponse> {
  return apiRequest<AIMessageResponse>(`/rooms/${roomId}/messages/ai/stream`, {
    method: "POST",
    body: JSON.stringify({ content, model }),
  })
}
