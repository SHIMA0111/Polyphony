"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { sendAIMessage } from "../api/send-ai-message"
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

export interface SendAIMessageInput {
  content: string
  model?: string
}

/** Context carried from `onMutate` through to `onSuccess`/`onError`. */
interface SendAIMessageContext {
  humanOptimisticId: string
  aiOptimisticId: string
}

export interface UseSendAIMessageOptions {
  /**
   * Invoked from `onError` with the failed human echo's id and the model
   * that was requested, so a caller (see `useChatRoom`'s `handleRetry`) can
   * remember that this particular failed message's retry should re-invoke
   * the AI mutation with the same model, rather than silently falling back
   * to a plain (non-AI) resend and losing the user's original intent.
   */
  onSendFailed?: (humanMessageId: string, model?: string) => void
}

/**
 * Sends a message with an AI response, optimistically appending both a
 * `status: "sending"` human echo and a `status: "sending"` AI placeholder
 * (rendered by `MessageBubble` as a `ThinkingBubble`) the instant the
 * mutation is invoked — no wait for the round trip.
 *
 * On success both optimistic entries are replaced by the real persisted
 * messages — unless either message's own WS `message_created` echo already
 * won the race and merged the server copy into the cache first (routinely
 * happens locally, since a WS frame can beat the POST response), in which
 * case that optimistic entry is dropped instead of being swapped in too
 * (checked independently per message, since the human and AI messages' WS
 * frames can each arrive on their own schedule) — otherwise the swap would
 * leave two copies of the same server message (see `mergeMessageEvent`'s
 * dedup-by-id, which cannot recognize an optimistic entry as "the same
 * message" since it has a different, client-generated id). Note that a
 * *successful* HTTP response can still carry
 * `ai_message.status === "failed"` — `SendAIMessage` always persists a
 * human message plus an AI message row, saving `status: "failed"` on the AI
 * row when the LLM call itself failed (see
 * `server/internal/usecase/message/usecase.go`) so `RegenerateAIMessage` can
 * retry it later. That is handled here like any other `onSuccess`
 * reconciliation, not this hook's `onError` path.
 *
 * `onError` (a genuine request failure: network error, non-2xx from the
 * endpoint itself, meaning *neither* message was persisted) rolls the human
 * echo back to `status: "failed"` for `MessageBubble`'s retry affordance and
 * drops the AI placeholder outright — it never represented anything real to
 * retry, and the existing AI regenerate/retry control only makes sense
 * against a real, persisted human message id. It also invokes
 * `options.onSendFailed` (see {@link UseSendAIMessageOptions}) with the
 * failed human echo's id and the requested model, so a caller can route a
 * later retry of that specific message back through this same AI mutation
 * instead of a plain resend.
 *
 * @param roomId - The room to send into.
 * @param options - See {@link UseSendAIMessageOptions}.
 */
export function useSendAIMessage(roomId: string, options?: UseSendAIMessageOptions) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const

  return useMutation({
    mutationFn: ({ content, model }: SendAIMessageInput) =>
      sendAIMessage(roomId, content, model),
    onMutate: async ({ content }): Promise<SendAIMessageContext> => {
      await queryClient.cancelQueries({ queryKey })

      const humanOptimisticId = `optimistic-human-${crypto.randomUUID()}`
      const aiOptimisticId = `optimistic-ai-${crypto.randomUUID()}`
      const now = new Date().toISOString()

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

      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) => {
        const withoutPlaceholder = removeFromNewestPage(old, context.aiOptimisticId)
        return markStatusInNewestPage(withoutPlaceholder, context.humanOptimisticId, "failed")
      })

      options?.onSendFailed?.(context.humanOptimisticId, vars.model)

      toaster.create({
        type: "error",
        title: "Message failed to send",
        description:
          error instanceof Error ? error.message : "Please try again.",
      })
    },
  })
}
