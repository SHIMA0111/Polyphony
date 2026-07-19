import { apiRequest } from "@/lib/http-client"
import type { Member, RoomRole } from "../types"

/**
 * Calls `PATCH /api/proxy/rooms/:roomId/members/:userId/role` — the response
 * body (`Member`, mirroring the server's `MemberResponse`) carries `username`
 * as `""` (a non-JOINed lookup): callers must invalidate/refetch the
 * member-list query rather than patch the cache with this response, per the
 * username-display note in `docs/tasks/step37.md`.
 */
export function changeMemberRole(
  roomId: string,
  userId: string,
  role: RoomRole,
): Promise<Member> {
  return apiRequest<Member>(`/rooms/${roomId}/members/${userId}/role`, {
    method: "PATCH",
    body: JSON.stringify({ role }),
  })
}
