import type { Message } from "../types"

/**
 * Mirrors `server/internal/domain/event/hub.go`'s `EventType` constants —
 * the three values the server's `WebSocketHandler` (Steps 15/51) emits as a
 * frame's `"type"` field. `"token_chunk"` is Step 51's AI streaming chunk
 * forwarding (see `ChunkPayload`/`ChunkSocketEvent` below); it was the
 * anticipated extension point this union was originally kept standalone
 * for.
 */
export type RoomSocketEventType = "message_created" | "message_updated" | "token_chunk"

/**
 * A `"message_created"`/`"message_updated"` inbound WebSocket frame,
 * mirroring the Go API's `wsEventFrame`
 * (`server/internal/interface/handler/websocket_handler.go`):
 * `{"type": "message_created" | "message_updated", "room_id": "...", "message": {...}}`.
 *
 * `message` is typed as the same `Message` shape the REST
 * `GET /rooms/:roomId/messages` endpoint returns (both ultimately serialize
 * the Go API's `MessageResponse` DTO), so `merge-message-event.ts` can splice
 * it directly into the `["rooms", roomId, "messages"]` query cache without a
 * separate mapping step.
 */
export interface MessageSocketEvent {
  type: "message_created" | "message_updated"
  room_id: string
  message: Message
}

/**
 * The JSON payload of a `"token_chunk"` `wsEventFrame`'s `chunk` field,
 * mirroring the Go API's `ChunkResponse`
 * (`server/internal/interface/handler/websocket_handler.go`), which in turn
 * mirrors `event.StreamChunkEvent`. `summary_used` is the per-message
 * "this response's context included a summarized history" signal — the same
 * one-time signal `Message.used_context_summary` carries on a finalized
 * message, surfaced early here so a live-streaming bubble can show it before
 * the terminating `message_updated` frame arrives.
 */
export interface ChunkPayload {
  message_id: string
  delta: string
  summary_used: boolean
}

/**
 * A `"token_chunk"` inbound WebSocket frame (Step 51's AI streaming chunk
 * forwarding), mirroring the Go API's `wsEventFrame`:
 * `{"type": "token_chunk", "room_id": "...", "chunk": {"message_id": "...", "delta": "...", "summary_used": false}}`.
 *
 * Mutually exclusive with {@link MessageSocketEvent}'s `message` field, per
 * `wsEventFrame`'s own doc comment: a single frame carries either `message`
 * or `chunk`, never both.
 */
export interface ChunkSocketEvent {
  type: "token_chunk"
  room_id: string
  chunk: ChunkPayload
}

/**
 * A single inbound WebSocket frame: either a full-message event
 * ({@link MessageSocketEvent}) or a streaming token chunk
 * ({@link ChunkSocketEvent}), discriminated on `type`.
 */
export type RoomSocketEvent = MessageSocketEvent | ChunkSocketEvent

/**
 * Runtime type guard validating that an inbound, already-JSON-parsed WS
 * payload actually has the shape of a {@link RoomSocketEvent} before
 * `use-room-socket.ts` trusts it enough to feed into the query cache.
 *
 * Deliberately conservative: only checks the fields this client's merge
 * logic reads (`type`, `message.id` for a message event; `type`,
 * `chunk.message_id`, `chunk.delta` for a chunk event) — a payload missing a
 * field this client doesn't use yet should not be dropped outright.
 */
export function isRoomSocketEvent(value: unknown): value is RoomSocketEvent {
  if (typeof value !== "object" || value === null) return false

  const frame = value as Record<string, unknown>
  if (typeof frame.room_id !== "string") return false

  if (frame.type === "message_created" || frame.type === "message_updated") {
    const message = frame.message
    if (typeof message !== "object" || message === null) return false
    return typeof (message as Record<string, unknown>).id === "string"
  }

  if (frame.type === "token_chunk") {
    const chunk = frame.chunk
    if (typeof chunk !== "object" || chunk === null) return false
    const c = chunk as Record<string, unknown>
    return typeof c.message_id === "string" && typeof c.delta === "string"
  }

  return false
}
