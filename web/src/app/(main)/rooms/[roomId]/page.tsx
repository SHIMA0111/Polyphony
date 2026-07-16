import { HydrationBoundary, dehydrate } from "@tanstack/react-query"
import { getQueryClient } from "@/lib/query-client"
import { serverHttpClient } from "@/lib/http-client.server"
import { getRoomQueryOptions } from "@/features/rooms/api/get-room"
import { getMessagesQueryOptions } from "@/features/messages/api/get-messages"
import { getModelsQueryOptions } from "@/features/messages/api/get-models"
import { ChatRoom } from "@/features/messages/components/ChatRoom"

/**
 * Server Component that prefetches the room, its messages, and the
 * available models in parallel before first paint, then hands the
 * dehydrated cache to `ChatRoom` via `HydrationBoundary` — avoiding the
 * client-side loading spinner flash the pre-migration `useEffect` fetch had
 * on first navigation into a room.
 */
export default async function ChatRoomPage({
  params,
}: {
  params: Promise<{ roomId: string }>
}) {
  const { roomId } = await params
  const queryClient = getQueryClient()

  await Promise.all([
    queryClient.prefetchQuery(getRoomQueryOptions(roomId, serverHttpClient.get)),
    queryClient.prefetchQuery(
      getMessagesQueryOptions(roomId, serverHttpClient.get),
    ),
    queryClient.prefetchQuery(getModelsQueryOptions(serverHttpClient.get)),
  ])

  return (
    <HydrationBoundary state={dehydrate(queryClient)}>
      <ChatRoom roomId={roomId} />
    </HydrationBoundary>
  )
}
