"use client"

import { useState } from "react"
import { Flex, Spinner, Text } from "@chakra-ui/react"
import { useChatRoom } from "@/features/messages/hooks/use-chat-room"
import { useRoomSocket } from "@/features/messages/hooks/use-room-socket"
import { canInvokeAI, canSendMessage } from "@/features/members/lib/roles"
import { RoomSettingsDrawer } from "@/features/rooms/components/RoomSettingsDrawer"
import { ChatRoomHeader } from "./ChatRoomHeader"
import { ConnectionStatus } from "./ConnectionStatus"
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
    aiError,
  } = useChatRoom(roomId)
  const connectionStatus = useRoomSocket(roomId)
  const [isSettingsOpen, setIsSettingsOpen] = useState(false)

  if (isLoading) {
    return (
      <Flex h="full" flex={1} minW={0} align="center" justify="center">
        <Spinner size="xl" colorPalette="blue" />
      </Flex>
    )
  }

  const viewerRole = room?.role ?? "reader"

  return (
    <Flex h="full" flex={1} minW={0} direction="column" bg="bg">
      <ChatRoomHeader
        roomName={room?.name}
        connectionStatus={<ConnectionStatus status={connectionStatus} />}
        room={room}
        onOpenSettings={() => setIsSettingsOpen(true)}
      />
      {room && (
        <RoomSettingsDrawer
          open={isSettingsOpen}
          onOpenChange={setIsSettingsOpen}
          room={room}
          role={room.role}
        />
      )}

      <MessageList
        messages={messages}
        onRegenerate={handleRegenerate}
        isRegenerating={isRegenerating}
        onRetry={handleRetry}
        hasNextPage={hasNextPage}
        isFetchingNextPage={isFetchingNextPage}
        fetchNextPage={() => {
          void fetchNextPage()
        }}
        pageCount={pageCount}
      />

      {canSendMessage(viewerRole) ? (
        <MessageInput
          roomId={roomId}
          onSend={handleSend}
          onSendWithAI={handleSendWithAI}
          models={models}
          canInvokeAI={canInvokeAI(viewerRole)}
          messages={messages}
          aiError={aiError}
        />
      ) : (
        <Text textAlign="center" fontSize="xs" color="fg.muted" py={4}>
          You have read-only access to this room.
        </Text>
      )}
    </Flex>
  )
}
