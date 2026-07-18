"use client"

import { useCallback, useMemo, useRef } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { useRoom } from "@/features/rooms/hooks/use-room"
import { useMessages } from "@/features/messages/hooks/use-messages"
import { useModels } from "@/features/messages/hooks/use-models"
import { useSendMessage } from "@/features/messages/hooks/use-send-message"
import { useSendAIMessage } from "@/features/messages/hooks/use-send-ai-message"
import { useRegenerateAIMessage } from "@/features/messages/hooks/use-regenerate-ai-message"
import { flattenMessagePages } from "@/features/messages/lib/flatten-message-pages"
import { removeFromNewestPage, type MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import type { Message, ModelInfo } from "@/features/messages/types"
import type { Room } from "@/features/rooms/types"

/** Stable identity fallbacks so `useCallback`/`useMemo` deps below don't
 * change on every render while a query has no data yet. */
const EMPTY_MESSAGES: Message[] = []
const EMPTY_MODELS: ModelInfo[] = []

export interface UseChatRoomResult {
  room: Room | undefined
  messages: Message[]
  models: ModelInfo[]
  isLoading: boolean
  /** The AI message id currently being regenerated, or `null`. */
  isRegenerating: string | null
  /** Whether an older page of history is available via `fetchNextPage`. */
  hasNextPage: boolean
  /** Whether the next (older) page is currently being fetched. */
  isFetchingNextPage: boolean
  /** Fetches the next older page of message history. */
  fetchNextPage: () => Promise<unknown>
  /** Number of currently loaded pages; used to anchor scroll position across a load. */
  pageCount: number
  handleSend: (content: string) => Promise<void>
  handleSendWithAI: (content: string, model: string) => Promise<void>
  handleRegenerate: (aiMessageId: string) => Promise<void>
  /** Re-sends a failed human message's original content, replacing its
   * failed optimistic entry so no duplicate bubble is left behind. */
  handleRetry: (messageId: string, content: string) => Promise<void>
}

/**
 * Data-orchestration hook for the chat room page: composes step 9's
 * per-feature TanStack Query hooks (room, messages, models) with the
 * send/send-with-AI/regenerate mutations and derives the loading and
 * regenerating state `ChatRoom` needs to render.
 *
 * Kept separate from `ChatRoom.tsx` so the component itself stays a thin
 * composition of `ChatRoomHeader` + `MessageList` + `MessageInput`.
 *
 * @param roomId - The room to load and interact with.
 */
export function useChatRoom(roomId: string): UseChatRoomResult {
  const queryClient = useQueryClient()

  const roomQuery = useRoom(roomId)
  const messagesQuery = useMessages(roomId)
  const modelsQuery = useModels()

  const sendMessageMutation = useSendMessage(roomId)

  // Tracks the send intent of each *currently failed* message that
  // originated from an AI send, keyed by the failed human echo's id (the
  // same id `MessageBubble` passes back to `handleRetry` below): populated
  // by `useSendAIMessage`'s `onSendFailed` callback whenever an AI send
  // fails, and consulted (then cleared) by `handleRetry` to decide whether
  // a retry must re-invoke the AI mutation with the original model instead
  // of silently falling back to a plain resend. A plain `useRef` (not
  // state) is enough: nothing needs to re-render when this map changes, it
  // only needs to be read/written imperatively from `handleRetry`.
  const retryIntentRef = useRef(new Map<string, { model?: string }>())
  const sendAIMessageMutation = useSendAIMessage(roomId, {
    onSendFailed: (humanMessageId, model) => {
      retryIntentRef.current.set(humanMessageId, { model })
    },
  })
  const regenerateMutation = useRegenerateAIMessage(roomId)

  const room = roomQuery.data
  const pages = messagesQuery.data?.pages
  const messages = useMemo(
    () => (pages ? flattenMessagePages(pages) : EMPTY_MESSAGES),
    [pages],
  )
  const models = modelsQuery.data ?? EMPTY_MODELS
  const isLoading =
    roomQuery.isPending || messagesQuery.isPending || modelsQuery.isPending

  const handleSend = useCallback(
    async (content: string) => {
      await sendMessageMutation.mutateAsync(content)
    },
    [sendMessageMutation],
  )

  const handleSendWithAI = useCallback(
    async (content: string, model: string) => {
      await sendAIMessageMutation.mutateAsync({ content, model })
    },
    [sendAIMessageMutation],
  )

  const handleRegenerate = useCallback(
    async (aiMessageId: string) => {
      // Resolve the target human message directly from the AI message's
      // `in_response_to_message_id` link (a Step 7 server addition) instead
      // of scanning `messages` backwards by array position — the previous
      // approach could resolve the wrong human message whenever the AI
      // message wasn't immediately preceded by its own human message (e.g.
      // after an interleaved system/failed entry).
      const aiMessage = messages.find((m) => m.id === aiMessageId)
      if (!aiMessage?.in_response_to_message_id) {
        // Should not happen for an AI message created via `SendAIMessage`
        // (see `server/internal/usecase/message/usecase.go`); no-op rather
        // than guess at a fallback target.
        return
      }

      try {
        await regenerateMutation.mutateAsync({
          aiMessageId,
          humanMessageId: aiMessage.in_response_to_message_id,
        })
      } catch {
        // The mutation's rejection is enough for callers that want to
        // observe it (e.g. via `regenerateMutation.isError`); `MessageBubble`
        // already reflects a persisted `status: "failed"` AI message via its
        // own styling, so there is nothing further to do here.
      }
    },
    [messages, regenerateMutation],
  )

  const handleRetry = useCallback(
    async (messageId: string, content: string) => {
      // A failed message that originated from an AI send has an entry here
      // (see `retryIntentRef`'s docstring above); anything else (a plain
      // send's failure) has none, and falls back to a plain resend below —
      // its original intent already *was* plain, so there is nothing to
      // recover.
      const intent = retryIntentRef.current.get(messageId)
      retryIntentRef.current.delete(messageId)

      // Drop the stale failed optimistic entry first so the mutation's own
      // `onMutate` (which appends a *new* optimistic entry with a fresh id)
      // doesn't leave both the old failed bubble and the new "sending"
      // bubble on screen at once.
      queryClient.setQueryData<MessagesInfiniteData>(
        ["rooms", roomId, "messages"],
        (old) => removeFromNewestPage(old, messageId),
      )

      try {
        if (intent) {
          await sendAIMessageMutation.mutateAsync({ content, model: intent.model })
        } else {
          await sendMessageMutation.mutateAsync(content)
        }
      } catch {
        // The mutation's own `onError` already reflects the failure (a new
        // `status: "failed"` entry, plus a toast) and — for the AI path —
        // re-populates `retryIntentRef` for the newly-failed message id via
        // `onSendFailed`; there is nothing further to do here, mirroring
        // `handleRegenerate`'s identical catch-and-ignore above.
      }
    },
    [queryClient, roomId, sendMessageMutation, sendAIMessageMutation],
  )

  const isRegenerating = regenerateMutation.isPending
    ? (regenerateMutation.variables?.aiMessageId ?? null)
    : null

  return {
    room,
    messages,
    models,
    isLoading,
    isRegenerating,
    hasNextPage: messagesQuery.hasNextPage,
    isFetchingNextPage: messagesQuery.isFetchingNextPage,
    fetchNextPage: messagesQuery.fetchNextPage,
    pageCount: pages?.length ?? 0,
    handleSend,
    handleSendWithAI,
    handleRegenerate,
    handleRetry,
  }
}
