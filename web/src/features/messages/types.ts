/**
 * Message-domain types, mirroring the Go API's message/model DTOs
 * (`server/internal/interface/handler/dto.go`), reached client-side via the
 * Step 4 data-plane proxy at `/api/proxy/rooms/:roomId/messages*` and
 * `/api/proxy/models`.
 */

export type MessageType = "human" | "ai"

/**
 * `"completed"` and `"failed"` are persisted server statuses (see
 * `server/internal/domain/message`'s `MessageStatus`); `"failed"` covers both
 * a genuinely failed AI response (an LLM-call failure the server itself
 * recorded, retryable via `RegenerateAIMessage`) and, client-side, an
 * optimistic send that never made it to the server at all.
 *
 * `"sending"` is a **client-only** status: it is synthesized locally by
 * `useSendMessage`/`useSendAIMessage`'s optimistic `onMutate` for a message
 * that has been echoed into the transcript but not yet acknowledged by the
 * API. The server never emits `"sending"` in any response body — do not
 * treat it as a persisted state.
 */
export type MessageStatus = "completed" | "failed" | "sending"

export interface Message {
  id: string
  room_id: string
  sender_id: string | null
  content: string
  type: MessageType
  status: MessageStatus
  sequence: number
  /**
   * The human message this AI message was generated in response to; `null`
   * for human messages, and set for every AI message created via
   * `SendAIMessage` (see `server/internal/interface/handler/dto.go`'s
   * `MessageResponse`). `handleRegenerate` resolves its target human message
   * id from this field rather than scanning the message list by position.
   */
  in_response_to_message_id: string | null
  created_at: string
  updated_at: string
}

/** Raw paginated response from `GET /rooms/:roomId/messages`. */
export interface MessagePage {
  messages: Message[]
  next_cursor: string | null
}

/** Response body for `POST /rooms/:roomId/messages/ai`. */
export interface AIMessageResponse {
  user_message: Message
  ai_message: Message
}

/**
 * An available LLM model. Deliberately structurally identical to
 * `ModelSelector`'s local `Model` interface so query data can be passed
 * straight through without a mapping step; token limits/pricing are added in
 * Step 34.
 */
export interface ModelInfo {
  id: string
  name: string
  provider: string
}

/** Raw response from `GET /models`. */
export interface ModelListResponse {
  models: ModelInfo[]
}
