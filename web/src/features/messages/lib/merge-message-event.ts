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
 *   echo of the same message arrives. Exception: if the cached entry is
 *   still `status: "streaming"` and the incoming event is itself a
 *   non-terminal streaming placeholder (`status: "streaming"`, no finished
 *   content of its own), a wholesale replace is skipped in favor of
 *   preserving the cached entry's already-accumulated content and OR-ing
 *   the two `used_context_summary` flags — see the inline comment at that
 *   branch for the race this guards against.
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
 * `sender_id`. A human message always does. A *public* AI message has a
 * `null` `sender_id`, but that's moot for this guard either way since its
 * `visibility` is never `"private"`. A *private* AI message is the case
 * this guard actually covers for AI replies: it deliberately records the
 * owning user's id as its `sender_id` (a documented deviation from the
 * usual "AI messages have a nil SenderID" convention -- see
 * `server/internal/usecase/message/usecase.go`'s `SendAIMessage`), which is
 * exactly what lets this guard scope a private AI reply to its owner the
 * same way it scopes a private human message -- no new auth/session
 * endpoint is introduced to make this check possible.
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
 *
 * Per-request correlation under concurrent sends (wave-9 review follow-up):
 * the placeholder lookup above is unambiguous when exactly one AI send is in
 * flight, but two or more concurrent `useSendAIMessage` calls each leave
 * their own `status: "sending"` placeholder in the cache at the same time --
 * a first-match there would splice one send's echo/chunk onto a *different*
 * send's bubble (wrong-bubble content until the next finalize). With 2+
 * pending placeholders, {@link findSendingOptimisticAIPlaceholder} instead
 * requires an exact `in_response_to_message_id` match against the real human
 * message id, once that is known for the send in question. That id becomes
 * known either through `useSendAIMessage`'s own `onSuccess` (which replaces
 * the whole placeholder directly by its closed-over optimistic id,
 * sidestepping this lookup entirely) or, if a WS frame beats that response,
 * through the human-echo reconciliation branch below ({@link
 * findUniqueSendingOptimisticHuman} / {@link findPendingAIPlaceholderExpecting}),
 * which patches a still-pending AI placeholder's expected parent from its
 * original optimistic human id to the real one as soon as that send's own
 * human `message_created` echo lands. Until one of those has happened for a
 * given send, its AI echo/chunk cannot yet be told apart from another
 * concurrent send's -- see `applyTokenChunk`'s matching fallback, which
 * creates a standalone streaming entry rather than guessing in exactly that
 * window.
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

  const existingMessage = findMessageInPages(data, message.id)
  const alreadyPresent = existingMessage !== undefined

  if (event.type === "message_updated") {
    if (!alreadyPresent) return data
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, message)
  }

  // event.type === "message_created"
  if (alreadyPresent) {
    if (existingMessage.status === "streaming" && message.status === "streaming") {
      // A late-arriving created echo of the same in-flight AI placeholder: a
      // `token_chunk` can race ahead of this echo and already be
      // accumulating real content in the cache (see `applyTokenChunk`'s own
      // first-chunk-creates-placeholder branch) by the time it lands. This
      // echo's own `message` payload is still the placeholder's original,
      // empty content -- a wholesale replace with it would wipe the
      // accumulated progress back to "". Preserve the cached content and OR
      // the two `used_context_summary` flags instead, mirroring
      // `applyTokenChunk`'s identical merge for the reverse race.
      return replaceMessageInAnyPage(data, (m) => m.id === message.id, {
        ...message,
        content: existingMessage.content,
        used_context_summary:
          existingMessage.used_context_summary || message.used_context_summary,
      })
    }
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, message)
  }
  if (message.type === "ai") {
    const placeholder = findSendingOptimisticAIPlaceholder(
      data,
      message.in_response_to_message_id,
    )
    if (placeholder) {
      return replaceMessageInAnyPage(data, (m) => m.id === placeholder.id, message)
    }
  } else {
    // message.type === "human": reconcile the optimistic human echo (if
    // this is one of ours) and propagate its now-known real id to any AI
    // placeholder waiting on it -- see the "Per-request correlation under
    // concurrent sends" docstring above.
    const optimisticHuman = findUniqueSendingOptimisticHuman(data, message.content)
    if (optimisticHuman) {
      const reconciled = replaceMessageInAnyPage(
        data,
        (m) => m.id === optimisticHuman.id,
        message,
      )
      const pendingAIPlaceholder = findPendingAIPlaceholderExpecting(
        reconciled,
        optimisticHuman.id,
      )
      return pendingAIPlaceholder
        ? replaceMessageInAnyPage(reconciled, (m) => m.id === pendingAIPlaceholder.id, {
            ...pendingAIPlaceholder,
            in_response_to_message_id: message.id,
          })
        : reconciled
    }
  }
  return prependToNewestPage(data, message)
}

/**
 * Finds a still-pending `status: "sending"` optimistic AI placeholder (id
 * prefixed `optimistic-ai-`, created by `useSendAIMessage`'s `onMutate`) in
 * the cache, if one can be identified unambiguously. See {@link
 * mergeMessageEvent}'s docstring ("Duplicate-placeholder race" and
 * "Per-request correlation under concurrent sends") for why the AI merge
 * paths use this instead of the usual id-based already-present check.
 *
 * - Zero pending placeholders: nothing to match, `undefined`.
 * - Exactly one: returned unconditionally, regardless of
 *   `expectedParentId` -- with only one AI send in flight it is the only
 *   possible match, which is what keeps a single concurrent send's behavior
 *   byte-identical to before per-request correlation existed.
 * - Two or more (concurrent sends): a guess would risk attaching this
 *   event's content to the wrong bubble, so only an exact
 *   `in_response_to_message_id` match against `expectedParentId` (the real
 *   human message id this event is known to correlate to, when available)
 *   is accepted. Anything else -- no `expectedParentId` yet, zero matches,
 *   or (should never happen) more than one -- is treated as "not yet
 *   correlatable" and returns `undefined` rather than guessing; the caller
 *   falls back to inserting a standalone/new entry instead.
 *
 * @param expectedParentId - The real human message id this event's AI
 *   message correlates to, if already known. `undefined`/`null` when the
 *   caller has no such signal (e.g. a `token_chunk`, whose payload never
 *   carries `in_response_to_message_id`).
 */
function findSendingOptimisticAIPlaceholder(
  data: MessagesInfiniteData | undefined,
  expectedParentId?: string | null,
): Message | undefined {
  if (!data) return undefined

  const candidates: Message[] = []
  for (const page of data.pages) {
    for (const m of page.messages) {
      if (m.type === "ai" && m.status === "sending" && m.id.startsWith("optimistic-ai-")) {
        candidates.push(m)
      }
    }
  }

  if (candidates.length === 0) return undefined
  if (candidates.length === 1) return candidates[0]

  if (expectedParentId == null) return undefined
  const matches = candidates.filter((c) => c.in_response_to_message_id === expectedParentId)
  return matches.length === 1 ? matches[0] : undefined
}

/**
 * Finds the single still-pending `status: "sending"` optimistic human entry
 * (id prefixed `optimistic-human-`, created by `useSendAIMessage`'s
 * `onMutate`) whose `content` exactly matches `content`, if there is exactly
 * one such candidate in the cache.
 *
 * A real human `message_created` echo carries only the server-assigned id,
 * never the client-generated optimistic one, so verbatim `content` is the
 * only signal available to correlate it back to the optimistic entry that
 * spawned it. Two or more candidates (e.g. two concurrent sends with
 * identical text) is genuinely ambiguous and is never guessed at --
 * `undefined` is returned, leaving reconciliation to `useSendAIMessage`'s
 * own `onSuccess`, which has no such ambiguity since it closes over the
 * exact optimistic id.
 */
function findUniqueSendingOptimisticHuman(
  data: MessagesInfiniteData | undefined,
  content: string,
): Message | undefined {
  if (!data) return undefined

  let match: Message | undefined
  for (const page of data.pages) {
    for (const m of page.messages) {
      if (
        m.type === "human" &&
        m.status === "sending" &&
        m.id.startsWith("optimistic-human-") &&
        m.content === content
      ) {
        if (match) return undefined
        match = m
      }
    }
  }
  return match
}

/**
 * Finds the still-pending `status: "sending"` optimistic AI placeholder
 * whose `in_response_to_message_id` equals `parentId`, if any.
 *
 * Only ever called right after `parentId` (an optimistic human id) has just
 * been confirmed by {@link findUniqueSendingOptimisticHuman} to identify
 * exactly one pending send, so -- unlike {@link
 * findSendingOptimisticAIPlaceholder} -- no ambiguity is possible here:
 * `useSendAIMessage`'s `onMutate` links each AI placeholder to its own
 * send's optimistic human id one-to-one.
 */
function findPendingAIPlaceholderExpecting(
  data: MessagesInfiniteData | undefined,
  parentId: string,
): Message | undefined {
  if (!data) return undefined

  for (const page of data.pages) {
    const found = page.messages.find(
      (m) =>
        m.type === "ai" && m.status === "sending" && m.in_response_to_message_id === parentId,
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
 *   to the newest page instead, seeded with just this chunk's delta --
 *   *unless* exactly one `status: "sending"` optimistic AI placeholder is
 *   still pending, in which case the chunk attaches to it in place instead
 *   (see {@link findSendingOptimisticAIPlaceholder}). A `chunk` payload
 *   never carries `in_response_to_message_id`, so with two or more pending
 *   placeholders (concurrent sends) there is no signal to correlate this
 *   chunk to the right one -- the standalone-entry path is taken instead of
 *   guessing; the eventual `message_created` echo (correlated via {@link
 *   mergeMessageEvent}'s own AI branch, which does have the real id to
 *   match against) or `message_updated` finalize reconciles it properly.
 *   Later chunks then append to it by the same `message_id`; the eventual
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
  // place instead of prepending a second bubble. No `expectedParentId` is
  // passed -- a chunk never carries `in_response_to_message_id` -- so with
  // two or more concurrent sends pending this only matches when exactly one
  // candidate exists; otherwise it falls through to the standalone-entry
  // path below rather than guessing which send this chunk belongs to.
  const optimisticPlaceholder = findSendingOptimisticAIPlaceholder(data)
  if (optimisticPlaceholder) {
    return replaceMessageInAnyPage(data, (m) => m.id === optimisticPlaceholder.id, placeholder)
  }
  return prependToNewestPage(data, placeholder)
}
