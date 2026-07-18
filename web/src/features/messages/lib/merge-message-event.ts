import {
  findMessageInPages,
  prependToNewestPage,
  replaceMessageInAnyPage,
  type MessagesInfiniteData,
} from "./message-cache"
import type { RoomSocketEvent } from "../types/ws-events"

/**
 * Merges a single inbound `RoomSocketEvent` (a `message_created` or
 * `message_updated` WS frame — see `../types/ws-events.ts`) into the
 * `["rooms", roomId, "messages"]` `useInfiniteQuery` cache Step 29
 * established, called from `use-room-socket.ts`'s `onmessage` handler via
 * `queryClient.setQueryData(queryKey, (old) => mergeMessageEvent(old, event))`.
 *
 * De-dupes by message `id` against everything already cached (optimistic
 * entries already reconciled by the send mutations, REST-fetched pages, and
 * previously-merged WS events):
 * - `message_created` for an id already present anywhere in the cache is
 *   reconciled in place (the server-authoritative copy replaces whatever was
 *   there) rather than appended a second time — this is what keeps the
 *   sender's own optimistic entry from becoming a duplicate once its own WS
 *   echo of the same message arrives.
 * - `message_created` for an id not yet present is prepended to the newest
 *   page only (see `prependToNewestPage`'s docstring for why: cursor
 *   pagination guarantees a live-created message is always newer than every
 *   already-loaded page).
 * - `message_updated` patches the existing entry in place, searching every
 *   loaded page (a regenerated AI message can live in any page once the
 *   reader has scrolled up through history) — if the target id isn't present
 *   in the cache at all yet, this is a no-op rather than an insert, since an
 *   "updated" event only makes sense once the corresponding "created" event
 *   or an initial page fetch has already brought the message in.
 *
 * Never re-sorts or re-derives page boundaries from `sequence` — page/array
 * order is preserved exactly as `prependToNewestPage`/`replaceMessageInAnyPage`
 * leave it, so events arriving out of send order still land in a stable,
 * predictable position rather than reshuffling the whole cache.
 *
 * @param data - Current cache data, or `undefined` if nothing has loaded yet.
 * @param event - The inbound, already-validated WS event.
 */
export function mergeMessageEvent(
  data: MessagesInfiniteData | undefined,
  event: RoomSocketEvent,
): MessagesInfiniteData | undefined {
  const { message } = event
  const alreadyPresent = findMessageInPages(data, message.id) !== undefined

  if (event.type === "message_updated") {
    if (!alreadyPresent) return data
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, message)
  }

  // event.type === "message_created"
  if (alreadyPresent) {
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, message)
  }
  return prependToNewestPage(data, message)
}
