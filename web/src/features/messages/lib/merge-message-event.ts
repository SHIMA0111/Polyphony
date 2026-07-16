import {
  findMessageInPages,
  prependToNewestPage,
  replaceMessageInAnyPage,
  type MessagesInfiniteData,
} from "./message-cache"
import type { Message } from "../types"
import type { RoomSocketEvent } from "../types/ws-events"

/**
 * Merges a single inbound `RoomSocketEvent` (a `message_created`,
 * `message_updated`, or `token_chunk` WS frame — see `../types/ws-events.ts`)
 * into the `["rooms", roomId, "messages"]` `useInfiniteQuery` cache Step 29
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
 *   or an initial page fetch has already brought the message in. This is
 *   also Step 51/54's streaming *finalize* signal: the AI placeholder's
 *   accumulated in-flight content is replaced wholesale by the authoritative
 *   final message (real content, `status: "completed"`/`"failed"`).
 * - `token_chunk` (Step 54) appends `chunk.delta` to the message identified
 *   by `chunk.message_id`, marking it `status: "streaming"` — see
 *   {@link applyTokenChunk}'s own docstring for the insert-or-append and
 *   finalize-race handling this implements.
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
  if (event.type === "token_chunk") {
    return applyTokenChunk(data, event.room_id, event.chunk)
  }

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

/**
 * Applies a single `token_chunk` frame's delta to the cache: the core of
 * Step 54's streaming render.
 *
 * - **Append to existing placeholder**: if a message with `chunk.message_id`
 *   is already cached (normally inserted by the WS `message_created` echo
 *   of the AI placeholder `SendAIMessageStream` persists synchronously
 *   before streaming begins — see `server/internal/usecase/message/stream.go`
 *   — or, in the optimistic-send window, by the mutation's own `onSuccess`),
 *   its `content` is extended by `chunk.delta` and its `status` is set to
 *   `"streaming"`.
 * - **First-chunk-creates-placeholder**: if no message with that id is
 *   cached yet (chunk delivery is not strictly ordered relative to the
 *   `message_created` echo or the mutation's HTTP response, both of which
 *   race it independently), a new `status: "streaming"` entry is prepended
 *   to the newest page instead, seeded with just this chunk's delta. Later
 *   chunks then append to it by the same `message_id`; the eventual
 *   `message_updated` finalize event replaces it wholesale with the
 *   authoritative final message (fixing up `sequence`,
 *   `in_response_to_message_id`, timestamps, etc. — this placeholder's
 *   values for those fields are only ever transiently displayed).
 * - **Idempotent against a finalize race**: a `token_chunk` for a message
 *   that has already been finalized (`status: "completed"` or `"failed"`)
 *   is a no-op — a stray or late-delivered chunk frame must never reopen an
 *   already-settled bubble back into a streaming state or corrupt its
 *   authoritative final content by appending to it.
 *
 * @param data - Current cache data, or `undefined` if nothing has loaded yet.
 * @param roomId - The chunk event's `room_id` (used only if a new
 *   placeholder must be created).
 * @param chunk - The chunk frame's payload.
 */
function applyTokenChunk(
  data: MessagesInfiniteData | undefined,
  roomId: string,
  chunk: { message_id: string; delta: string; summary_used: boolean },
): MessagesInfiniteData | undefined {
  const existing = findMessageInPages(data, chunk.message_id)

  if (existing) {
    if (existing.status === "completed" || existing.status === "failed") {
      return data
    }
    return replaceMessageInAnyPage(data, (m) => m.id === chunk.message_id, {
      ...existing,
      content: existing.content + chunk.delta,
      status: "streaming",
      used_context_summary: existing.used_context_summary || chunk.summary_used,
    })
  }

  const now = new Date().toISOString()
  const placeholder: Message = {
    id: chunk.message_id,
    room_id: roomId,
    sender_id: null,
    content: chunk.delta,
    type: "ai",
    status: "streaming",
    sequence: -1,
    in_response_to_message_id: null,
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: chunk.summary_used,
    created_at: now,
    updated_at: now,
  }
  return prependToNewestPage(data, placeholder)
}
