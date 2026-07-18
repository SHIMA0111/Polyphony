// Runtime-timezone-independence regression test for `groupMessagesForDisplay`'s
// `now === null` (pre-hydration SSR) path.
//
// Set TZ to a large positive offset (UTC+14) *before* any Date computation in this
// file, so a bug that computed the day-grouping key from local-time fields while the
// label used UTC fields (see `dayGroupKey`'s docstring in `group-messages.ts`) would
// be exercised: at that offset, two timestamps straddling UTC midnight by only 20
// minutes both fall on the *same* local calendar day, so a local-time key would fail
// to split them into separate day groups even though they render UTC-dated labels
// from different days.
//
// The original `TZ` is captured before this override so it can be restored in
// `afterAll` below: without restoring it, this override would leak into every
// other test file that shares the same Vitest worker process and runs after
// this one, silently changing their runtime timezone too.
const ORIGINAL_TZ = process.env.TZ
process.env.TZ = "Pacific/Kiritimati" // UTC+14, IANA's largest positive offset

import { afterAll, describe, expect, it } from "vitest"
import type { Message } from "@/features/messages/types"
import { groupMessagesForDisplay } from "./group-messages"

afterAll(() => {
  if (ORIGINAL_TZ === undefined) {
    delete process.env.TZ
  } else {
    process.env.TZ = ORIGINAL_TZ
  }
})

function makeMessage(overrides: Partial<Message> & Pick<Message, "id">): Message {
  return {
    room_id: "room-1",
    sender_id: "user-1",
    content: "hello",
    type: "human",
    status: "completed",
    sequence: 1,
    in_response_to_message_id: null,
    created_at: "2026-01-01T23:50:00.000Z",
    updated_at: "2026-01-01T23:50:00.000Z",
    ...overrides,
  }
}

describe("groupMessagesForDisplay (now === null, pre-hydration) day-key/label consistency", () => {
  it("splits messages straddling UTC midnight into separate UTC-dated day groups regardless of the runtime timezone", () => {
    // 20 minutes apart (well under GROUP_GAP_MS), but on different UTC calendar
    // days. At UTC+14 they fall on the *same* local calendar day.
    const messages = [
      makeMessage({ id: "1", created_at: "2026-01-01T23:50:00.000Z" }),
      makeMessage({ id: "2", created_at: "2026-01-02T00:10:00.000Z" }),
    ]

    const items = groupMessagesForDisplay(messages, null)
    const days = items.filter((item) => item.kind === "day")

    expect(days).toHaveLength(2)
    expect(days[0].label).toBe("January 1, 2026")
    expect(days[1].label).toBe("January 2, 2026")

    const groups = items.filter((item) => item.kind === "group")
    expect(groups).toHaveLength(2)
    expect(groups[0].messages.map((m) => m.id)).toEqual(["1"])
    expect(groups[1].messages.map((m) => m.id)).toEqual(["2"])
  })

  it("keeps messages on the same UTC calendar day grouped together, even near local midnight at the runtime timezone", () => {
    // Both within the same UTC day (Jan 1), 4 minutes apart (under GROUP_GAP_MS).
    const messages = [
      makeMessage({ id: "1", created_at: "2026-01-01T23:50:00.000Z" }),
      makeMessage({ id: "2", created_at: "2026-01-01T23:54:00.000Z" }),
    ]

    const items = groupMessagesForDisplay(messages, null)
    const days = items.filter((item) => item.kind === "day")

    expect(days).toHaveLength(1)
    expect(days[0].label).toBe("January 1, 2026")

    const groups = items.filter((item) => item.kind === "group")
    expect(groups).toHaveLength(1)
    expect(groups[0].messages.map((m) => m.id)).toEqual(["1", "2"])
  })
})
