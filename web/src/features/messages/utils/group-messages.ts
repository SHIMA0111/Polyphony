import type { Message, MessageType } from "@/features/messages/types"

/**
 * Maximum gap, in milliseconds, between two consecutive messages from the
 * same sender/type for them to still be grouped visually. A gap at or above
 * this threshold starts a new group even if the sender and type match.
 */
export const GROUP_GAP_MS = 5 * 60 * 1000

/**
 * A rendering-ready item derived from a flat `Message[]`: either a day
 * separator (inserted whenever the calendar day changes, local time) or a
 * group of consecutive same-sender/same-type messages.
 */
export type DisplayItem =
  | { kind: "day"; label: string; iso: string }
  | {
      kind: "group"
      type: MessageType
      senderId: string | null
      messages: Message[]
    }

/** Midnight (local time) of the day containing `date`, as an epoch timestamp. */
function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
}

/**
 * Human-readable day label for a day separator: `"Today"`/`"Yesterday"`
 * relative to `now`, otherwise a localized long date (e.g. "July 14, 2026").
 */
function dayLabel(date: Date, now: Date): string {
  const dayMs = 24 * 60 * 60 * 1000
  const diffDays = Math.round((startOfDay(now) - startOfDay(date)) / dayMs)

  if (diffDays === 0) return "Today"
  if (diffDays === 1) return "Yesterday"

  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "long",
    day: "numeric",
  })
}

/**
 * Groups a flat, chronologically-ordered message list into day separators
 * and message groups for transcript rendering.
 *
 * Consecutive messages are merged into one group when all of the following
 * hold: `type` is unchanged, `sender_id` is unchanged (relevant for human
 * messages, where different users can post in the same room), the gap since
 * the previous message is under {@link GROUP_GAP_MS}, and both messages fall
 * on the same calendar day (local time). A day separator is inserted
 * whenever the calendar day changes, which also forces a new group to start
 * even if the sender/type/gap conditions would otherwise merge across the
 * boundary.
 *
 * @param messages - Messages in chronological (ascending `created_at`) order.
 * @returns Day separators and message groups in display order.
 */
export function groupMessagesForDisplay(messages: Message[]): DisplayItem[] {
  const items: DisplayItem[] = []
  const now = new Date()

  let lastDayKey: number | null = null
  let lastMessage: Message | null = null
  let currentGroup: Extract<DisplayItem, { kind: "group" }> | null = null

  for (const message of messages) {
    const createdAt = new Date(message.created_at)
    const dayKey = startOfDay(createdAt)

    if (lastDayKey === null || dayKey !== lastDayKey) {
      items.push({
        kind: "day",
        label: dayLabel(createdAt, now),
        iso: createdAt.toISOString(),
      })
      lastDayKey = dayKey
      // A day boundary always starts a fresh group.
      currentGroup = null
    }

    const gapMs = lastMessage
      ? createdAt.getTime() - new Date(lastMessage.created_at).getTime()
      : Number.POSITIVE_INFINITY

    const continuesGroup =
      currentGroup !== null &&
      currentGroup.type === message.type &&
      currentGroup.senderId === message.sender_id &&
      gapMs < GROUP_GAP_MS

    if (continuesGroup && currentGroup) {
      currentGroup.messages.push(message)
    } else {
      currentGroup = {
        kind: "group",
        type: message.type,
        senderId: message.sender_id,
        messages: [message],
      }
      items.push(currentGroup)
    }

    lastMessage = message
  }

  return items
}
