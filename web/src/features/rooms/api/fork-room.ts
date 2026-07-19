import { apiRequest } from "@/lib/http-client"
import type { RoomForkResponse } from "../types"

/**
 * Triggers a room fork: calls `POST /api/proxy/rooms/:roomId/fork` (Step
 * 32's `RoomHandler.Fork`), which creates a new, archived room that is a
 * structural copy of `roomId`'s message history, plus a room-fork job
 * tracking the background copy. Returns immediately (HTTP 202) with the
 * job's initial `"pending"` state and the new room; poll
 * {@link import("./get-fork-job-status").getForkJobStatus} with
 * `job.id` until `status` reaches `"completed"` or `"failed"`.
 */
export function forkRoom(roomId: string): Promise<RoomForkResponse> {
  return apiRequest<RoomForkResponse>(`/rooms/${roomId}/fork`, {
    method: "POST",
  })
}
