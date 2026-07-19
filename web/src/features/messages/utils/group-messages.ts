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
 * Day-grouping key for `date`, in milliseconds since the epoch.
 *
 * Uses UTC calendar fields (`Date.UTC(...)`) when `useUtc` is true, and local
 * calendar fields ({@link startOfDay}) otherwise. This must always agree with
 * whichever calendar `dayLabel` renders the day separator's *label* from for
 * the same `now`/`null` case — see `groupMessagesForDisplay`'s call site,
 * where `now === null` selects the UTC key to match `dayLabel`'s UTC-`timeZone`
 * fallback. Using the wrong (local) key while the label renders in UTC would
 * let the key and label disagree near local midnight, splitting or merging
 * groups differently depending on the runtime's timezone.
 */
function dayGroupKey(date: Date, useUtc: boolean): number {
  if (useUtc) {
    return Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate())
  }
  return startOfDay(date)
}

/**
 * Human-readable day label for a day separator: `"Today"`/`"Yesterday"`
 * relative to `now`, otherwise a localized long date (e.g. "July 14, 2026").
 *
 * @param now - The reference "current" instant used to decide "Today" /
 *   "Yesterday", or `null` to skip relative labels entirely and always
 *   render a fixed-locale, fixed-`timeZone` absolute date instead.
 *
 *   `now: null` exists purely to avoid a React hydration mismatch: "Today"
 *   depends on wall-clock time, and both the relative-label decision and the
 *   `undefined`-locale absolute-date fallback read the *runtime's* local
 *   timezone/locale — either of which can differ between the Next.js
 *   server render and the browser. `MessageList` passes `null` for the SSR
 *   render and the browser's pre-hydration first paint (both then produce
 *   the exact same, deterministic string), then switches to a real
 *   `new Date()` once mounted, trading one harmless post-hydration label
 *   update for a first paint that is guaranteed to match.
 */
function dayLabel(date: Date, now: Date | null): string {
  if (now) {
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

  return date.toLocaleDateString("en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
    timeZone: "UTC",
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
 * @param now - Passed through to {@link dayLabel}; defaults to the real
 *   current instant, but callers rendering during SSR/pre-hydration should
 *   pass `null` (see {@link dayLabel}'s docstring) to avoid a hydration
 *   mismatch on the day-separator labels.
 * @returns Day separators and message groups in display order.
 */
export function groupMessagesForDisplay(
  messages: Message[],
  now: Date | null = new Date(),
): DisplayItem[] {
  const items: DisplayItem[] = []

  let lastDayKey: number | null = null
  let lastMessage: Message | null = null
  let currentGroup: Extract<DisplayItem, { kind: "group" }> | null = null

  for (const message of messages) {
    const createdAt = new Date(message.created_at)
    const dayKey = dayGroupKey(createdAt, now === null)

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
