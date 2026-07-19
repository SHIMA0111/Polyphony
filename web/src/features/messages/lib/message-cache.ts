import type { InfiniteData } from "@tanstack/react-query"
import type { Message, MessagePage, MessageStatus } from "../types"

/**
 * The `["rooms", roomId, "messages"]` query's cache shape once migrated to
 * `useInfiniteQuery` (see `../api/get-messages.ts`): `pages[0]` is the
 * newest page, `pages[1..]` are progressively older history loaded via
 * `fetchNextPage`.
 */
export type MessagesInfiniteData = InfiniteData<MessagePage, string | undefined>

/**
 * Prepends `message` to the front of the newest (first) page's `messages`
 * array. Since a page's `messages` are newest-first, prepending here means
 * "this is now the newest message in the room" — the correct place for a
 * just-sent optimistic entry.
 *
 * Never touches `pages[1..]`, which represent already-loaded older history;
 * per the cursor pagination contract, optimistic sends only ever affect the
 * newest page.
 *
 * @param data - Current cache data, or `undefined` if nothing has loaded yet.
 * @param message - The message to prepend.
 */
export function prependToNewestPage(
  data: MessagesInfiniteData | undefined,
  message: Message,
): MessagesInfiniteData {
  if (!data || data.pages.length === 0) {
    return {
      pages: [{ messages: [message], next_cursor: null }],
      pageParams: [undefined],
    }
  }

  const [firstPage, ...rest] = data.pages
  return {
    ...data,
    pages: [
      { ...firstPage, messages: [message, ...firstPage.messages] },
      ...rest,
    ],
  }
}

/**
 * Replaces the first message matching `predicate` within the newest page
 * with `replacement` (e.g. swapping a client-generated optimistic entry for
 * the real, server-returned message on mutation success). Leaves the cache
 * unchanged (structurally, via a no-op map) if no message matches.
 *
 * Only searches `pages[0]` — reconciling an optimistic send never needs to
 * look at older, already-loaded pages.
 */
export function replaceInNewestPage(
  data: MessagesInfiniteData | undefined,
  predicate: (message: Message) => boolean,
  replacement: Message,
): MessagesInfiniteData | undefined {
  if (!data || data.pages.length === 0) return data

  const [firstPage, ...rest] = data.pages
  return {
    ...data,
    pages: [
      {
        ...firstPage,
        messages: firstPage.messages.map((m) => (predicate(m) ? replacement : m)),
      },
      ...rest,
    ],
  }
}

/**
 * Updates the `status` of the message with the given `id` in place within
 * the newest page (e.g. rolling an optimistic "sending" entry back to
 * "failed" on mutation error, without removing it from the transcript).
 */
export function markStatusInNewestPage(
  data: MessagesInfiniteData | undefined,
  id: string,
  status: MessageStatus,
): MessagesInfiniteData | undefined {
  if (!data || data.pages.length === 0) return data

  const [firstPage, ...rest] = data.pages
  return {
    ...data,
    pages: [
      {
        ...firstPage,
        messages: firstPage.messages.map((m) => (m.id === id ? { ...m, status } : m)),
      },
      ...rest,
    ],
  }
}

/**
 * Removes the message with the given `id` from the newest page (e.g.
 * dropping a placeholder that never represented anything retryable, or
 * clearing a stale failed entry immediately before re-enqueuing a retry so
 * the retry's own optimistic entry doesn't produce a duplicate bubble).
 */
export function removeFromNewestPage(
  data: MessagesInfiniteData | undefined,
  id: string,
): MessagesInfiniteData | undefined {
  if (!data || data.pages.length === 0) return data

  const [firstPage, ...rest] = data.pages
  return {
    ...data,
    pages: [
      { ...firstPage, messages: firstPage.messages.filter((m) => m.id !== id) },
      ...rest,
    ],
  }
}

/**
 * Finds the message with the given `id` across every loaded page, or
 * `undefined` if it is not present anywhere in the cache yet.
 *
 * Used by `../lib/merge-message-event.ts` to decide whether an inbound WS
 * `message_created` event is a genuinely new message (append it) or a
 * duplicate/echo of something already reconciled into the cache by the
 * optimistic-send path, a REST response, or a previously-merged WS event
 * (reconcile in place instead of appending a second copy).
 */
export function findMessageInPages(
  data: MessagesInfiniteData | undefined,
  id: string,
): Message | undefined {
  if (!data) return undefined

  for (const page of data.pages) {
    const found = page.messages.find((m) => m.id === id)
    if (found) return found
  }
  return undefined
}

/**
 * Replaces the first message matching `predicate` with `replacement`,
 * searching *every* loaded page rather than just the newest one.
 *
 * Unlike optimistic sends (always in `pages[0]`), a regenerated AI message
 * can legitimately live in any already-loaded page once the reader has
 * scrolled up through history, so reconciling `RegenerateAIMessage`'s
 * response has to search across all of them. Also reused by
 * `useUpdateMessageExclude` (Step 38), since the message being toggled can
 * likewise live in any already-loaded page.
 */
export function replaceMessageInAnyPage(
  data: MessagesInfiniteData | undefined,
  predicate: (message: Message) => boolean,
  replacement: Message,
): MessagesInfiniteData | undefined {
  if (!data) return data

  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      messages: page.messages.map((m) => (predicate(m) ? replacement : m)),
    })),
  }
}

/**
 * Removes the message with the given `id` from every loaded page (unlike
 * {@link removeFromNewestPage}, which only ever looks at `pages[0]`).
 *
 * Used by `useDeleteMessage` (Step 38): a soft-deleted message can live in
 * any already-loaded page once the reader has scrolled up through history,
 * so the client-side removal that mirrors the server's own exclusion of
 * soft-deleted rows from future `GET` responses has to search across all of
 * them, not just the newest one.
 */
export function removeMessageInAnyPage(
  data: MessagesInfiniteData | undefined,
  id: string,
): MessagesInfiniteData | undefined {
  if (!data) return data

  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      messages: page.messages.filter((m) => m.id !== id),
    })),
  }
}
