"use client"

import { useCallback } from "react"
import Link from "next/link"
import { Box, Button, Flex, Heading, Spinner } from "@chakra-ui/react"
import { ChevronLeft, MoreVertical } from "lucide-react"
import { useRoom } from "@/features/rooms/hooks/use-room"
import { useMessages } from "@/features/messages/hooks/use-messages"
import { useModels } from "@/features/messages/hooks/use-models"
import { useSendMessage } from "@/features/messages/hooks/use-send-message"
import { useSendAIMessage } from "@/features/messages/hooks/use-send-ai-message"
import { useRegenerateAIMessage } from "@/features/messages/hooks/use-regenerate-ai-message"
import { MessageList } from "./MessageList"
import { MessageInput } from "./MessageInput"
import type { Message, ModelInfo } from "@/features/messages/types"

interface ChatRoomProps {
  roomId: string
}

/** Stable identity fallbacks so `useCallback`/`useMemo` deps below don't
 * change on every render while a query has no data yet. */
const EMPTY_MESSAGES: Message[] = []
const EMPTY_MODELS: ModelInfo[] = []

export function ChatRoom({ roomId }: ChatRoomProps) {
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

  if (isLoading) {
    return (
      <Flex h="100vh" align="center" justify="center">
        <Spinner size="xl" colorPalette="blue" />
      </Flex>
    )
  }

  return (
    <Flex h="100vh" direction="column" bg="bg">
      {/* Header */}
      <Flex
        as="header"
        borderBottomWidth="1px"
        bg="bg/80"
        backdropFilter="blur(8px)"
        align="center"
        gap={3}
        px={4}
        h={16}
        flexShrink={0}
      >
        {/* The persistent `(main)` layout's rail already provides room
            navigation at `md`+, so this mobile-only back control (which
            returns to the rail-as-list screen at `/rooms`) is hidden there. */}
        <Box display={{ base: "flex", md: "none" }}>
          <Link href="/rooms">
            <Button variant="ghost" size="sm" p={0}>
              <ChevronLeft size={20} />
            </Button>
          </Link>
        </Box>
        <Box flex={1} minW={0}>
          <Heading size="md" truncate>
            {room?.name ?? "Chat Room"}
          </Heading>
        </Box>
        <Button variant="ghost" size="sm" p={0}>
          <MoreVertical size={20} />
        </Button>
      </Flex>

      {/* Messages */}
      <MessageList
        messages={messages}
        onRegenerate={handleRegenerate}
        isRegenerating={isRegenerating}
      />

      {/* Input */}
      <MessageInput
        onSend={handleSend}
        onSendWithAI={handleSendWithAI}
        models={models}
      />
    </Flex>
  )
}
