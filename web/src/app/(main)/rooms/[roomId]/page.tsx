import { notFound } from "next/navigation"
import { HydrationBoundary, dehydrate } from "@tanstack/react-query"
import { getQueryClient } from "@/lib/query-client"
import { serverHttpClient } from "@/lib/http-client.server"
import { getRoomQueryOptions } from "@/features/rooms/api/get-room"
import { getMessagesInfiniteQueryOptions } from "@/features/messages/api/get-messages"
import { getModelsQueryOptions } from "@/features/messages/api/get-models"
import { ChatRoom } from "@/features/messages/components/ChatRoom"

/**
 * Server Component that prefetches the room, its messages, and the
 * available models in parallel before first paint, then hands the
 * dehydrated cache to `ChatRoom` via `HydrationBoundary` — avoiding the
 * client-side loading spinner flash the pre-migration `useEffect` fetch had
 * on first navigation into a room.
 *
 * The room itself is fetched with `fetchQuery` (not `prefetchQuery`, which
 * swallows query errors by design) inside a `try/catch` so a 404/failed
 * fetch can trigger `notFound()`, rendering this route's `not-found.tsx`
 * instead of a chat UI with no room to show.
 */
export default async function ChatRoomPage({
  params,
}: {
  params: Promise<{ roomId: string }>
}) {
  const { roomId } = await params
  const queryClient = getQueryClient()

  try {
    await queryClient.fetchQuery(getRoomQueryOptions(roomId, serverHttpClient.get))
  } catch {
    notFound()
  }

  await Promise.all([
    queryClient.prefetchInfiniteQuery(
      getMessagesInfiniteQueryOptions(roomId, serverHttpClient.get),
    ),
    queryClient.prefetchQuery(getModelsQueryOptions(serverHttpClient.get)),
  ])

  return (
    <HydrationBoundary state={dehydrate(queryClient)}>
      <ChatRoom roomId={roomId} />
    </HydrationBoundary>
  )
}
