"use client"

import { Box, Button } from "@chakra-ui/react"
import { ArrowDown } from "lucide-react"
import type { Message } from "@/features/messages/types"
import { useNearBottomScroll } from "@/features/messages/hooks/use-near-bottom-scroll"
import { groupMessagesForDisplay } from "@/features/messages/utils/group-messages"
import { DaySeparator } from "./DaySeparator"
import { MessageGroup } from "./MessageGroup"

interface MessageListProps {
  messages: Message[]
  onRegenerate: (messageId: string) => void
  isRegenerating: string | null
}

export function MessageList({
  messages,
  onRegenerate,
  isRegenerating,
}: MessageListProps) {
  const { containerRef, hasNewMessages, scrollToBottom } =
    useNearBottomScroll(messages)
  const items = groupMessagesForDisplay(messages)

  return (
    <Box position="relative" flex={1} minH={0}>
      <Box ref={containerRef} h="full" overflowY="auto">
        <Box maxW="4xl" mx="auto" px={4} py={6} spaceY={6}>
          {items.map((item) =>
            item.kind === "day" ? (
              <DaySeparator key={`day-${item.iso}`} label={item.label} />
            ) : (
              <MessageGroup
                key={`group-${item.messages[0].id}`}
                type={item.type}
                messages={item.messages}
                onRegenerate={onRegenerate}
                isRegenerating={isRegenerating}
              />
            ),
          )}
        </Box>
      </Box>

      {hasNewMessages && (
        <Box
          position="absolute"
          bottom={4}
          left="50%"
          transform="translateX(-50%)"
        >
          <Button
            size="sm"
            rounded="full"
            colorPalette="blue"
            shadow="md"
            onClick={scrollToBottom}
          >
            <ArrowDown size={14} />
            New messages
          </Button>
        </Box>
      )}
    </Box>
  )
}
