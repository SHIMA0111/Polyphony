"use client"

import { useRef, useEffect } from "react"
import { Avatar, Box, Button, Flex, Text } from "@chakra-ui/react"
import { AlertTriangle, RefreshCw } from "lucide-react"
import type { Message } from "@/types/api"

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
  const endRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [messages])

  return (
    <Box flex={1} overflowY="auto">
      <Box maxW="4xl" mx="auto" px={4} py={6} spaceY={6}>
        {messages.map((message) => (
          <Flex
            key={message.id}
            gap={3}
            role="group"
            direction={message.type === "human" ? "row-reverse" : "row"}
          >
            {/* Avatar */}
            <Avatar.Root
              size="sm"
              flexShrink={0}
              colorPalette={message.type === "ai" ? "blue" : "gray"}
            >
              <Avatar.Fallback
                name={message.type === "ai" ? "AI" : "You"}
              />
            </Avatar.Root>

            {/* Message Content */}
            <Flex
              flex={1}
              direction="column"
              align={message.type === "human" ? "flex-end" : "flex-start"}
              gap={1}
            >
              <Flex
                align="baseline"
                gap={2}
                direction={message.type === "human" ? "row-reverse" : "row"}
              >
                <Text fontSize="sm" fontWeight="medium">
                  {message.type === "ai" ? "AI" : "You"}
                </Text>
                <Text
                  fontSize="xs"
                  color="fg.muted"
                  opacity={0}
                  _groupHover={{ opacity: 1 }}
                  transition="opacity 0.2s"
                >
                  {new Date(message.created_at).toLocaleTimeString("en-US", {
                    hour: "numeric",
                    minute: "2-digit",
                  })}
                </Text>
              </Flex>

              <Box
                display="inline-block"
                rounded="2xl"
                px={4}
                py={2.5}
                maxW="85%"
                bg={
                  message.status === "failed"
                    ? "red.50"
                    : message.type === "human"
                      ? "blue.500"
                      : "bg.subtle"
                }
                color={
                  message.status === "failed"
                    ? "red.700"
                    : message.type === "human"
                      ? "white"
                      : "fg"
                }
                borderWidth={message.status === "failed" ? "1px" : 0}
                borderColor={
                  message.status === "failed" ? "red.200" : undefined
                }
              >
                {message.status === "failed" && (
                  <Flex align="center" gap={1} mb={1}>
                    <AlertTriangle size={14} />
                    <Text fontSize="xs" fontWeight="medium">
                      AI response failed
                    </Text>
                  </Flex>
                )}
                <Text
                  fontSize="15px"
                  lineHeight="relaxed"
                  whiteSpace="pre-wrap"
                >
                  {message.content || "(No response)"}
                </Text>
              </Box>

              {/* Regenerate button for AI messages */}
              {message.type === "ai" && (
                <Button
                  variant="ghost"
                  size="xs"
                  h={7}
                  px={2}
                  fontSize="xs"
                  opacity={message.status === "failed" ? 1 : 0}
                  _groupHover={{ opacity: 1 }}
                  transition="opacity 0.2s"
                  onClick={() => onRegenerate(message.id)}
                  loading={isRegenerating === message.id}
                  loadingText="Regenerating"
                >
                  <RefreshCw size={12} />
                  {message.status === "failed" ? "Retry" : "Regenerate"}
                </Button>
              )}
            </Flex>
          </Flex>
        ))}
        <div ref={endRef} />
      </Box>
    </Box>
  )
}
