"use client"

import { useQuery } from "@tanstack/react-query"
import { getRoomsQueryOptions } from "../api/get-rooms"

/**
 * List of rooms the current user belongs to, sourced from
 * `GET /api/proxy/rooms`.
 *
 * TanStack Query replacement for the Step 5 placeholder hook of the same
 * name (see that file's removed docstring for context): same `["rooms"]`
 * data, now cached/deduped/prefetchable via the shared `QueryClient`.
 */
export function useRooms() {
  return useQuery(getRoomsQueryOptions())
}
