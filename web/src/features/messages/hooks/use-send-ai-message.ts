"use client"

import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { sendAIMessage, sendAIMessageStream } from "../api/send-ai-message"
import { toaster } from "@/components/ui/toaster"
import {
  findMessageInPages,
  markStatusInNewestPage,
  prependToNewestPage,
  removeFromNewestPage,
  replaceInNewestPage,
  type MessagesInfiniteData,
} from "../lib/message-cache"
import type { Message } from "../types"

/**
 * The original send parameters a failed AI send needs to be retried
 * faithfully -- recorded in this hook's own `onError` (keyed by the failed
 * human message's optimistic id) and consulted by `useChatRoom.handleRetry`
 * via {@link takeFailedAISendIntent} to decide whether a retry should go
 * through `useSendAIMessage` (with these exact parameters) instead of the
 * plain `useSendMessage` mutation. Carries `stream`/`private` alongside
 * `model` -- an earlier version of this map only carried `model`, so a
 * retry of a failed private (or non-streaming, e.g. attachment-flow) AI
 * send silently downgraded to a public/streaming send instead of replaying
 * the original request.
 */
export interface FailedAISendIntent {
  model?: string
  stream: boolean
  /**
   * Whether the failed send was a private AI send. Restored verbatim on
   * retry (see `useChatRoom.handleRetry`) so a retried private send stays
   * private instead of silently downgrading to a public one.
   */
  private: boolean
  /**
   * Staged attachment ids the failed send was carrying, if any. Restored on
   * retry (see `useChatRoom.handleRetry`) so a retry of a failed attachment
   * send replays the link + regenerate workflow instead of the bare AI-send
   * mutation, which would silently drop the attachments. Empty/`undefined`
   * for a failed AI send that had no attachments.
   */
  attachmentIds?: string[]
}

/**
 * Query key under which the retry-intent map for `roomId` is stored in the
 * `QueryClient`, keyed by each failed AI send's human-echo optimistic id.
 * Kept in the query cache -- rather than a component-local `useRef` -- so a
 * failed AI send's retry intent survives a remount of the chat room screen
 * (e.g. navigating away and back before retrying a failed send): the
 * `QueryClient` instance outlives any single mount of
 * `useSendAIMessage`/`useChatRoom`, while a ref does not. No component ever
 * subscribes to this key via `useQuery`; it is only ever read/written
 * directly through `queryClient.getQueryData`/`setQueryData`.
 */
export function failedAISendIntentQueryKey(roomId: string) {
  return ["retry-intent", roomId] as const
}

/**
 * Reads and removes any {@link FailedAISendIntent} recorded for
 * `humanMessageId` in `roomId`'s retry-intent map, returning it (or
 * `undefined` for a failed message that originated from a plain, non-AI
 * send). Called from `useChatRoom.handleRetry`. Removed unconditionally as
 * soon as it is consulted -- whether the ensuing retry itself succeeds or
 * fails -- since a retry that fails again repopulates the map under the
 * *new* optimistic id this hook's own `onError` produces for that new
 * attempt; leaving the old entry behind would only grow the map without it
 * ever being read again.
 */
export function takeFailedAISendIntent(
  queryClient: QueryClient,
  roomId: string,
  humanMessageId: string,
): FailedAISendIntent | undefined {
  const key = failedAISendIntentQueryKey(roomId)
  const current = queryClient.getQueryData<Map<string, FailedAISendIntent>>(key)
  const intent = current?.get(humanMessageId)
  if (current?.has(humanMessageId)) {
    const next = new Map(current)
    next.delete(humanMessageId)
    queryClient.setQueryData(key, next)
  }
  return intent
}

export interface SendAIMessageInput {
  content: string
  model?: string
  /**
   * Private AI mode flag (Step 41's `SendAIMessageRequest.Private`); the
   * exact wire field name, kept as-is on this input rather than renamed to
   * an `isPrivate` boolean, so no mapping step is needed before it reaches
   * `sendAIMessage`. Defaults to `false`.
   */
  private?: boolean
  /**
   * Whether to use the streaming endpoint (default `true`). `useChatRoom`'s
   * attachment flow passes `false`: a send-with-attachments is immediately
   * followed by a `RegenerateAIMessage` call (so the model actually sees the
   * just-linked images), and regenerating while the original streamed reply
   * is still in flight races the stream's own finalize -- whichever write
   * lands last clobbers the other's content (observed live in the wave-7
   * integration run as the streamed text overwriting the Vision-aware
   * regenerated text). The non-streaming endpoint only resolves once the
   * throwaway first reply is complete, so the follow-up regenerate always
   * targets a settled (`"completed"`/`"failed"`) message. Ignored (treated
   * as `false`) when `private` is set -- `StreamAI` rejects private sends
   * with HTTP 400 regardless.
   */
  stream?: boolean
  /**
   * Staged attachment ids for this send, if any. Never sent to the API by
   * this mutation itself (`mutationFn` below ignores it) -- `useChatRoom`
   * links attachments to the message in a follow-up call once it exists
   * (see its `linkAttachments`). Carried on the input purely so a failed
   * send's `onError` below can capture it into the retry-intent map,
   * letting `useChatRoom.handleRetry` replay the link + regenerate workflow
   * instead of silently dropping the attachments on retry.
   */
  attachmentIds?: string[]
}

/** Context carried from `onMutate` through to `onSuccess`/`onError`. */
interface SendAIMessageContext {
  humanOptimisticId: string
  aiOptimisticId: string
}

/**
 * Sends a message with an AI response via Step 51's streaming endpoint
 * (`sendAIMessageStream`, `POST /rooms/:roomId/messages/ai/stream`),
 * optimistically appending both a `status: "sending"` human echo and a
 * `status: "sending"` AI placeholder (rendered by `MessageBubble` as a
 * `ThinkingBubble`) the instant the mutation is invoked — no wait for the
 * round trip.
 *
 * When `private: true` is requested, this mutation routes to the
 * non-streaming `sendAIMessage` (`../api/send-ai-message`) instead —
 * `StreamAI` rejects `private: true` with HTTP 400 (private AI mode is not
 * yet supported for streaming) — so its `onSuccess` response already
 * carries the finished AI text and no `token_chunk` frames follow. For the
 * (default) non-private, streaming path, this mutation's own `202` response
 * never carries the finished AI text: `ai_message.status` is
 * `"streaming"` with empty `content` on the happy path. The actual response
 * text arrives afterward as `token_chunk` WebSocket frames merged by
 * `mergeMessageEvent` (see `../lib/merge-message-event.ts`), terminated by a
 * `message_updated` frame carrying the finalized message
 * (`status: "completed"`/`"failed"`) — this hook's `onSuccess` only ever
 * needs to reconcile the *optimistic* entries against the placeholder, not
 * against final content.
 *
 * On success both optimistic entries are replaced by the real persisted
 * messages — unless either message's own WS `message_created` echo (or, for
 * the AI message, its first `token_chunk`) already won the race and merged
 * the server copy into the cache first (routinely happens locally, since a
 * WS frame can beat the POST response), in which case that optimistic entry
 * is dropped instead of being swapped in too (checked independently per
 * message, since the human and AI messages' WS frames can each arrive on
 * their own schedule) — otherwise the swap would leave two copies of the
 * same server message (see `mergeMessageEvent`'s dedup-by-id, which cannot
 * recognize an optimistic entry as "the same message" since it has a
 * different, client-generated id) or, worse, clobber content already
 * accumulated from `token_chunk` deltas with the placeholder's empty
 * `content`. Note that a *successful* HTTP response can still carry
 * `ai_message.status === "failed"` — `SendAIMessageStream` immediately marks
 * the placeholder failed (never streaming) when the LLM Gateway rejects the
 * request synchronously (see
 * `server/internal/usecase/message/stream.go`) so `RegenerateAIMessage` can
 * retry it later. That is handled here like any other `onSuccess`
 * reconciliation, not this hook's `onError` path.
 *
 * `onError` (a genuine request failure: network error, non-2xx from the
 * endpoint itself, meaning *neither* message was persisted) rolls the human
 * echo back to `status: "failed"` for `MessageBubble`'s retry affordance and
 * drops the AI placeholder outright — it never represented anything real to
 * retry, and the existing AI regenerate/retry control only makes sense
 * against a real, persisted human message id. It also records the failed
 * send's original `model`/`stream`/`private`/`attachmentIds` parameters into
 * the `QueryClient`-backed retry-intent map (see
 * `failedAISendIntentQueryKey`'s own doc comment), keyed by
 * `context.humanOptimisticId` -- the same id the human echo's `status:
 * "failed"` entry keeps in the cache, and so the same id `MessageBubble`'s
 * retry affordance passes back as `messageId`. `useChatRoom.handleRetry`
 * consults this map (via `takeFailedAISendIntent`) to route a retry through
 * this same AI mutation (with the original model/stream/private) instead of
 * falling back to the plain-send mutation, which would otherwise silently
 * downgrade every AI-send retry into a plain send -- including dropping
 * `private`, which would leak a failed private send's retry to the whole
 * room.
 *
 * @param roomId - The room to send into.
 */
export function useSendAIMessage(roomId: string) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const

  return useMutation({
    // Private-mode sends cannot use the streaming endpoint (`StreamAI`
    // rejects `private: true` with HTTP 400), and callers may opt out of
    // streaming explicitly (`stream: false`, see `SendAIMessageInput`) --
    // both route through the non-streaming `sendAIMessage`; everything else
    // (the default) goes through `sendAIMessageStream`.
    mutationFn: ({ content, model, private: isPrivate, stream = true }: SendAIMessageInput) =>
      isPrivate || !stream
        ? sendAIMessage(roomId, content, model, isPrivate ?? false)
        : sendAIMessageStream(roomId, content, model),
    onMutate: async ({ content, private: isPrivate }): Promise<SendAIMessageContext> => {
      await queryClient.cancelQueries({ queryKey })

      const humanOptimisticId = `optimistic-human-${crypto.randomUUID()}`
      const aiOptimisticId = `optimistic-ai-${crypto.randomUUID()}`
      const now = new Date().toISOString()
      // Tag both optimistic entries with the visibility the user selected,
      // so `MessageBubble`'s private badge/border render immediately, before
      // the WS/REST-confirmed message reconciles over them (see
      // `mergeMessageEvent`).
      const visibility = isPrivate ? "private" : "public"

      const optimisticHuman: Message = {
        id: humanOptimisticId,
        room_id: roomId,
        sender_id: null,
        content,
        type: "human",
        status: "sending",
        sequence: -1,
        in_response_to_message_id: null,
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: false,
        visibility,
        created_at: now,
        updated_at: now,
      }
      const optimisticAI: Message = {
        id: aiOptimisticId,
        room_id: roomId,
        sender_id: null,
        content: "",
        type: "ai",
        status: "sending",
        sequence: -1,
        in_response_to_message_id: humanOptimisticId,
        is_deleted: false,
        exclude_from_ai: false,
        used_context_summary: false,
        visibility,
        created_at: now,
        updated_at: now,
      }

      // Prepend the human echo first, then the AI placeholder on top of it —
      // since a page's `messages` are newest-first, the last-prepended entry
      // (the AI placeholder) ends up as the newest, which flattens back to
      // "human, then AI" in display (oldest-to-newest) order.
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        prependToNewestPage(prependToNewestPage(old, optimisticHuman), optimisticAI),
      )

      return { humanOptimisticId, aiOptimisticId }
    },
    onSuccess: (res, _vars, context) => {
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) => {
        const withHuman =
          findMessageInPages(old, res.user_message.id) !== undefined
            ? removeFromNewestPage(old, context.humanOptimisticId)
            : replaceInNewestPage(
                old,
                (m) => m.id === context.humanOptimisticId,
                res.user_message,
              )
        return findMessageInPages(withHuman, res.ai_message.id) !== undefined
          ? removeFromNewestPage(withHuman, context.aiOptimisticId)
          : replaceInNewestPage(
              withHuman,
              (m) => m.id === context.aiOptimisticId,
              res.ai_message,
            )
      })
    },
    onError: (error, vars, context) => {
      if (!context) return

      // Recorded regardless of error kind so a retry of *any* failed AI
      // send routes back through this same AI mutation with the original
      // model/stream/private, not just retries that hit a generic failure.
      const intentKey = failedAISendIntentQueryKey(roomId)
      const currentIntents = queryClient.getQueryData<Map<string, FailedAISendIntent>>(intentKey)
      const nextIntents = new Map(currentIntents)
      nextIntents.set(context.humanOptimisticId, {
        model: vars.model,
        stream: vars.private ? false : (vars.stream ?? true),
        private: vars.private ?? false,
        attachmentIds: vars.attachmentIds,
      })
      queryClient.setQueryData(intentKey, nextIntents)

      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) => {
        const withoutPlaceholder = removeFromNewestPage(old, context.aiOptimisticId)
        return markStatusInNewestPage(withoutPlaceholder, context.humanOptimisticId, "failed")
      })

      toaster.create({
        type: "error",
        title: "Message failed to send",
        description:
          error instanceof Error ? error.message : "Please try again.",
      })
    },
  })
}
