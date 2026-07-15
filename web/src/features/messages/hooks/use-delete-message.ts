"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { deleteMessage } from "../api/delete-message"
import { toaster } from "@/components/ui/toaster"
import { removeMessageInAnyPage, type MessagesInfiniteData } from "../lib/message-cache"

/**
 * Soft-deletes a message (`DELETE /rooms/:roomId/messages/:messageId`) and,
 * on success, drops it from every already-loaded page of the cached
 * `["rooms", roomId, "messages"]` query immediately — the server already
 * excludes soft-deleted rows from future `GET` responses, so this mirrors
 * that outcome client-side without waiting for a refetch (per Step 38's
 * scope).
 *
 * Surfaces a failure toast on error rather than optimistically removing the
 * message up front: unlike sends, there is no "instant echo" affordance to
 * preserve here, so it is simpler and safer to only remove the message once
 * the server has actually confirmed the deletion.
 */
export function useDeleteMessage(roomId: string) {
  const queryClient = useQueryClient()
  const queryKey = ["rooms", roomId, "messages"] as const

  return useMutation({
    mutationFn: (messageId: string) => deleteMessage(roomId, messageId),
    onSuccess: (_data, messageId) => {
      queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
        removeMessageInAnyPage(old, messageId),
      )
    },
    onError: (error) => {
      toaster.create({
        type: "error",
        title: "Failed to delete message",
        description:
          error instanceof Error ? error.message : "Please try again.",
      })
    },
  })
}
