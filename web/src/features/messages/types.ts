/**
 * Message-domain types, mirroring the Go API's message/model DTOs
 * (`server/internal/interface/handler/dto.go`), reached client-side via the
 * Step 4 data-plane proxy at `/api/proxy/rooms/:roomId/messages*` and
 * `/api/proxy/models`.
 */

export type MessageType = "human" | "ai"

/**
 * `"completed"`, `"failed"`, and `"streaming"` are persisted server statuses
 * (see `server/internal/domain/message`'s `MessageStatus`); `"failed"`
 * covers both a genuinely failed AI response (an LLM-call failure the
 * server itself recorded, retryable via `RegenerateAIMessage`) and,
 * client-side, an optimistic send that never made it to the server at all.
 *
 * `"streaming"` (Step 51/54) is a **wire value**, not a client-only
 * invention: `POST /rooms/:roomId/messages/ai/stream`'s `202` response
 * returns `ai_message.status === "streaming"` for the AI placeholder it
 * persists synchronously, before any token has actually been generated.
 * `token_chunk` WS frames (see `../types/ws-events.ts`) append to a message
 * in this state; the terminating `message_updated` frame always resolves it
 * to `"completed"` or `"failed"`, exactly as the non-streaming path does.
 *
 * `"sending"` is a **client-only** status: it is synthesized locally by
 * `useSendMessage`/`useSendAIMessage`'s optimistic `onMutate` for a message
 * that has been echoed into the transcript but not yet acknowledged by the
 * API. The server never emits `"sending"` in any response body — do not
 * treat it as a persisted state.
 */
export type MessageStatus = "completed" | "failed" | "sending" | "streaming"

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
  /**
   * `true` when this AI message's context included a summary of older room
   * history in place of the raw messages it replaces (Step 50's context
   * summarization; see `server/internal/interface/handler/dto.go`'s
   * `MessageResponse.UsedContextSummary`). This is a one-time,
   * request-scoped signal describing how the message was *generated*, not a
   * persisted property: it is only ever `true` on the fresh response body
   * from `POST /rooms/:roomId/messages/ai` or the regenerate endpoint --
   * historical messages returned by `GET /rooms/:roomId/messages` (and a
   * later refetch of the same message) always report `false`.
   */
  used_context_summary: boolean
  /**
   * `"public"` (the default) or `"private"` (Step 41's private AI mode,
   * `phases.md` Phase 14; see `server/internal/interface/handler/dto.go`'s
   * `MessageResponse.Visibility`). A `"private"` message (set via
   * `SendAIMessageRequest.Private`) is only ever delivered -- over both REST
   * (`GET /rooms/:roomId/messages`) and WebSocket (`message_created`/
   * `message_updated`) -- to its own sender's client; the server never sends
   * a private row/event to any other room member in the first place. Because
   * of that server-side guarantee, this client never needs to compare
   * `sender_id` to decide whether to show `MessageBubble`'s private badge --
   * any `visibility === "private"` message present in this client's cache
   * already belongs to the current user's own private exchange (see
   * `../lib/merge-message-event.ts`'s defensive sender-mismatch guard for the
   * one place that check *does* happen, as a belt-and-suspenders measure).
   */
  visibility: "public" | "private"
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

/**
 * Response body for `POST /rooms/:roomId/attachments/upload-url` (Step 12's
 * `attachment_handler.go`'s `PresignUploadResponse`). `upload_url` is a
 * presigned S3 `PUT` URL valid until `expires_at`; the browser uploads the
 * raw file bytes directly to it (see `../lib/upload-attachment.ts`), never
 * through the Go API itself.
 */
export interface UploadTicket {
  attachment_id: string
  s3_key: string
  upload_url: string
  expires_at: string
}

/**
 * The JSON representation of a single attachment without a view URL,
 * returned by `POST /rooms/:roomId/messages/:messageId/attachments` (Step
 * 12's `AttachmentResponse` DTO) once an already-uploaded object has been
 * linked to a message. `message_id` mirrors the Go DTO's nullable
 * `*string` verbatim, even though it is always non-null in the responses
 * this step's client ever reads (both the attach and list endpoints always
 * key on a real message id).
 *
 * Unlike {@link AttachmentWithUrl}, there is no `view_url` here -- the attach
 * endpoint doesn't mint a presigned read URL, only `GET
 * /rooms/:roomId/messages/:messageId/attachments` does.
 */
export interface AttachmentResponse {
  id: string
  message_id: string | null
  s3_key: string
  mime_type: string
  size_bytes: number
  created_at: string
}

/**
 * An attachment as returned by `GET
 * /rooms/:roomId/messages/:messageId/attachments` (Step 12's
 * `AttachmentViewResponse` DTO): every field of {@link AttachmentResponse}
 * plus a freshly-presigned `view_url`, ready to hand straight to an `<img>`.
 */
export interface AttachmentWithUrl extends AttachmentResponse {
  view_url: string
}
