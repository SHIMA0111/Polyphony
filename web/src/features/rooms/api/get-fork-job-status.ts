import { apiRequest } from "@/lib/http-client"
import type { ForkJob } from "../types"

/**
 * Calls `GET /api/proxy/rooms/:roomId/fork-jobs/:jobId` (Step 32's
 * `RoomHandler.GetForkJobStatus`) for the current progress/status of a
 * room-fork job. Any member of either the source or destination room may
 * poll this. See `../hooks/use-fork-job.ts` for the polling `useQuery`
 * wrapper.
 */
export function getForkJobStatus(
  roomId: string,
  jobId: string,
): Promise<ForkJob> {
  return apiRequest<ForkJob>(`/rooms/${roomId}/fork-jobs/${jobId}`)
}
