"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { sendMessage } from "../api/send-message"
import { toaster } from "@/components/ui/toaster"
import {
  markStatusInNewestPage,
  prependToNewestPage,
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
 * position). `onError` rolls the optimistic entry back to `status: "failed"`
 * — kept visible, never removed, so `MessageBubble` can offer a retry
 * affordance — and surfaces a failure toast.
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
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        replaceInNewestPage(old, (m) => m.id === context.optimisticId, message),
      )
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
