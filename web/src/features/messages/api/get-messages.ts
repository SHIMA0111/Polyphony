import { queryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { Message, MessagePage } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/**
 * `queryOptions()` factory for the `["rooms", roomId, "messages"]` query.
 *
 * Mirrors the pre-migration `listMessages(roomId, undefined, 100)` call
 * shape exactly (a single page, no cursor, `limit=100`) — cursor-based
 * pagination via `useInfiniteQuery` is Step 18's job.
 *
 * The API returns messages in descending order; `queryFn` reverses them so
 * cached data is already display-ordered (oldest first). This also means
 * the cache holds a plain `Message[]` (not the raw `MessagePage`), which is
 * the shape `useSendMessage`/`useSendAIMessage`/`useRegenerateAIMessage`
 * expect when they `setQueryData` onto this same key, and the shape a
 * future WebSocket/streaming handler can append to.
 */
export function getMessagesQueryOptions(
  roomId: string,
  fetcher: Fetcher = apiRequest,
) {
  return queryOptions({
    queryKey: ["rooms", roomId, "messages"] as const,
    queryFn: async (): Promise<Message[]> => {
      const page = await fetcher<MessagePage>(
        `/rooms/${roomId}/messages?limit=100`,
      )
      return [...page.messages].reverse()
    },
  })
}
