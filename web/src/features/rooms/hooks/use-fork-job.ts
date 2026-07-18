"use client"

import { useQuery } from "@tanstack/react-query"
import { getForkJobStatus } from "../api/get-fork-job-status"
import type { ForkJob } from "../types"

/** Terminal statuses at which polling stops (see {@link useForkJob}). */
const TERMINAL_STATUSES: ReadonlySet<ForkJob["status"]> = new Set([
  "completed",
  "failed",
])

/** Fixed polling interval while a fork job is still `"pending"`/`"running"`. */
const POLL_INTERVAL_MS = 1500

/**
 * Polls `GET /rooms/:roomId/fork-jobs/:jobId` (Step 32) on a fixed
 * `POLL_INTERVAL_MS` cadence, driving `RoomSettingsDrawer`'s inline fork
 * progress view. Polling stops automatically once `status` reaches
 * `"completed"` or `"failed"`, and is cleaned up by TanStack Query itself
 * when the drawer unmounts/closes.
 *
 * `jobId` is `undefined` before a fork has been triggered in this drawer
 * session; the query is disabled until one exists.
 */
export function useForkJob(roomId: string, jobId: string | undefined) {
  return useQuery({
    queryKey: ["rooms", roomId, "fork-jobs", jobId] as const,
    queryFn: () => getForkJobStatus(roomId, jobId as string),
    enabled: jobId !== undefined,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status !== undefined && TERMINAL_STATUSES.has(status)
        ? false
        : POLL_INTERVAL_MS
    },
  })
}
