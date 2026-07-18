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
  created_at: string
  updated_at: string
}
