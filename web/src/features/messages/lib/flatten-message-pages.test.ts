import { describe, expect, it } from "vitest"
import type { Message, MessagePage } from "@/features/messages/types"
import { flattenMessagePages } from "./flatten-message-pages"

/** Builds a minimal `Message` fixture; only `id` varies per test call. */
function makeMessage(id: string): Message {
  return {
    id,
    room_id: "room-1",
    sender_id: "user-1",
    content: `content-${id}`,
    type: "human",
    status: "completed",
    sequence: Number(id),
    in_response_to_message_id: null,
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    created_at: `2026-01-01T00:00:0${id}Z`,
    updated_at: `2026-01-01T00:00:0${id}Z`,
  }
}

describe("flattenMessagePages", () => {
  it("flattens two pages of two messages each into oldest-to-newest order", () => {
    // Page 0 (newest page, most recently fetched): messages 4 and 3 in
    // newest-first API order.
    const page0: MessagePage = {
      messages: [makeMessage("4"), makeMessage("3")],
      next_cursor: "3",
    }
    // Page 1 (older history, fetched via fetchNextPage keyed on page0's
    // next_cursor): messages 2 and 1, also newest-first.
    const page1: MessagePage = {
      messages: [makeMessage("2"), makeMessage("1")],
      next_cursor: null,
    }

    const result = flattenMessagePages([page0, page1])

    expect(result.map((m) => m.id)).toEqual(["1", "2", "3", "4"])
  })

  it("returns an empty array for no pages", () => {
    expect(flattenMessagePages([])).toEqual([])
  })

  it("reverses a single page's newest-first order for display", () => {
    const page: MessagePage = {
      messages: [makeMessage("2"), makeMessage("1")],
      next_cursor: null,
    }

    expect(flattenMessagePages([page]).map((m) => m.id)).toEqual(["1", "2"])
  })
})
