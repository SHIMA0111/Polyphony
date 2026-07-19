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
 *
 * `onMutate` cancels any in-flight refetch of `["rooms", roomId,
 * "messages"]`, mirroring `useSendMessage`/`useSendAIMessage`'s sibling
 * hooks: without it, a background refetch that was already in flight when
 * regenerate was triggered could resolve *after* `onSuccess`'s
 * `setQueryData` call above and clobber the just-regenerated message with
 * the stale (pre-regeneration) data that refetch fetched.
 */
export function useRegenerateAIMessage(roomId: string) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const

  return useMutation({
    mutationFn: ({ humanMessageId, model }: RegenerateAIMessageInput) =>
      regenerateAIMessage(roomId, humanMessageId, model),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey })
    },
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
