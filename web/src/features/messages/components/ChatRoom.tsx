"use client"

import { Flex, Spinner } from "@chakra-ui/react"
import { useChatRoom } from "@/features/messages/hooks/use-chat-room"
import { ChatRoomHeader } from "./ChatRoomHeader"
import { MessageList } from "./MessageList"
import { MessageInput } from "./MessageInput"

interface ChatRoomProps {
  roomId: string
}

export function ChatRoom({ roomId }: ChatRoomProps) {
  const {
    room,
    messages,
    models,
    isLoading,
    isRegenerating,
    handleSend,
    handleSendWithAI,
    handleRegenerate,
  } = useChatRoom(roomId)

  if (isLoading) {
    return (
      <Flex h="100vh" align="center" justify="center">
        <Spinner size="xl" colorPalette="blue" />
      </Flex>
    )
  }

  return (
    <Flex h="100vh" direction="column" bg="bg">
      <ChatRoomHeader roomName={room?.name} />

      <MessageList
        messages={messages}
        onRegenerate={handleRegenerate}
        isRegenerating={isRegenerating}
      />

      <MessageInput
        onSend={handleSend}
        onSendWithAI={handleSendWithAI}
        models={models}
      />
    </Flex>
  )
}
