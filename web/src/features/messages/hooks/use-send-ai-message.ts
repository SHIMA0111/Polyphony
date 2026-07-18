"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useRef } from "react"
import { sendAIMessage, sendAIMessageStream } from "../api/send-ai-message"
import { ApiRequestError } from "@/lib/http-client"
import { getErrorMessage } from "@/lib/get-error-message"
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
 * The original send parameters an AI send needs to be retried faithfully —
 * captured in `useSendAIMessage`'s `onError` (keyed by the failed human
 * message's optimistic id) and consulted by `useChatRoom.handleRetry` to
 * decide whether a retry should go through `useSendAIMessage` (with these
 * exact parameters) instead of the plain `useSendMessage` mutation.
 */
export interface FailedAISendIntent {
  model?: string
  stream: boolean
  /**
   * Whether the failed send was a private AI send. Restored verbatim on
   * retry (see `useChatRoom.handleRetry`) so a retried private send stays
   * private instead of silently downgrading to a public one -- the same
   * leak the pre-retry-intent code always had, now closed for the one
   * parameter this map previously dropped.
   */
  private: boolean
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
 * against a real, persisted human message id. The optimistic-rollback part
 * of `onError` always runs, but the generic failed-to-send toast is
 * suppressed for a `402` (`ApiRequestError.status === 402`,
 * `domain.ErrInsufficientBalance`): `useChatRoom.handleSendWithAI` already
 * surfaces that specific rejection via its own inline `aiError` alert
 * (`INSUFFICIENT_BALANCE_MESSAGE`), and showing both at once (M1 post-review
 * finding) was a confusing double-toast for the exact same failure.
 *
 * `onError` also records the failed send's original `model`/`stream`/
 * `private` parameters into `failedIntentsRef.current`, keyed by
 * `context.humanOptimisticId` -- the same id the human echo's `status:
 * "failed"` entry keeps in the cache, and so the same id `MessageBubble`'s
 * retry affordance passes back as `messageId`. `useChatRoom.handleRetry`
 * consults this map (via the returned ref, not a dereferenced value -- see
 * `failedIntentsRef`'s own doc comment) to route a retry through this same
 * AI mutation (with the original model/stream/private) instead of always
 * falling back to the plain-send mutation, which previously silently
 * downgraded every AI-send retry into a plain send -- including dropping
 * `private`, which would have leaked a failed private send's retry to the
 * whole room. Entries are removed once consumed by a retry (successful or
 * not) to avoid unbounded growth over a long session; a retry that itself
 * fails repopulates the map under the *new* optimistic id its own `onError`
 * call produces.
 */
export function useSendAIMessage(roomId: string) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const
  // Returned as-is (never dereferenced here) so callers only ever read/write
  // `.current` from their own event-handler-time code, not during this
  // hook's render -- see the `return` statement's comment below.
  const failedIntentsRef = useRef(new Map<string, FailedAISendIntent>())

  const mutation = useMutation({
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

      // Recorded regardless of error kind (including a 402 below) so a
      // retry of *any* failed AI send -- insufficient balance included --
      // still routes back through this same AI mutation with the original
      // model/stream, not just retries that hit a generic failure.
      failedIntentsRef.current.set(context.humanOptimisticId, {
        model: vars.model,
        stream: vars.private ? false : (vars.stream ?? true),
        private: vars.private ?? false,
      })

      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) => {
        const withoutPlaceholder = removeFromNewestPage(old, context.aiOptimisticId)
        return markStatusInNewestPage(withoutPlaceholder, context.humanOptimisticId, "failed")
      })

      if (error instanceof ApiRequestError && error.status === 402) {
        // Insufficient balance is surfaced via `useChatRoom`'s inline
        // `aiError` alert instead -- see this function's docstring.
        return
      }

      toaster.create({
        type: "error",
        title: "Message failed to send",
        description: getErrorMessage(error, "Please try again."),
      })
    },
  })

  // Returns the ref itself, not `failedIntentsRef.current`: dereferencing
  // `.current` here (during this hook's own render) trips
  // `react-hooks/refs` ("refs should only be accessed outside of render").
  // `useChatRoom.handleRetry` reads/writes `.current` from inside its own
  // event-handler-time callback instead, which the rule permits.
  return { ...mutation, failedIntentsRef }
}
