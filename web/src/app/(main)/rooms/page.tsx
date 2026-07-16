import { HydrationBoundary, dehydrate } from "@tanstack/react-query"
import { getQueryClient } from "@/lib/query-client"
import { serverHttpClient } from "@/lib/http-client.server"
import { getRoomsQueryOptions } from "@/features/rooms/api/get-rooms"
import { RoomList } from "@/features/rooms/components/RoomList"

/**
 * Server Component that prefetches the `["rooms"]` query before first paint
 * (so the initial HTML already contains the room list) and hands the
 * dehydrated cache to `RoomList` via `HydrationBoundary`.
 */
export default async function RoomsPage() {
  const queryClient = getQueryClient()
  await queryClient.prefetchQuery(getRoomsQueryOptions(serverHttpClient.get))

  return (
    <HydrationBoundary state={dehydrate(queryClient)}>
      <RoomList />
    </HydrationBoundary>
  )
}
