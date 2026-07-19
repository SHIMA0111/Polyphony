"use client"

import { useInfiniteQuery } from "@tanstack/react-query"
import { getMessagesInfiniteQueryOptions } from "../api/get-messages"

/**
 * Cursor-paginated messages in a room, sourced from
 * `GET /api/proxy/rooms/:roomId/messages`.
 *
 * Returns the raw `useInfiniteQuery` result (`data.pages`, `fetchNextPage`,
 * `hasNextPage`, `isFetchingNextPage`, ...); callers that need a flat,
 * display-ordered list should run `data.pages` through
 * `../lib/flatten-message-pages.ts`'s `flattenMessagePages` (done for them
 * by `useChatRoom`).
 */
export function useMessages(roomId: string) {
  return useInfiniteQuery(getMessagesInfiniteQueryOptions(roomId))
}
