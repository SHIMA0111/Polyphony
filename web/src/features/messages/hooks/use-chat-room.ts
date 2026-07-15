"use client"

import { useCallback } from "react"
import { useRoom } from "@/features/rooms/hooks/use-room"
import { useMessages } from "@/features/messages/hooks/use-messages"
import { useModels } from "@/features/messages/hooks/use-models"
import { useSendMessage } from "@/features/messages/hooks/use-send-message"
import { useSendAIMessage } from "@/features/messages/hooks/use-send-ai-message"
import { useRegenerateAIMessage } from "@/features/messages/hooks/use-regenerate-ai-message"
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
  handleSend: (content: string) => Promise<void>
  handleSendWithAI: (content: string, model: string) => Promise<void>
  handleRegenerate: (aiMessageId: string) => Promise<void>
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
  const roomQuery = useRoom(roomId)
  const messagesQuery = useMessages(roomId)
  const modelsQuery = useModels()

  const sendMessageMutation = useSendMessage(roomId)
  const sendAIMessageMutation = useSendAIMessage(roomId)
  const regenerateMutation = useRegenerateAIMessage(roomId)

  const room = roomQuery.data
  const messages = messagesQuery.data ?? EMPTY_MESSAGES
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
      // Find the human message that precedes this AI message
      const msgIndex = messages.findIndex((m) => m.id === aiMessageId)
      if (msgIndex < 0) return

      // Find the preceding human message
      let humanMessageId: string | null = null
      for (let i = msgIndex - 1; i >= 0; i--) {
        if (messages[i].type === "human") {
          humanMessageId = messages[i].id
          break
        }
      }
      if (!humanMessageId) return

      try {
        await regenerateMutation.mutateAsync({ aiMessageId, humanMessageId })
      } catch {
        // TODO: handle error
      }
    },
    [messages, regenerateMutation],
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
    handleSend,
    handleSendWithAI,
    handleRegenerate,
  }
}
