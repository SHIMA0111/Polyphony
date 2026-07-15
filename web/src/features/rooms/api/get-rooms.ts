import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { Room } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["rooms"]` query. Parameterized by
 * fetcher so the same factory both prefetches on the server
 * (`serverHttpClient.get`, in `app/(main)/rooms/page.tsx`) and fetches on the
 * client (default `apiRequest`, via `useRooms`).
 */
export function getRoomsQueryOptions(fetcher: Fetcher = apiRequest) {
  return queryOptions({
    queryKey: ["rooms"] as const,
    queryFn: () => fetcher<Room[]>("/rooms"),
  })
}
