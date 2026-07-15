"use client"

import { Box, Button, Flex, Spinner, Text } from "@chakra-ui/react"
import { AlertTriangle, RefreshCw } from "lucide-react"
import type { Message } from "@/features/messages/types"
import { Tooltip } from "@/components/ui/tooltip"
import { MarkdownContent } from "./MarkdownContent"
import { ThinkingBubble } from "./ThinkingBubble"

interface MessageBubbleProps {
  message: Message
  onRegenerate: (messageId: string) => void
  /** Whether this specific message is the one currently being regenerated. */
  isRegenerating: boolean
  /** Re-sends a failed human message's original content. */
  onRetry: (messageId: string, content: string) => void
}

// Both formatters below are pinned to `timeZone: "UTC"` rather than the
// host's local timezone, so the rendered string is byte-identical between
// the Next.js server render and the browser's hydration render — otherwise
// a server/browser timezone mismatch produces a React hydration error on
// every message bubble's timestamp.

/** Full date-time shown in the timestamp tooltip. */
function formatFullTimestamp(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  })
}

/** Short time-of-day shown next to each bubble. */
function formatShortTimestamp(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", {
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
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
  onRetry,
}: MessageBubbleProps) {
  const isFailed = message.status === "failed"
  const isSending = message.status === "sending"
  const isFailedHuman = isFailed && message.type === "human"

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
        opacity={isSending ? 0.6 : 1}
        bg={
          isFailed
            ? "red.50"
            : message.type === "human"
              ? "blue.500"
              : "bg.subtle"
        }
        color={
          isFailed
            ? "red.700"
            : message.type === "human"
              ? "white"
              : "fg"
        }
        borderWidth={isFailed ? "1px" : 0}
        borderColor={isFailed ? "red.200" : undefined}
      >
        {isFailed && (
          <Flex align="center" gap={1} mb={1}>
            <AlertTriangle size={14} />
            <Text fontSize="xs" fontWeight="medium">
              {message.type === "human"
                ? "Message failed to send"
                : "AI response failed"}
            </Text>
          </Flex>
        )}
        {message.type === "ai" && isSending ? (
          <ThinkingBubble />
        ) : message.type === "ai" && message.content ? (
          <MarkdownContent content={message.content} />
        ) : (
          <Text fontSize="15px" lineHeight="relaxed" whiteSpace="pre-wrap">
            {message.content || "(No response)"}
          </Text>
        )}
      </Box>

      <Flex align="center" gap={2}>
        {isSending ? (
          <Flex align="center" gap={1}>
            <Spinner size="xs" color="fg.muted" />
            <Text fontSize="xs" color="fg.muted">
              Sending…
            </Text>
          </Flex>
        ) : (
          <Tooltip content={formatFullTimestamp(message.created_at)}>
            <Text fontSize="xs" color="fg.muted" tabIndex={0}>
              {formatShortTimestamp(message.created_at)}
            </Text>
          </Tooltip>
        )}

        {message.type === "ai" && (
          <Button
            variant="ghost"
            size="xs"
            h={7}
            px={2}
            fontSize="xs"
            opacity={isFailed ? 1 : 0}
            _groupHover={{ opacity: 1 }}
            transition="opacity 0.2s"
            onClick={() => onRegenerate(message.id)}
            loading={isRegenerating}
            loadingText="Regenerating"
          >
            <RefreshCw size={12} />
            {isFailed ? "Retry" : "Regenerate"}
          </Button>
        )}

        {isFailedHuman && (
          <Button
            variant="ghost"
            size="xs"
            h={7}
            px={2}
            fontSize="xs"
            onClick={() => onRetry(message.id, message.content)}
          >
            <RefreshCw size={12} />
            Retry
          </Button>
        )}
      </Flex>
    </Flex>
  )
}
