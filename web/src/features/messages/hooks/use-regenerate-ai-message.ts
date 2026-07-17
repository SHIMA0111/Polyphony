"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { regenerateAIMessage } from "../api/regenerate-ai-message"
import { replaceMessageInAnyPage, type MessagesInfiniteData } from "../lib/message-cache"

export interface RegenerateAIMessageInput {
  /** ID of the AI message being regenerated (replaced in the cache on success). */
  aiMessageId: string
  /** ID of the preceding human message the regenerate endpoint is keyed on. */
  humanMessageId: string
  model?: string
}

/**
 * Regenerates an AI response and replaces the matching message (by
 * `aiMessageId`) in the cached `["rooms", roomId, "messages"]` array,
 * matching `ChatRoom.tsx`'s pre-migration `handleRegenerate` behavior
 * (`setMessages((prev) => prev.map((m) => (m.id === messageId ? updated : m)))`).
 *
 * The mutation's `variables` intentionally carry both ids: the Go API's
 * regenerate endpoint is keyed on the *preceding human message's* id
 * (`humanMessageId`), while the cache replacement targets the *AI message*
 * being regenerated (`aiMessageId`) — the same two-id distinction the
 * pre-migration component logic already made when it walked `messages` to
 * find the human message preceding the clicked AI message.
 */
export function useRegenerateAIMessage(roomId: string) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const

  return useMutation({
    mutationFn: ({ humanMessageId, model }: RegenerateAIMessageInput) =>
      regenerateAIMessage(roomId, humanMessageId, model),
    onSuccess: (updated, variables) => {
      // Unlike optimistic sends (always in `pages[0]`), the AI message being
      // regenerated can live in any already-loaded page once the reader has
      // scrolled up through history, so this searches every page rather
      // than assuming the newest one.
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        replaceMessageInAnyPage(old, (m) => m.id === variables.aiMessageId, updated),
      )
    },
  })
}
