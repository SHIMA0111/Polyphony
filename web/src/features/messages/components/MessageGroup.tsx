import { Avatar, Flex, Text } from "@chakra-ui/react"
import type { Message, MessageType } from "@/features/messages/types"
import { MessageBubble } from "./MessageBubble"

interface MessageGroupProps {
  type: MessageType
  messages: Message[]
  onRegenerate: (messageId: string) => void
  isRegenerating: string | null
  onRetry: (messageId: string, content: string) => void
}

/**
 * Renders one avatar + sender name (shown once per group of consecutive
 * same-sender/same-type messages, per `groupMessagesForDisplay`) and stacks
 * the group's `MessageBubble`s below it.
 */
export function MessageGroup({
  type,
  messages,
  onRegenerate,
  isRegenerating,
  onRetry,
}: MessageGroupProps) {
  const isHuman = type === "human"

  return (
    <Flex gap={3} direction={isHuman ? "row-reverse" : "row"}>
      <Avatar.Root
        size="sm"
        flexShrink={0}
        colorPalette={isHuman ? "gray" : "blue"}
      >
        <Avatar.Fallback name={isHuman ? "You" : "AI"} />
      </Avatar.Root>

      <Flex
        flex={1}
        direction="column"
        align={isHuman ? "flex-end" : "flex-start"}
        gap={2}
      >
        <Text fontSize="sm" fontWeight="medium">
          {isHuman ? "You" : "AI"}
        </Text>

        {messages.map((message) => (
          <MessageBubble
            key={message.id}
            message={message}
            onRegenerate={onRegenerate}
            isRegenerating={isRegenerating === message.id}
            onRetry={onRetry}
          />
        ))}
      </Flex>
    </Flex>
  )
}
