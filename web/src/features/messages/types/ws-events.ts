import type { Message } from "../types"

/**
 * Mirrors `server/internal/domain/event/hub.go`'s `EventType` constants —
 * the only two values the server's `WebSocketHandler` (Step 15) currently
 * emits as a frame's `"type"` field. Kept as a standalone union (rather than
 * inlined into `RoomSocketEvent`) so a later step adding a new event type
 * (e.g. streaming token chunks, Step 54) can extend this without touching
 * the discriminated union's shape.
 */
export type RoomSocketEventType = "message_created" | "message_updated"

/**
 * A single inbound WebSocket frame, mirroring the Go API's `wsEventFrame`
 * (`server/internal/interface/handler/websocket_handler.go`):
 * `{"type": "message_created" | "message_updated", "room_id": "...", "message": {...}}`.
 *
 * `message` is typed as the same `Message` shape the REST
 * `GET /rooms/:roomId/messages` endpoint returns (both ultimately serialize
 * the Go API's `MessageResponse` DTO), so `merge-message-event.ts` can splice
 * it directly into the `["rooms", roomId, "messages"]` query cache without a
 * separate mapping step.
 */
export interface RoomSocketEvent {
  type: RoomSocketEventType
  room_id: string
  message: Message
}

/**
 * Runtime type guard validating that an inbound, already-JSON-parsed WS
 * payload actually has the shape of a {@link RoomSocketEvent} before
 * `use-room-socket.ts` trusts it enough to feed into the query cache.
 *
 * Deliberately conservative: only checks the fields this client's merge
 * logic reads (`type`, `message.id`), not every field of `Message` — a
 * message payload that is missing a field this client doesn't use yet
 * should not be dropped outright.
 */
export function isRoomSocketEvent(value: unknown): value is RoomSocketEvent {
  if (typeof value !== "object" || value === null) return false

  const frame = value as Record<string, unknown>
  if (frame.type !== "message_created" && frame.type !== "message_updated") {
    return false
  }
  if (typeof frame.room_id !== "string") return false

  const message = frame.message
  if (typeof message !== "object" || message === null) return false
  if (typeof (message as Record<string, unknown>).id !== "string") return false

  return true
}
