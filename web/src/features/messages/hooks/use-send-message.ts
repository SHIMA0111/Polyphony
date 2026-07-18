"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { sendMessage } from "../api/send-message"
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

/** Context carried from `onMutate` through to `onSuccess`/`onError`. */
interface SendMessageContext {
  optimisticId: string
}

/**
 * Sends a plain (non-AI) message with an optimistic, instant echo.
 *
 * `onMutate` cancels any in-flight refetch of `["rooms", roomId, "messages"]`
 * and immediately prepends a client-generated `status: "sending"` entry to
 * the cache's newest page, so `MessageList` renders the message before the
 * network round-trip completes at all. `onSuccess` swaps that entry for the
 * real, server-returned message (matched by the optimistic id, not array
 * position) — unless the message's own WS `message_created` echo already won
 * the race and merged the server copy into the cache first (routinely
 * happens locally, since the WS frame can beat the POST response), in which
 * case the optimistic entry is simply dropped instead of being swapped in
 * too, which would otherwise leave two copies of the same server message
 * (see `mergeMessageEvent`'s dedup-by-id, which cannot recognize the
 * optimistic entry as "the same message" since it has a different, client-
 * generated id). `onError` rolls the optimistic entry back to `status:
 * "failed"` — kept visible, never removed, so `MessageBubble` can offer a
 * retry affordance — and surfaces a failure toast.
 */
export function useSendMessage(roomId: string) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const

  return useMutation({
    mutationFn: (content: string) => sendMessage(roomId, content),
    onMutate: async (content): Promise<SendMessageContext> => {
      await queryClient.cancelQueries({ queryKey })

      const optimisticId = `optimistic-${crypto.randomUUID()}`
      const now = new Date().toISOString()
      const optimisticMessage: Message = {
        id: optimisticId,
        room_id: roomId,
        sender_id: null,
        content,
        type: "human",
        status: "sending",
        sequence: -1,
        in_response_to_message_id: null,
        is_deleted: false,
        exclude_from_ai: false,
        created_at: now,
        updated_at: now,
      }

      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        prependToNewestPage(old, optimisticMessage),
      )

      return { optimisticId }
    },
    onSuccess: (message, _content, context) => {
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) => {
        if (findMessageInPages(old, message.id) !== undefined) {
          return removeFromNewestPage(old, context.optimisticId)
        }
        return replaceInNewestPage(old, (m) => m.id === context.optimisticId, message)
      })
    },
    onError: (error, _content, context) => {
      if (!context) return

      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        markStatusInNewestPage(old, context.optimisticId, "failed"),
      )

      toaster.create({
        type: "error",
        title: "Message failed to send",
        description:
          error instanceof Error ? error.message : "Please try again.",
      })
    },
  })
}
