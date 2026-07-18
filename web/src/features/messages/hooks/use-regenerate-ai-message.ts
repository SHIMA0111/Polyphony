"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { regenerateAIMessage } from "../api/regenerate-ai-message"
import { replaceMessageInAnyPage, type MessagesInfiniteData } from "../lib/message-cache"
import { ApiRequestError } from "@/lib/http-client"
import { getErrorMessage } from "@/lib/get-error-message"
import { toaster } from "@/components/ui/toaster"

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
 * Error surfacing (M1 post-review finding; previously this mutation had no
 * `onError` at all, and `useChatRoom.handleRegenerate` swallowed every
 * rejection, making a 402/502 regenerate failure a silent no-op): a `402`
 * (`ApiRequestError.status === 402`, `domain.ErrInsufficientBalance`) is left
 * for the caller to surface via its own inline `aiError` alert (mirroring
 * `useSendAIMessage`'s same suppression, see that hook's docstring), since
 * this hook has no component state to render an inline alert into; every
 * other error shows a generic toast here, consistent with the send path.
 *
 * A successful regenerate also invalidates `["billing", "balance"]`: like a
 * send, it debits the room owner's balance server-side
 * (`BillingUsecase.RecordUsage`), so the top-bar balance should refresh
 * promptly rather than waiting for `useBalance`'s background poll (parity
 * with `useChatRoom.handleSendWithAI`'s own invalidation after a send).
 *
 * `onMutate` cancels any in-flight refetch of `["rooms", roomId, "messages"]`
 * (mirroring `useSendMessage`/`useSendAIMessage`'s same first step), so a
 * racing background refetch that resolves between this mutation starting
 * and its own `onSuccess` running can't clobber the `setQueryData` swap
 * below with stale (pre-regenerate) server data.
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
      void queryClient.invalidateQueries({ queryKey: ["billing", "balance"] })
    },
    onError: (error) => {
      if (error instanceof ApiRequestError && error.status === 402) {
        return
      }
      toaster.create({
        type: "error",
        title: "Failed to regenerate response",
        description: getErrorMessage(error, "Please try again."),
      })
    },
  })
}
