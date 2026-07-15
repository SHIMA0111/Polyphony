"use client"

import { Box, Button, Flex, Text } from "@chakra-ui/react"
import { AlertTriangle, RefreshCw } from "lucide-react"
import type { Message } from "@/features/messages/types"
import { Tooltip } from "@/components/ui/tooltip"
import { MarkdownContent } from "./MarkdownContent"

interface MessageBubbleProps {
  message: Message
  onRegenerate: (messageId: string) => void
  /** Whether this specific message is the one currently being regenerated. */
  isRegenerating: boolean
}

/** Full localized date-time shown in the timestamp tooltip. */
function formatFullTimestamp(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  })
}

/** Short time-of-day shown next to each bubble. */
function formatShortTimestamp(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", {
    hour: "numeric",
    minute: "2-digit",
  })
}

/**
 * A single message bubble within a `MessageGroup`: the failed-state styling,
 * body content (markdown for AI messages, plain text for human messages),
 * the regenerate/retry action, and an always-visible dim timestamp that
 * expands to the full date-time in a tooltip on hover/focus.
 */
export function MessageBubble({
  message,
  onRegenerate,
  isRegenerating,
}: MessageBubbleProps) {
  return (
    <Flex
      direction="column"
      align={message.type === "human" ? "flex-end" : "flex-start"}
      gap={1}
      role="group"
    >
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
        borderColor={message.status === "failed" ? "red.200" : undefined}
      >
        {message.status === "failed" && (
          <Flex align="center" gap={1} mb={1}>
            <AlertTriangle size={14} />
            <Text fontSize="xs" fontWeight="medium">
              AI response failed
            </Text>
          </Flex>
        )}
        {message.type === "ai" && message.content ? (
          <MarkdownContent content={message.content} />
        ) : (
          <Text fontSize="15px" lineHeight="relaxed" whiteSpace="pre-wrap">
            {message.content || "(No response)"}
          </Text>
        )}
      </Box>

      <Flex align="center" gap={2}>
        <Tooltip content={formatFullTimestamp(message.created_at)}>
          <Text fontSize="xs" color="fg.muted" tabIndex={0}>
            {formatShortTimestamp(message.created_at)}
          </Text>
        </Tooltip>

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
            loading={isRegenerating}
            loadingText="Regenerating"
          >
            <RefreshCw size={12} />
            {message.status === "failed" ? "Retry" : "Regenerate"}
          </Button>
        )}
      </Flex>
    </Flex>
  )
}
