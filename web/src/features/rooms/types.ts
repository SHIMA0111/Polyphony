import type { RoomRole } from "@/features/members/types"

/**
 * A chat room, exactly as returned by the Go API's `RoomResponse`
 * (`server/internal/interface/handler/dto.go`), reached client-side via the
 * Step 4 data-plane proxy at `/api/proxy/rooms*`.
 */
export interface Room {
  id: string
  name: string
  description: string
  owner_id: string
  /**
   * The requesting user's own 5-tier role in this room (Step 13's
   * `RoomResponse.role`) — the single source of truth for every
   * member-management/AI/send gate in `features/members`; see
   * `features/members/lib/roles.ts`. Also read directly as a plain string
   * by `MessageBubble`'s per-message action-menu role gate (Step 38).
   */
  role: RoomRole
  /**
   * The room's per-room AI context cutoff (Step 23's
   * `RoomResponse.ai_context_cutoff_at`), an RFC3339 timestamp, or `null`
   * when unset. Messages created before this point are excluded from AI
   * context assembly. Set/cleared via `PATCH
   * /rooms/:roomId/ai-context-cutoff` (see `../api/update-ai-context-cutoff.ts`).
   */
  ai_context_cutoff_at: string | null
  /**
   * The room's default AI provider/model (Step 24's
   * `RoomResponse.ai_provider`/`ai_model`), or `null` when the room has no
   * per-room default and falls through to the deployment-wide default.
   * Set/cleared via `PATCH /rooms/:roomId/settings` (see
   * `../api/update-room-settings.ts`).
   */
  ai_provider: string | null
  ai_model: string | null
  /**
   * Names the source room this room was created from (Step 32's
   * `RoomResponse.forked_from_room_id`), or `null` if this room was not
   * created via `POST /rooms/:roomId/fork`.
   */
  forked_from_room_id: string | null
  /**
   * `true` from the moment a fork of this room is created until its
   * background copy job (see {@link ForkJob}) reaches `"completed"` (Step
   * 32's `RoomResponse.is_archived`). While `true`, the server rejects
   * `POST .../messages` and `.../messages/ai` with HTTP 409, so
   * `ChatRoom` renders the room read-only and does not render
   * `MessageInput` (see `features/messages/components/ChatRoom.tsx`).
   */
  is_archived: boolean
  created_at: string
  updated_at: string
}

/**
 * Progress/status of a room-fork background copy job, exactly as returned
 * by Step 32's `ForkJobResponse`
 * (`server/internal/interface/handler/dto.go`). `total_messages` is `0`
 * while `status` is `"pending"` (the worker only sets the real count once
 * the job transitions to `"running"`). `error_message` is non-`null` only
 * when `status` is `"failed"`.
 */
export interface ForkJob {
  id: string
  source_room_id: string
  new_room_id: string
  status: "pending" | "running" | "completed" | "failed"
  total_messages: number
  copied_messages: number
  error_message: string | null
  created_at: string
  updated_at: string
}

/**
 * Response body for `POST /rooms/:roomId/fork` (Step 32's
 * `RoomForkResponse`): the newly created (archived) room paired with the
 * room-fork job tracking the background copy into it.
 */
export interface RoomForkResponse {
  job: ForkJob
  new_room: Room
}
