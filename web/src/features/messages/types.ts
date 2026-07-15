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
  /**
   * `true` for a soft-deleted message (see `DELETE
   * /rooms/:roomId/messages/:messageId`). The server already omits
   * soft-deleted rows from `GET /rooms/:roomId/messages`, so this is only
   * ever `true` transiently on a not-yet-reconciled local cache entry — see
   * `../lib/message-cache.ts`'s any-page helpers.
   */
  is_deleted: boolean
  /**
   * `true` once a message has been opted out of future AI context assembly
   * via `PATCH /rooms/:roomId/messages/:messageId` (Step 23's
   * `SetExcludeFromAI`). Toggled from `MessageBubble`'s per-message menu;
   * `MessageInput`'s token meter filters these out of its estimate payload.
   */
  exclude_from_ai: boolean
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
 * An available LLM model, matching the flat JSON shape the Go API's
 * `GET /models` serializes (`server/internal/interface/handler/dto.go`'s
 * `ModelResponse` -- a pure passthrough of `ai.ModelInfo`).
 *
 * `context_window`/`input_price_per_million_tokens`/
 * `output_price_per_million_tokens` are `0` when unknown (not "no limit"/
 * "free"); `supports_image_input` is `false` for both "no" and "unknown".
 * `ModelSelector` imports this type directly rather than maintaining a
 * parallel `Model` type, to avoid drift.
 */
export interface ModelInfo {
  id: string
  name: string
  provider: string
  /** Maximum input+output token count the model supports; `0` if unknown. */
  context_window: number
  /** USD price per 1,000,000 input (prompt) tokens; `0` if unknown. */
  input_price_per_million_tokens: number
  /** USD price per 1,000,000 output (completion) tokens; `0` if unknown. */
  output_price_per_million_tokens: number
  /** Whether the model accepts image/Vision content parts. */
  supports_image_input: boolean
}

/** Raw response from `GET /models`. */
export interface ModelListResponse {
  models: ModelInfo[]
}

/**
 * A single chat turn as sent in `POST /tokens/estimate`'s `messages` array
 * (`server/internal/interface/handler/dto.go`'s `ChatMessageDTO`). Mirrors
 * the human/AI distinction `Message.type` already models, but re-expressed
 * as the `"user"`/`"assistant"` role vocabulary the LLM Gateway's token
 * estimator expects.
 */
export interface EstimateChatMessage {
  role: "user" | "assistant"
  content: string
}

/** Response body for `POST /tokens/estimate`. */
export interface TokenEstimateResponse {
  model: string
  estimated_tokens: number
}
