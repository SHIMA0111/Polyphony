import { infiniteQueryOptions } from "@tanstack/react-query"
import { apiRequest } from "@/lib/http-client"
import type { Message, MessagePage } from "../types"

/** Fetcher shape shared by every per-feature `queryOptions()` factory. */
type Fetcher = <T>(path: string) => Promise<T>

/** Default page size for `GET /rooms/:roomId/messages`. */
export const MESSAGES_PAGE_LIMIT = 50

/**
 * Calls `GET /rooms/:roomId/messages`, returning a single page exactly as
 * the API shapes it: `messages` newest-first (`ORDER BY sequence DESC`),
 * `cursor` (when supplied) is the id of the oldest message already fetched,
 * and `next_cursor` is the cursor to pass to fetch the next *older* page
 * (`null` once there is no more history).
 */
export function getMessages(
  roomId: string,
  { cursor, limit = MESSAGES_PAGE_LIMIT }: { cursor?: string; limit?: number } = {},
  fetcher: Fetcher = apiRequest,
): Promise<MessagePage> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) {
    params.set("cursor", cursor)
  }
  return fetcher<MessagePage>(`/rooms/${roomId}/messages?${params.toString()}`)
}

/**
 * `infiniteQueryOptions()` factory for the `["rooms", roomId, "messages"]`
 * query — cursor-based backward/upward pagination via `useInfiniteQuery`.
 *
 * `pages[0]` is always the most recently fetched (newest) page; each
 * subsequent page (fetched via `fetchNextPage`, keyed on the previous page's
 * `next_cursor`) represents *older* history. Each page's own `messages`
 * array keeps the API's newest-first order — see
 * `../lib/flatten-message-pages.ts` for the display-ordering step that turns
 * this shape into a flat, oldest-to-newest `Message[]`.
 *
 * Keeping the query key identical to the pre-Step-29 single-page `useQuery`
 * version (`["rooms", roomId, "messages"]`) is deliberate: Step 35's
 * WebSocket cache-merge targets this same key and must not need to change.
 */
export function getMessagesInfiniteQueryOptions(
  roomId: string,
  fetcher: Fetcher = apiRequest,
) {
  return infiniteQueryOptions({
    queryKey: ["rooms", roomId, "messages"] as const,
    queryFn: ({ pageParam }): Promise<MessagePage> =>
      getMessages(roomId, { cursor: pageParam, limit: MESSAGES_PAGE_LIMIT }, fetcher),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage: MessagePage) => lastPage.next_cursor ?? undefined,
  })
}

// Re-exported so callers that only need the element type don't have to reach
// into `../types` themselves.
export type { Message, MessagePage }
