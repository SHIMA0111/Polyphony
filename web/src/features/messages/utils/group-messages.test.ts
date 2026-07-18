import { afterEach, beforeEach, describe, expect, it } from "vitest"
import type { Message } from "@/features/messages/types"
import { groupMessagesForDisplay } from "./group-messages"

/**
 * Builds a minimal `Message` for grouping tests; only the fields that
 * `groupMessagesForDisplay` inspects (`type`, `sender_id`, `created_at`) vary
 * per test, everything else is a fixed placeholder.
 */
function makeMessage(overrides: Partial<Message> & Pick<Message, "id">): Message {
  return {
    room_id: "room-1",
    sender_id: "user-1",
    content: "hello",
    type: "human",
    status: "completed",
    sequence: 1,
    in_response_to_message_id: null,
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
    created_at: "2026-01-01T12:00:00.000Z",
    updated_at: "2026-01-01T12:00:00.000Z",
    ...overrides,
  }
}

describe("groupMessagesForDisplay", () => {
  it("groups two consecutive human messages from the same sender under the time threshold on the same day", () => {
    const messages = [
      makeMessage({ id: "1", created_at: "2026-01-01T12:00:00.000Z" }),
      makeMessage({ id: "2", created_at: "2026-01-01T12:01:00.000Z" }),
    ]

    const items = groupMessagesForDisplay(messages)
    const groups = items.filter((item) => item.kind === "group")

    expect(groups).toHaveLength(1)
    expect(groups[0].messages.map((m) => m.id)).toEqual(["1", "2"])
  })

  it("starts a new group when the message type changes", () => {
    const messages = [
      makeMessage({ id: "1", type: "human", sender_id: "user-1" }),
      makeMessage({
        id: "2",
        type: "ai",
        sender_id: null,
        created_at: "2026-01-01T12:00:30.000Z",
      }),
    ]

    const items = groupMessagesForDisplay(messages)
    const groups = items.filter((item) => item.kind === "group")

    expect(groups).toHaveLength(2)
    expect(groups[0].messages.map((m) => m.id)).toEqual(["1"])
    expect(groups[1].messages.map((m) => m.id)).toEqual(["2"])
  })

  it("starts a new group when the sender changes even if the type is the same", () => {
    const messages = [
      makeMessage({ id: "1", type: "human", sender_id: "user-1" }),
      makeMessage({
        id: "2",
        type: "human",
        sender_id: "user-2",
        created_at: "2026-01-01T12:00:30.000Z",
      }),
    ]

    const items = groupMessagesForDisplay(messages)
    const groups = items.filter((item) => item.kind === "group")

    expect(groups).toHaveLength(2)
  })

  it("starts a new group when the gap since the previous message exceeds the threshold, even for the same sender", () => {
    const messages = [
      makeMessage({ id: "1", created_at: "2026-01-01T12:00:00.000Z" }),
      makeMessage({ id: "2", created_at: "2026-01-01T12:06:00.000Z" }),
    ]

    const items = groupMessagesForDisplay(messages)
    const groups = items.filter((item) => item.kind === "group")

    expect(groups).toHaveLength(2)
    expect(groups[0].messages.map((m) => m.id)).toEqual(["1"])
    expect(groups[1].messages.map((m) => m.id)).toEqual(["2"])
  })

  it("inserts a day separator when the calendar day changes, with the expected label", () => {
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 12)
    const yesterday = new Date(today.getTime() - 24 * 60 * 60 * 1000)

    const messages = [
      makeMessage({ id: "1", created_at: yesterday.toISOString() }),
      makeMessage({ id: "2", created_at: today.toISOString() }),
    ]

    const items = groupMessagesForDisplay(messages)
    const days = items.filter((item) => item.kind === "day")

    expect(days).toHaveLength(2)
    expect(days[0].label).toBe("Yesterday")
    expect(days[1].label).toBe("Today")

    // The day boundary also forces a new group, even though sender/type/gap
    // conditions alone would otherwise merge these two messages.
    const groups = items.filter((item) => item.kind === "group")
    expect(groups).toHaveLength(2)
  })

  it("renders a non-relative label for days further in the past", () => {
    const messages = [
      makeMessage({ id: "1", created_at: "2020-03-15T12:00:00.000Z" }),
    ]

    const items = groupMessagesForDisplay(messages)
    const days = items.filter((item) => item.kind === "day")

    expect(days).toHaveLength(1)
    expect(days[0].label).not.toBe("Today")
    expect(days[0].label).not.toBe("Yesterday")
  })

  it("returns an empty array for an empty message list", () => {
    expect(groupMessagesForDisplay([])).toEqual([])
  })

  describe("now: null (pre-hydration SSR pass)", () => {
    // dayLabel(date, null) always renders in UTC. Before the fix, the day-
    // boundary *key* was always computed in local time regardless of `now`,
    // so a runtime whose local timezone straddles UTC midnight differently
    // from a message's timestamp could get a key/label mismatch: two
    // adjacent day separators showing the same UTC-rendered label, or a
    // single UTC calendar day split into two groups. Force a non-UTC
    // TZ for this block so the regression would actually be exercised
    // (running under a UTC-TZ CI machine would happen to pass either way).
    const originalTZ = process.env.TZ

    beforeEach(() => {
      process.env.TZ = "Pacific/Kiritimati" // UTC+14: local date is always "ahead" of UTC date
    })

    afterEach(() => {
      process.env.TZ = originalTZ
    })

    it("groups a timestamp near midnight UTC into a single day separator whose label matches the UTC key, independent of local TZ", () => {
      // Both timestamps fall on 2026-01-01 in UTC, but under UTC+14 local
      // time they fall on different local calendar days (2026-01-01 and
      // 2026-01-02) — exactly the key/label mismatch this test guards
      // against. Kept within GROUP_GAP_MS of each other so they'd merge
      // into one group if (and only if) the day key agrees with the label.
      const messages = [
        makeMessage({ id: "1", created_at: "2026-01-01T23:58:00.000Z" }),
        makeMessage({ id: "2", created_at: "2026-01-01T23:59:30.000Z" }),
      ]

      const items = groupMessagesForDisplay(messages, null)
      const days = items.filter((item) => item.kind === "day")
      const groups = items.filter((item) => item.kind === "group")

      // A single UTC calendar day: exactly one separator, one group.
      expect(days).toHaveLength(1)
      expect(groups).toHaveLength(1)
      expect(groups[0].messages.map((m) => m.id)).toEqual(["1", "2"])

      // The label itself is the fixed-locale UTC rendering (see dayLabel's
      // `now: null` branch), consistent with the UTC key that produced
      // exactly one separator above.
      expect(days[0].label).toBe("January 1, 2026")
    })

    it("splits into two day separators when timestamps cross a UTC calendar day boundary, even though they share a local calendar day under UTC+14", () => {
      const messages = [
        makeMessage({ id: "1", created_at: "2026-01-01T23:59:00.000Z" }),
        makeMessage({ id: "2", created_at: "2026-01-02T00:01:00.000Z" }),
      ]

      const items = groupMessagesForDisplay(messages, null)
      const days = items.filter((item) => item.kind === "day")

      expect(days).toHaveLength(2)
      expect(days[0].label).toBe("January 1, 2026")
      expect(days[1].label).toBe("January 2, 2026")
    })
  })
})
