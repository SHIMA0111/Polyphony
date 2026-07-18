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
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
    pageCount,
    handleSend,
    handleSendWithAI,
    handleRegenerate,
    handleRetry,
  } = useChatRoom(roomId)

  if (isLoading) {
    return (
      <Flex h="full" flex={1} minW={0} align="center" justify="center">
        <Spinner size="xl" colorPalette="blue" />
      </Flex>
    )
  }

  return (
    <Flex h="full" flex={1} minW={0} direction="column" bg="bg">
      <ChatRoomHeader roomName={room?.name} />

      <MessageList
        messages={messages}
        onRegenerate={handleRegenerate}
        isRegenerating={isRegenerating}
        onRetry={handleRetry}
        hasNextPage={hasNextPage}
        isFetchingNextPage={isFetchingNextPage}
        fetchNextPage={fetchNextPage}
        pageCount={pageCount}
      />

      <MessageInput
        onSend={handleSend}
        onSendWithAI={handleSendWithAI}
        models={models}
      />
    </Flex>
  )
}
