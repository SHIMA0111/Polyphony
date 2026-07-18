import { apiRequest } from "@/lib/http-client"
import type { Room } from "@/features/rooms/types"

/**
 * Calls `PATCH /api/proxy/rooms/:roomId/owner`. The response is the updated
 * `Room` — the caller's own `role` on it flips from `master` to `admin`
 * (the previous owner) once this succeeds, so `useTransferOwnership`
 * invalidates the `["rooms", roomId]` / `["rooms"]` queries rather than
 * patching the cache with this response directly.
 */
export function transferOwnership(
  roomId: string,
  newOwnerId: string,
): Promise<Room> {
  return apiRequest<Room>(`/rooms/${roomId}/owner`, {
    method: "PATCH",
    body: JSON.stringify({ new_owner_id: newOwnerId }),
  })
}
