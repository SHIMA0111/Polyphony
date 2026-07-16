"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { sendMessage } from "../api/send-message"
import type { Message } from "../types"

/**
 * Sends a plain (non-AI) message and appends it to the cached
 * `["rooms", roomId, "messages"]` array on success, preserving the
 * append-only UX `ChatRoom.tsx` had before this migration (`setMessages((prev)
 * => [...prev, msg])`) — a full invalidate would drop the optimistic-feeling
 * instant append in favor of a round-trip refetch.
 */
export function useSendMessage(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (content: string) => sendMessage(roomId, content),
    onSuccess: (message) => {
      queryClient.setQueryData<Message[]>(
        ["rooms", roomId, "messages"],
        (old = []) => [...old, message],
      )
    },
  })
}
