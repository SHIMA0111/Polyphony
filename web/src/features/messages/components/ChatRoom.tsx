"use client"

import { useState, useEffect, useCallback } from "react"
import Link from "next/link"
import { Box, Button, Flex, Heading, Spinner } from "@chakra-ui/react"
import { ChevronLeft, MoreVertical } from "lucide-react"
import { apiClient } from "@/lib/api"
import type { Message, Room } from "@/types/api"
import type { Model } from "./ModelSelector"
import { MessageList } from "./MessageList"
import { MessageInput } from "./MessageInput"

interface ChatRoomProps {
  roomId: string
}

export function ChatRoom({ roomId }: ChatRoomProps) {
  const [room, setRoom] = useState<Room | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [models, setModels] = useState<Model[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isRegenerating, setIsRegenerating] = useState<string | null>(null)

  const fetchData = useCallback(async () => {
    try {
      const [roomData, msgData, modelData] = await Promise.all([
        apiClient.getRoom(roomId),
        apiClient.listMessages(roomId, undefined, 100),
        apiClient.listModels(),
      ])
      setRoom(roomData)
      // Messages come in descending order from API, reverse for display
      setMessages([...msgData.messages].reverse())
      setModels(
        modelData.models.map((m) => ({
          id: m.id,
          name: m.name,
          provider: m.provider,
        })),
      )
    } catch {
      // TODO: handle error
    } finally {
      setIsLoading(false)
    }
  }, [roomId])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handleSend = useCallback(
    async (content: string) => {
      const msg = await apiClient.sendMessage(roomId, content)
      setMessages((prev) => [...prev, msg])
    },
    [roomId],
  )

  const handleSendWithAI = useCallback(
    async (content: string, model: string) => {
      const res = await apiClient.sendAIMessage(roomId, content, model)
      setMessages((prev) => [...prev, res.user_message, res.ai_message])
    },
    [roomId],
  )

  const handleRegenerate = useCallback(
    async (messageId: string) => {
      // Find the human message that precedes this AI message
      const msgIndex = messages.findIndex((m) => m.id === messageId)
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

      setIsRegenerating(messageId)
      try {
        const updated = await apiClient.regenerateAIMessage(
          roomId,
          humanMessageId,
        )
        setMessages((prev) =>
          prev.map((m) => (m.id === messageId ? updated : m)),
        )
      } catch {
        // TODO: handle error
      } finally {
        setIsRegenerating(null)
      }
    },
    [roomId, messages],
  )

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
        <Link href="/rooms">
          <Button variant="ghost" size="sm" p={0}>
            <ChevronLeft size={20} />
          </Button>
        </Link>
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
