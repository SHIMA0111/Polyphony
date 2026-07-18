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
   * `features/members/lib/roles.ts`.
   */
  role: RoomRole
  created_at: string
  updated_at: string
}
