/**
 * Message-domain types, mirroring the Go API's message/model DTOs
 * (`server/internal/interface/handler/dto.go`), reached client-side via the
 * Step 4 data-plane proxy at `/api/proxy/rooms/:roomId/messages*` and
 * `/api/proxy/models`.
 */

export type MessageType = "human" | "ai"
export type MessageStatus = "completed" | "failed"

export interface Message {
  id: string
  room_id: string
  sender_id: string | null
  content: string
  type: MessageType
  status: MessageStatus
  sequence: number
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
