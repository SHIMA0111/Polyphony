import { Avatar, Flex, Text } from "@chakra-ui/react"
import type { Message, MessageType } from "@/features/messages/types"
import { MessageBubble } from "./MessageBubble"

interface MessageGroupProps {
  type: MessageType
  /**
   * The sender of every message in this group (`groupMessagesForDisplay`
   * only merges same-sender messages into one group), or `null` for an AI
   * group. Used, together with {@link MessageGroupProps.currentUserId} and
   * {@link MessageGroupProps.senderUsernames}, to resolve the human-readable
   * label rendered above the group.
   */
  senderId: string | null
  messages: Message[]
  onRegenerate: (messageId: string) => void
  isRegenerating: string | null
  /** See `MessageBubble`'s `onRetry` doc comment for the `Promise<void>` return type. */
  onRetry: (messageId: string, content: string) => Promise<void>
  /**
   * The signed-in viewer's own user id, or `null` before it's known. A human
   * group whose `senderId` matches this is labeled "You" instead of the
   * sender's username.
   */
  currentUserId?: string | null
  /**
   * Map from member `user_id` to `username` (sourced from
   * `useMembers(roomId)`), used to label a human group that isn't the
   * viewer's own. Defaults to `{}` so callers that don't yet have a member
   * list (or don't need per-sender labels, e.g. existing tests) don't have
   * to pass one.
   */
  senderUsernames?: Record<string, string>
}

/**
 * Length of the sender-id fragment shown by {@link fallbackSenderLabel}.
 * Long enough to be distinguishable across a handful of concurrent senders
 * in the same room, short enough to still read as a label rather than a raw
 * UUID.
 */
const FALLBACK_ID_LENGTH = 6

/**
 * Human-readable fallback label for a human sender with no entry in
 * `senderUsernames` — e.g. a member who has since left the room, so the
 * current member list (which the label is otherwise resolved from) no
 * longer carries their username. Falls back to a short, still-distinct
 * fragment of their id rather than the full UUID.
 */
function fallbackSenderLabel(senderId: string): string {
  return `User ${senderId.slice(0, FALLBACK_ID_LENGTH)}`
}

/**
 * Resolves the label rendered above a message group: "AI" for an AI group;
 * for a human group, "You" if `senderId` is the viewer's own id, else the
 * sender's username from `senderUsernames`, else {@link fallbackSenderLabel}.
 */
function resolveSenderLabel(
  isHuman: boolean,
  senderId: string | null,
  currentUserId: string | null | undefined,
  senderUsernames: Record<string, string>,
): string {
  if (!isHuman) return "AI"
  if (senderId == null) return "Unknown user"
  if (currentUserId != null && senderId === currentUserId) return "You"
  return senderUsernames[senderId] ?? fallbackSenderLabel(senderId)
}

/**
 * Renders one avatar + sender name (shown once per group of consecutive
 * same-sender/same-type messages, per `groupMessagesForDisplay`) and stacks
 * the group's `MessageBubble`s below it.
 */
export function MessageGroup({
  type,
  senderId,
  messages,
  onRegenerate,
  isRegenerating,
  onRetry,
  currentUserId = null,
  senderUsernames = {},
}: MessageGroupProps) {
  const isHuman = type === "human"
  const label = resolveSenderLabel(isHuman, senderId, currentUserId, senderUsernames)

  return (
    <Flex gap={3} direction={isHuman ? "row-reverse" : "row"}>
      <Avatar.Root
        size="sm"
        flexShrink={0}
        colorPalette={isHuman ? "gray" : "blue"}
      >
        <Avatar.Fallback name={label} />
      </Avatar.Root>

      <Flex
        flex={1}
        direction="column"
        align={isHuman ? "flex-end" : "flex-start"}
        gap={2}
      >
        <Text fontSize="sm" fontWeight="medium">
          {label}
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
