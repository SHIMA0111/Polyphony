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
 *   non-terminal streaming placeholder (a late created-echo of the AI
 *   placeholder racing behind one or more `token_chunk` frames), the cached
 *   entry's accumulated `content` is preserved instead of being wiped back
 *   to the echo's still-empty content — see the `message_created` branch
 *   below.
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
 *   final message (real content, `status: "completed"`/`"failed"`) --
 *   except `used_context_summary`, which is OR-ed against whatever the
 *   already-cached entry has instead of taken verbatim from the incoming
 *   message (post-review hardening: the server always sets it correctly on
 *   the finalize event -- see `stream.go`'s `consumeAIStream` -- but this
 *   keeps a stray backend regression from silently clearing a "Summarized
 *   history" badge the reader already saw rendered from an earlier
 *   `token_chunk` frame).
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
 * `sender_id` -- a *public* AI message always has a `null` `sender_id` and
 * is never subject to this check, but a *private* AI message is the
 * documented exception (see `server/internal/usecase/message/usecase.go`'s
 * `SendAIMessage` deviation comment): it deliberately records the owning
 * user's id as `sender_id` so `targetUserIDsForVisibility` can scope its
 * delivery, so private AI messages ARE covered by this guard just like
 * private human messages -- no new auth/session endpoint is introduced to
 * make this check possible.
 *
 * @param data - Current cache data, or `undefined` if nothing has loaded yet.
 * @param event - The inbound, already-validated WS event.
 * @param currentUserId - The authenticated user's local id (`useCurrentUser()`'s
 * `id`, sourced from `GET /users/me` -- not `useSession()`'s Kratos
 * `identity.id`, a different UUID under `AUTH_MODE=kratos`), or `undefined`
 * if not yet available -- in which case the sender-mismatch guard is skipped
 * entirely rather than guessed at.
 *
 * Duplicate-placeholder race (wave-9 review finding): `useSendAIMessage`'s
 * `onMutate` prepends a `status: "sending"` AI placeholder (id prefixed
 * `optimistic-ai-`) and only removes it in its own `onSuccess`, once the
 * send POST resolves. If a `message_created` or `token_chunk` WS frame
 * carrying the *real* server-assigned AI message id arrives first (routine,
 * since a WS frame can beat the POST response), the id-based
 * already-present check above never matches the differently-id'd optimistic
 * entry, so both the "sending" placeholder and the new real/streaming entry
 * would render side by side. {@link findMatchingAIPlaceholder} detects that
 * still-pending optimistic entry so the AI branches below replace it in
 * place instead of prepending a second bubble; `onSuccess`'s own
 * remove-if-real-present logic (`use-send-ai-message.ts`) still runs
 * afterward as a no-op fallback in that case (the optimistic id it looks for
 * has already been swapped out here).
 *
 * Concurrent-send follow-up (wave-9 review, round 2): the original fix
 * above matched the *first* `"sending"` AI placeholder it found, which is
 * only correct when at most one AI send is in flight at a time. With two
 * concurrent AI sends in the same room, the real echo/chunk for send B
 * could win the race and land on send A's placeholder -- wrong-bubble
 * content until the eventual finalize event overwrote it. Two or more
 * pending placeholders are now only matched by `in_response_to_message_id`
 * equality against the incoming event's real parent id, never by
 * first-match; see {@link findMatchingAIPlaceholder} and
 * {@link reconcileOptimisticAIParent} for how that real parent id becomes
 * known in the first place, since the placeholder's own
 * `in_response_to_message_id` initially only points at the *optimistic*
 * human id `onMutate` created it alongside (see `use-send-ai-message.ts`).
 * When no such correlation is available yet, the event is left to fall
 * through to the standalone-entry path below rather than guessing among
 * same-status placeholders -- the placeholder-vs-real-id duplicate this
 * self-heals through `onSuccess`'s own exact-id cleanup, same as before.
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

  const existing = findMessageInPages(data, message.id)

  if (event.type === "message_updated") {
    if (!existing) return data
    // Belt-and-suspenders: the server is the source of truth for
    // `used_context_summary` and always sets it correctly on the finalize
    // event (see H1 fix in `stream.go`'s `consumeAIStream`), but OR it
    // against whatever the cache already has anyway so a stray backend
    // regression can never silently wipe a badge the reader has already
    // seen rendered (e.g. from an earlier `token_chunk` frame).
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, {
      ...message,
      used_context_summary: existing.used_context_summary || message.used_context_summary,
    })
  }

  // event.type === "message_created"
  if (existing) {
    if (existing.status === "streaming" && message.status === "streaming") {
      // Late created-echo of the AI placeholder (`SendAIMessageStream`
      // persists it synchronously before any token is generated -- see
      // `applyTokenChunk`'s docstring): its own `content` is still empty at
      // that point, so if one or more `token_chunk` frames already
      // accumulated real content onto this id before the echo arrives,
      // replacing wholesale would wipe that content back out. Preserve the
      // cached accumulated content and OR `used_context_summary`, mirroring
      // the `message_updated` hardening above.
      return replaceMessageInAnyPage(data, (m) => m.id === message.id, {
        ...message,
        content: existing.content,
        used_context_summary: existing.used_context_summary || message.used_context_summary,
      })
    }
    return replaceMessageInAnyPage(data, (m) => m.id === message.id, message)
  }
  if (message.type === "ai") {
    const placeholder = findMatchingAIPlaceholder(data, message.in_response_to_message_id)
    if (placeholder) {
      return replaceMessageInAnyPage(data, (m) => m.id === placeholder.id, message)
    }
    return prependToNewestPage(data, message)
  }

  // message.type === "human": prepend the real echo as before (the
  // duplicate-vs-optimistic-entry race for a *human* message is left to
  // `onSuccess`'s own exact-id cleanup, same as pre-existing behavior --
  // only the AI side needed the placeholder-matching treatment above).
  // Additionally, if this is confirmed to be the current user's own send
  // (never true for another room member's message, since `"sending"`
  // optimistic entries are always local-only -- see `MessageStatus`'s
  // docstring), attempt to bridge any still-pending optimistic AI
  // placeholder's expected parent over to this message's now-real id; see
  // {@link reconcileOptimisticAIParent}.
  const prepended = prependToNewestPage(data, message)
  return currentUserId != null && message.sender_id === currentUserId
    ? reconcileOptimisticAIParent(prepended, message)
    : prepended
}

/**
 * Finds every still-pending `status: "sending"` optimistic AI placeholder
 * (id prefixed `optimistic-ai-`, created by `useSendAIMessage`'s `onMutate`)
 * in the cache -- there can be more than one once two or more AI sends are
 * in flight concurrently in the same room. See {@link findMatchingAIPlaceholder}
 * for how the result set is narrowed down to a single match (or none).
 */
function findSendingOptimisticAIPlaceholders(
  data: MessagesInfiniteData | undefined,
): Message[] {
  if (!data) return []

  const placeholders: Message[] = []
  for (const page of data.pages) {
    for (const m of page.messages) {
      if (m.type === "ai" && m.status === "sending" && m.id.startsWith("optimistic-ai-")) {
        placeholders.push(m)
      }
    }
  }
  return placeholders
}

/**
 * Resolves which (if any) pending optimistic AI placeholder a real AI
 * `message_created` or `token_chunk` event should replace in place -- see
 * {@link mergeMessageEvent}'s "Duplicate-placeholder race" / "Concurrent-send
 * follow-up" docstring for the wave-9 review finding this closes.
 *
 * - **Zero candidates**: nothing to replace -- `undefined`.
 * - **Exactly one candidate**: unambiguous regardless of `expectedParentId`
 *   -- this is the single-concurrent-send case (the overwhelming majority
 *   of sends), matching the pre-fix first-match behavior byte-for-byte.
 * - **Two or more candidates**: only the placeholder whose own
 *   `in_response_to_message_id` has already been reconciled to
 *   `expectedParentId` (see {@link reconcileOptimisticAIParent}) is
 *   returned. `expectedParentId` is `null` for a `token_chunk` -- its
 *   payload carries no parent reference at all (see `ChunkPayload`) -- so a
 *   chunk can never win this branch while two or more sends are racing; it
 *   is the incoming event's own `in_response_to_message_id` (the real human
 *   message id) for a `message_created` AI event. An unreconciled or
 *   non-matching candidate set returns `undefined` rather than guessing --
 *   the caller falls back to creating a standalone entry (pre-fix
 *   behavior), which self-heals once `useSendAIMessage`'s own `onSuccess`
 *   removes the stale placeholder by its exact optimistic id.
 */
function findMatchingAIPlaceholder(
  data: MessagesInfiniteData | undefined,
  expectedParentId: string | null,
): Message | undefined {
  const candidates = findSendingOptimisticAIPlaceholders(data)
  if (candidates.length === 0) return undefined
  if (candidates.length === 1) return candidates[0]
  return candidates.find(
    (c) => expectedParentId != null && c.in_response_to_message_id === expectedParentId,
  )
}

/**
 * Best-effort bridge for the concurrent-AI-send placeholder race (see
 * {@link findMatchingAIPlaceholder}): once a brand new human
 * `message_created` event has been confirmed as this user's own send
 * (`sender_id === currentUserId`, checked by the caller) and its `content`
 * uniquely matches exactly one still-`"sending"` optimistic human
 * placeholder (`useSendAIMessage`'s `onMutate` human echo), that
 * placeholder's real id is now known -- so the paired optimistic AI
 * placeholder (created in the same `onMutate` call, linked via
 * `in_response_to_message_id === optimisticHuman.id`) has its own
 * `in_response_to_message_id` updated from the optimistic human id to this
 * real one.
 *
 * This is what lets {@link findMatchingAIPlaceholder} correlate a *later*
 * AI `message_created`/`token_chunk` event to the right placeholder by
 * `in_response_to_message_id` equality once two or more AI sends are
 * racing in the same room -- without it, no placeholder's parent would
 * ever point at a real id, and the two-or-more-candidates branch would
 * always fall through to the standalone-entry path.
 *
 * Deliberately conservative in two ways, both erring toward "do nothing"
 * over "guess wrong" (the same class of bug this whole mechanism exists to
 * close): the content match is required to be *unique* among pending
 * placeholders (two pending sends with byte-identical text silently skip
 * the bridge rather than guessing between them), and the corresponding AI
 * placeholder must also be uniquely resolvable from the matched human
 * placeholder's id (always true by `onMutate`'s 1:1 construction, checked
 * anyway as a belt-and-suspenders guard). Skipping the bridge only means
 * the eventual AI event falls back to a standalone entry instead of
 * replacing the placeholder in place -- never a wrong-content attach.
 *
 * @param data - Cache data with `humanMessage` already merged in (the
 * caller prepends it before calling this).
 * @param humanMessage - The just-merged, real (non-optimistic) human
 * message.
 */
function reconcileOptimisticAIParent(
  data: MessagesInfiniteData,
  humanMessage: Message,
): MessagesInfiniteData | undefined {
  const pendingHumans: Message[] = []
  for (const page of data.pages) {
    for (const m of page.messages) {
      if (
        m.type === "human" &&
        m.status === "sending" &&
        m.id.startsWith("optimistic-human-") &&
        m.content === humanMessage.content
      ) {
        pendingHumans.push(m)
      }
    }
  }
  if (pendingHumans.length !== 1) return data
  const [pendingHuman] = pendingHumans

  const pairedAIPlaceholders = findSendingOptimisticAIPlaceholders(data).filter(
    (ai) => ai.in_response_to_message_id === pendingHuman.id,
  )
  if (pairedAIPlaceholders.length !== 1) return data
  const [pairedAI] = pairedAIPlaceholders

  return replaceMessageInAnyPage(data, (m) => m.id === pairedAI.id, {
    ...pairedAI,
    in_response_to_message_id: humanMessage.id,
  })
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

  // See `mergeMessageEvent`'s "Duplicate-placeholder race" / "Concurrent-send
  // follow-up" docstring: if the first `token_chunk` for this send beats the
  // POST response, the `onMutate`-created `status: "sending"` optimistic
  // placeholder is still in the cache under a different (`optimistic-ai-*`)
  // id -- replace it in place instead of prepending a second bubble. A chunk
  // carries no parent reference (see `ChunkPayload`), so `expectedParentId`
  // is `null` here -- `findMatchingAIPlaceholder` only resolves a match this
  // way when at most one optimistic AI placeholder is pending; with two or
  // more concurrent sends still unresolved, this intentionally falls through
  // to the standalone-placeholder branch below instead of guessing.
  const optimisticPlaceholder = findMatchingAIPlaceholder(data, null)
  if (optimisticPlaceholder) {
    return replaceMessageInAnyPage(data, (m) => m.id === optimisticPlaceholder.id, placeholder)
  }
  return prependToNewestPage(data, placeholder)
}
