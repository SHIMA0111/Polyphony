"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { updateMessageExclude } from "../api/update-message-exclude"
import { toaster } from "@/components/ui/toaster"
import { replaceMessageInAnyPage, type MessagesInfiniteData } from "../lib/message-cache"

/** Mutation variables for {@link useUpdateMessageExclude}. */
export interface UpdateMessageExcludeInput {
  messageId: string
  exclude: boolean
}

/**
 * Toggles a message's `exclude_from_ai` flag (`PATCH
 * /rooms/:roomId/messages/:messageId`, Step 23's `SetExcludeFromAI`
 * endpoint) and, on success, patches the returned message into whichever
 * already-loaded page currently holds it — no full refetch needed, matching
 * `useRegenerateAIMessage`'s existing any-page reconciliation pattern.
 */
export function useUpdateMessageExclude(roomId: string) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const

  return useMutation({
    mutationFn: ({ messageId, exclude }: UpdateMessageExcludeInput) =>
      updateMessageExclude(roomId, messageId, exclude),
    onSuccess: (updated) => {
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        replaceMessageInAnyPage(old, (m) => m.id === updated.id, updated),
      )
    },
    onError: (error) => {
      toaster.create({
        type: "error",
        title: "Failed to update message",
        description:
          error instanceof Error ? error.message : "Please try again.",
      })
    },
  })
}
