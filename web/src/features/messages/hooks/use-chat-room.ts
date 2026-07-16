"use client"

import { useCallback, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { ApiRequestError } from "@/lib/http-client"
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

/**
 * Copy shown by `MessageInput`'s inline error line when an AI send is
 * rejected with HTTP 402 (`domain.ErrInsufficientBalance`, see
 * `server/internal/usecase/billing/usecase.go`'s `CheckBalance`).
 */
export const INSUFFICIENT_BALANCE_MESSAGE = "Insufficient token balance."

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
  /**
   * Set to {@link INSUFFICIENT_BALANCE_MESSAGE} when the most recent
   * `handleSendWithAI` call was rejected with HTTP 402, `null` otherwise
   * (including after any other kind of send failure, which the mutation's
   * own toast already surfaces). Cleared at the start of every subsequent
   * `handleSendWithAI` call so a resolved-then-retried send doesn't leave a
   * stale error on screen.
   */
  aiError: string | null
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
  const [aiError, setAiError] = useState<string | null>(null)

  const roomQuery = useRoom(roomId)
  const messagesQuery = useMessages(roomId)
  const modelsQuery = useModels()

  const sendMessageMutation = useSendMessage(roomId)
  const sendAIMessageMutation = useSendAIMessage(roomId)
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
      setAiError(null)
      try {
        await sendAIMessageMutation.mutateAsync({ content, model })
        // Refresh the top-bar balance promptly after a successful AI send,
        // rather than waiting for `useBalance`'s background poll — a send
        // debits the room owner's balance server-side (see
        // `BillingUsecase.RecordUsage`).
        await queryClient.invalidateQueries({ queryKey: ["billing", "balance"] })
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 402) {
          // Distinguish "the AI declined to answer" from "the request was
          // never even allowed to run": surface a specific, actionable error
          // to `MessageInput` instead of the generic failed-send toast
          // `useSendAIMessage`'s own `onError` already shows.
          setAiError(INSUFFICIENT_BALANCE_MESSAGE)
        }
        // Re-throw regardless of status so `MessageInput`'s own catch still
        // restores the typed content and `useSendAIMessage`'s `onError`
        // still rolls back the optimistic entries — this only adds the
        // 402-specific `aiError` state on top of that existing handling.
        throw error
      }
    },
    [sendAIMessageMutation, queryClient],
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
      // Drop the stale failed optimistic entry first so `useSendMessage`'s
      // own `onMutate` (which appends a *new* optimistic entry with a fresh
      // id) doesn't leave both the old failed bubble and the new "sending"
      // bubble on screen at once.
      queryClient.setQueryData<MessagesInfiniteData>(
        ["rooms", roomId, "messages"],
        (old) => removeFromNewestPage(old, messageId),
      )
      await sendMessageMutation.mutateAsync(content)
    },
    [queryClient, roomId, sendMessageMutation],
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
    aiError,
  }
}
