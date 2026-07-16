"use client"

import { useState } from "react"
import {
  Badge,
  Box,
  Button,
  Dialog,
  Flex,
  IconButton,
  Menu,
  Portal,
  Spinner,
  Text,
} from "@chakra-ui/react"
import { AlertTriangle, EyeOff, Lock, MoreVertical, RefreshCw } from "lucide-react"
import type { Message } from "@/features/messages/types"
import { Tooltip } from "@/components/ui/tooltip"
import { formatDateTimeLocal } from "@/lib/format"
import { roleAtLeast } from "@/lib/roles"
import { useSession } from "@/features/auth/hooks/use-session"
import { useRoom } from "@/features/rooms/hooks/use-room"
import { useDeleteMessage } from "@/features/messages/hooks/use-delete-message"
import { useUpdateMessageExclude } from "@/features/messages/hooks/use-update-message-exclude"
import { MarkdownContent } from "./MarkdownContent"
import { MessageAttachments } from "./MessageAttachments"
import { ThinkingBubble } from "./ThinkingBubble"

interface MessageBubbleProps {
  message: Message
  onRegenerate: (messageId: string) => void
  /** Whether this specific message is the one currently being regenerated. */
  isRegenerating: boolean
  /** Re-sends a failed human message's original content. */
  onRetry: (messageId: string, content: string) => void
}

// The short-time formatter below is pinned to `timeZone: "UTC"` rather than
// the host's local timezone, so the rendered string is byte-identical
// between the Next.js server render and the browser's hydration render —
// otherwise a server/browser timezone mismatch produces a React hydration
// error on every message bubble's timestamp. The full timestamp (shown in
// the tooltip) uses the same-pinned shared `formatDateTimeLocal` (`@/lib/format`)
// instead of its own copy.

/** Short time-of-day shown next to each bubble. */
function formatShortTimestamp(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", {
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
  })
}

/**
 * Small pulsing cursor rendered at the end of a still-streaming AI
 * message's text (`status === "streaming"`, Step 54), so the bubble reads
 * as "still generating" without a second, separate indicator alongside
 * `ThinkingBubble`. Matches `ThinkingBubble`'s plain Chakra `css`-prop
 * `@keyframes` convention -- no extra animation dependency.
 */
function StreamingCursor() {
  return (
    <Box
      as="span"
      display="inline-block"
      w="2px"
      h="1em"
      ml="1px"
      bg="fg.muted"
      verticalAlign="text-bottom"
      aria-hidden="true"
      css={{
        "@keyframes streaming-cursor-blink": {
          "0%, 100%": { opacity: 1 },
          "50%": { opacity: 0 },
        },
        animation: "streaming-cursor-blink 1s step-start infinite",
      }}
    />
  )
}

/**
 * A single message bubble within a `MessageGroup`: the failed-state styling,
 * body content (markdown for AI messages, plain text for human messages),
 * the regenerate/retry action, an always-visible dim timestamp that expands
 * to the full date-time in a tooltip on hover/focus, and (Step 38) a
 * per-message action menu offering an AI-context exclude/include toggle and
 * a soft-delete action, gated by the caller's room role.
 *
 * The menu is entirely self-contained: it reads the requesting user's
 * identity (`useSession`) and room role (`useRoom(message.room_id)`, sharing
 * the same cached query `ChatRoom.tsx` already populated) directly rather
 * than needing either threaded down through `MessageList`/`MessageGroup` as
 * new props.
 */
export function MessageBubble({
  message,
  onRegenerate,
  isRegenerating,
  onRetry,
}: MessageBubbleProps) {
  const [confirmOpen, setConfirmOpen] = useState(false)

  const sessionQuery = useSession()
  const roomQuery = useRoom(message.room_id)
  const deleteMutation = useDeleteMessage(message.room_id)
  const excludeMutation = useUpdateMessageExclude(message.room_id)

  const isFailed = message.status === "failed"
  const isSending = message.status === "sending"
  /**
   * `true` while an AI message is receiving `token_chunk` WS deltas but has
   * not yet been finalized (Step 54; see `../lib/merge-message-event.ts`).
   * Distinct from `isSending`: `"sending"` is the pre-round-trip optimistic
   * state (no server id yet, always empty content), while `"streaming"` is
   * a real, persisted message id already accumulating live content.
   */
  const isStreaming = message.status === "streaming"
  const isFailedHuman = isFailed && message.type === "human"
  const isExcluded = message.exclude_from_ai
  // Step 41 guarantees a `visibility: "private"` message is only ever
  // delivered (REST or WS) to its own sender's client -- no sender-identity
  // comparison is needed here to decide whether to show the badge/border,
  // any private message present in this client's cache already belongs to
  // the current user's own private exchange.
  const isPrivate = message.visibility === "private"

  // A client-synthesized optimistic entry (see `useSendMessage`/
  // `useSendAIMessage`) has no real, persisted id yet — its menu (if any
  // were shown) would only ever 404 against the server, so both actions
  // below are gated on this in addition to the role/ownership checks.
  const isPersisted = !message.id.startsWith("optimistic-")

  const currentUserId = sessionQuery.data?.identity.id
  const role = roomQuery.data?.role
  const isOwnMessage =
    currentUserId != null && currentUserId === message.sender_id

  // `role` is `undefined` before `roomQuery` has loaded -- `roleAtLeast`
  // (shared `@/lib/roles`) requires a real `RoomRole`, so that case is
  // checked explicitly here and short-circuits to `false`, matching this
  // component's previous fail-closed local helper.
  const canToggleExclude =
    isPersisted &&
    !message.is_deleted &&
    role !== undefined &&
    roleAtLeast(role, "member")
  const canDelete =
    isPersisted &&
    !message.is_deleted &&
    (isOwnMessage || (role !== undefined && roleAtLeast(role, "admin")))
  const showMenu = canToggleExclude || canDelete

  const handleToggleExclude = () => {
    excludeMutation.mutate({ messageId: message.id, exclude: !isExcluded })
  }

  const handleConfirmDelete = async () => {
    try {
      await deleteMutation.mutateAsync(message.id)
      setConfirmOpen(false)
    } catch {
      // The mutation's own `onError` already surfaced a failure toast; keep
      // the confirmation dialog open so the user can retry or cancel rather
      // than silently discarding their intent to delete.
    }
  }

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
        opacity={isSending ? 0.6 : isExcluded ? 0.55 : 1}
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
        borderWidth={isFailed ? "1px" : isPrivate ? "1px" : 0}
        borderColor={isFailed ? "red.200" : isPrivate ? "purple.300" : undefined}
        borderStyle={!isFailed && isPrivate ? "dashed" : "solid"}
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
        {message.type === "ai" && (isSending || (isStreaming && !message.content)) ? (
          // Thinking/typing state: no content has arrived yet, whether
          // that's the pre-round-trip optimistic placeholder (`isSending`)
          // or a persisted streaming placeholder still waiting on its first
          // `token_chunk` (`isStreaming` with empty content).
          <ThinkingBubble />
        ) : message.type === "ai" && isStreaming ? (
          // Live-streaming state: content is growing in place as
          // `token_chunk` deltas arrive (see
          // `../lib/merge-message-event.ts`). Rendered as plain text rather
          // than through `MarkdownContent` -- partial markdown mid-generation
          // (an unclosed code fence, list, etc.) can render misleadingly --
          // with a pulsing cursor appended so the bubble visibly reads as
          // still in-flight. Once the terminating `message_updated` event
          // finalizes the message, `status` moves off `"streaming"` and this
          // same content renders through `MarkdownContent` below instead --
          // a content update within the same bubble, not a remount.
          <Text fontSize="15px" lineHeight="relaxed" whiteSpace="pre-wrap">
            {message.content}
            <StreamingCursor />
          </Text>
        ) : message.type === "ai" && message.content ? (
          <MarkdownContent content={message.content} />
        ) : (
          <Text fontSize="15px" lineHeight="relaxed" whiteSpace="pre-wrap">
            {message.content || "(No response)"}
          </Text>
        )}

        {message.type === "human" && !isSending && (
          <MessageAttachments message={message} />
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
        ) : isStreaming ? (
          <Flex align="center" gap={1}>
            <Spinner size="xs" color="fg.muted" />
            <Text fontSize="xs" color="fg.muted">
              Streaming…
            </Text>
          </Flex>
        ) : (
          <Tooltip content={formatDateTimeLocal(message.created_at)}>
            <Text fontSize="xs" color="fg.muted" tabIndex={0}>
              {formatShortTimestamp(message.created_at)}
            </Text>
          </Tooltip>
        )}

        {isPrivate && (
          <Tooltip content="Only you can see this exchange">
            <Badge size="xs" variant="subtle" colorPalette="purple" tabIndex={0}>
              <Lock size={10} />
              Private
            </Badge>
          </Tooltip>
        )}

        {isExcluded && (
          <Tooltip content="Excluded from AI context">
            <Flex align="center" color="fg.muted" tabIndex={0}>
              <EyeOff size={12} aria-label="Excluded from AI context" />
            </Flex>
          </Tooltip>
        )}

        {message.type === "ai" && message.used_context_summary && (
          <Tooltip content="Older messages were summarized to fit the model's context window">
            <Badge size="xs" variant="subtle" colorPalette="purple" tabIndex={0}>
              Summarized history
            </Badge>
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
            _focusVisible={{ opacity: 1 }}
            transition="opacity 0.2s"
            onClick={() => onRegenerate(message.id)}
            loading={isRegenerating}
            loadingText="Regenerating"
            disabled={isStreaming || isSending}
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

        {showMenu && (
          <Menu.Root>
            <Menu.Trigger asChild>
              <IconButton
                aria-label="Message actions"
                variant="ghost"
                size="xs"
                h={7}
                minW={7}
                opacity={0}
                _groupHover={{ opacity: 1 }}
                _focusVisible={{ opacity: 1 }}
                transition="opacity 0.2s"
              >
                <MoreVertical size={14} />
              </IconButton>
            </Menu.Trigger>
            <Portal>
              <Menu.Positioner>
                <Menu.Content>
                  {canToggleExclude && (
                    <Menu.Item
                      value="toggle-exclude"
                      onClick={handleToggleExclude}
                    >
                      {isExcluded ? "Include in AI" : "Exclude from AI"}
                    </Menu.Item>
                  )}
                  {canDelete && (
                    <Menu.Item
                      value="delete"
                      color="fg.error"
                      onClick={() => setConfirmOpen(true)}
                    >
                      Delete message
                    </Menu.Item>
                  )}
                </Menu.Content>
              </Menu.Positioner>
            </Portal>
          </Menu.Root>
        )}
      </Flex>

      <Dialog.Root
        role="alertdialog"
        open={confirmOpen}
        onOpenChange={(e) => setConfirmOpen(e.open)}
      >
        <Portal>
          <Dialog.Backdrop />
          <Dialog.Positioner>
            <Dialog.Content maxW="400px">
              <Dialog.Header>
                <Dialog.Title>Delete message?</Dialog.Title>
              </Dialog.Header>
              <Dialog.Body>
                <Text color="fg.muted">
                  This message will be removed from the room. This cannot be
                  undone.
                </Text>
              </Dialog.Body>
              <Dialog.Footer>
                <Button variant="outline" onClick={() => setConfirmOpen(false)}>
                  Cancel
                </Button>
                <Button
                  colorPalette="red"
                  loading={deleteMutation.isPending}
                  onClick={handleConfirmDelete}
                >
                  Delete
                </Button>
              </Dialog.Footer>
              <Dialog.CloseTrigger />
            </Dialog.Content>
          </Dialog.Positioner>
        </Portal>
      </Dialog.Root>
    </Flex>
  )
}
