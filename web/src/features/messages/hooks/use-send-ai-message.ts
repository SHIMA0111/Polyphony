"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { sendAIMessage } from "../api/send-ai-message"
import type { Message } from "../types"

export interface SendAIMessageInput {
  content: string
  model?: string
}

/**
 * Sends a message with an AI response and appends both the user message and
 * the AI message to the cached `["rooms", roomId, "messages"]` array on
 * success, matching `ChatRoom.tsx`'s pre-migration behavior
 * (`setMessages((prev) => [...prev, res.user_message, res.ai_message])`).
 */
export function useSendAIMessage(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ content, model }: SendAIMessageInput) =>
      sendAIMessage(roomId, content, model),
    onSuccess: (res) => {
      queryClient.setQueryData<Message[]>(
        ["rooms", roomId, "messages"],
        (old = []) => [...old, res.user_message, res.ai_message],
      )
    },
  })
}
