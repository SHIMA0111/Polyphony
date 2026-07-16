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
 * Defensive sender-mismatch guard (Step 47, private AI mode): the server's
 * per-user `MessageHub` delivery (Step 41) is the sole authority for
 * scoping a `visibility: "private"` message/event to its own sender's
 * connection -- this client should never actually receive a private event
 * whose `sender_id` doesn't match the current user. As a cheap
 * belt-and-suspenders check that costs nothing when the server behaves
 * correctly, any such event is dropped (logged via a single
 * `console.error`) instead of merged into the cache. The check only runs
 * when `currentUserId` is available and the message actually carries a
 * `sender_id` (a human message; AI messages have a `null` `sender_id` and
 * are never subject to this check) -- no new auth/session endpoint is
 * introduced to make this check possible.
 *
 * @param data - Current cache data, or `undefined` if nothing has loaded yet.
 * @param event - The inbound, already-validated WS event.
 * @param currentUserId - The authenticated user's id (`useSession()`'s
 * `identity.id`), or `undefined` if not yet available -- in which case the
 * sender-mismatch guard is skipped entirely rather than guessed at.
 *
 * Duplicate-placeholder race (wave-9 review finding): `useSendAIMessage`'s
 * `onMutate` prepends a `status: "sending"` AI placeholder (id prefixed
 * `optimistic-ai-`) and only removes it in its own `onSuccess`, once the
 * send POST resolves. If a `message_created` or `token_chunk` WS frame
 * carrying the *real* server-assigned AI message id arrives first (routine,
 * since a WS frame can beat the POST response), the id-based
 * already-present check above never matches the differently-id'd optimistic
 * entry, so both the "sending" placeholder and the new real/streaming entry
 * would render side by side. {@link findSendingOptimisticAIPlaceholder}
 * detects that still-pending optimistic entry so the AI branches below
 * replace it in place instead of prepending a second bubble; `onSuccess`'s
 * own remove-if-real-present logic (`use-send-ai-message.ts`) still runs
 * afterward as a no-op fallback in that case (the optimistic id it looks for
 * has already been swapped out here).
 */
export function mergeMessageEvent(
  data: MessagesInfiniteData | undefined,
  event: RoomSocketEvent,
  currentUserId?: string,
): MessagesInfiniteData | undefined {
  if (event.type === "token_chunk") {
    return applyTokenChunk(data, event.room_id, event.chunk)
  }

  const { message } = event

  if (
    message.visibility === "private" &&
    currentUserId != null &&
    message.sender_id != null &&
    message.sender_id !== currentUserId
  ) {
    console.error(
      "Dropping private message event not addressed to the current user",
      { messageId: message.id, senderId: message.sender_id, currentUserId },
    )
    return data
  }

  const alreadyPresent = findMessageInPages(data, message.id) !== undefined

  if (event.type === "message_updated") {
    if (!alreadyPresent) return data
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, message)
  }

  // event.type === "message_created"
  if (alreadyPresent) {
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, message)
  }
  if (message.type === "ai") {
    const placeholder = findSendingOptimisticAIPlaceholder(data)
    if (placeholder) {
      return replaceMessageInAnyPage(data, (m) => m.id === placeholder.id, message)
    }
  }
  return prependToNewestPage(data, message)
}

/**
 * Finds a still-pending `status: "sending"` optimistic AI placeholder (id
 * prefixed `optimistic-ai-`, created by `useSendAIMessage`'s `onMutate`) in
 * the cache, if one is present. See {@link mergeMessageEvent}'s docstring
 * ("Duplicate-placeholder race") for why the AI merge paths use this instead
 * of the usual id-based already-present check.
 */
function findSendingOptimisticAIPlaceholder(
  data: MessagesInfiniteData | undefined,
): Message | undefined {
  if (!data) return undefined

  for (const page of data.pages) {
    const found = page.messages.find(
      (m) => m.type === "ai" && m.status === "sending" && m.id.startsWith("optimistic-ai-"),
    )
    if (found) return found
  }
  return undefined
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
    visibility: "public",
    created_at: now,
    updated_at: now,
  }

  // See `mergeMessageEvent`'s "Duplicate-placeholder race" docstring: if the
  // first `token_chunk` for this send beats the POST response, the
  // `onMutate`-created `status: "sending"` optimistic placeholder is still
  // in the cache under a different (`optimistic-ai-*`) id -- replace it in
  // place instead of prepending a second bubble.
  const optimisticPlaceholder = findSendingOptimisticAIPlaceholder(data)
  if (optimisticPlaceholder) {
    return replaceMessageInAnyPage(data, (m) => m.id === optimisticPlaceholder.id, placeholder)
  }
  return prependToNewestPage(data, placeholder)
}
