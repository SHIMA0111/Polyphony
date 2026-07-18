import { describe, expect, it } from "vitest"
import type { Message } from "@/features/messages/types"
import type { RoomSocketEvent } from "@/features/messages/types/ws-events"
import { mergeMessageEvent } from "./merge-message-event"
import type { MessagesInfiniteData } from "./message-cache"

/** Builds a minimal `Message` fixture; only `id`/`content`/`status` vary per call. */
function makeMessage(id: string, overrides: Partial<Message> = {}): Message {
  return {
    id,
    room_id: "room-1",
    sender_id: "user-1",
    content: `content-${id}`,
    type: "human",
    status: "completed",
    sequence: Number(id),
    in_response_to_message_id: null,
    created_at: `2026-01-01T00:00:0${id}Z`,
    updated_at: `2026-01-01T00:00:0${id}Z`,
    ...overrides,
  }
}

function makeCreatedEvent(message: Message): RoomSocketEvent {
  return { type: "message_created", room_id: "room-1", message }
}

function makeUpdatedEvent(message: Message): RoomSocketEvent {
  return { type: "message_updated", room_id: "room-1", message }
}

describe("mergeMessageEvent", () => {
  it("prepends a message_created event for a new id to the newest page", () => {
    const seeded: MessagesInfiniteData = {
      pages: [{ messages: [makeMessage("2"), makeMessage("1")], next_cursor: null }],
      pageParams: [undefined],
    }

    const result = mergeMessageEvent(seeded, makeCreatedEvent(makeMessage("3")))

    expect(result?.pages[0].messages.map((m) => m.id)).toEqual(["3", "2", "1"])
  })

  it("initializes the cache from undefined for a message_created event", () => {
    const result = mergeMessageEvent(undefined, makeCreatedEvent(makeMessage("1")))

    expect(result?.pages).toEqual([{ messages: [makeMessage("1")], next_cursor: null }])
  })

  it("does not append a duplicate for a message_created event whose id is already cached (dedup)", () => {
    const seeded: MessagesInfiniteData = {
      pages: [{ messages: [makeMessage("1")], next_cursor: null }],
      pageParams: [undefined],
    }

    const result = mergeMessageEvent(
      seeded,
      makeCreatedEvent(makeMessage("1", { content: "server-reconciled" })),
    )

    expect(result?.pages[0].messages).toHaveLength(1)
    expect(result?.pages[0].messages[0].content).toBe("server-reconciled")
  })

  it("dedupes a message_created event against an id present only in an older page", () => {
    const seeded: MessagesInfiniteData = {
      pages: [
        { messages: [makeMessage("3")], next_cursor: "2" },
        { messages: [makeMessage("2"), makeMessage("1")], next_cursor: null },
      ],
      pageParams: [undefined, "2"],
    }

    const result = mergeMessageEvent(seeded, makeCreatedEvent(makeMessage("2")))

    // No new entry added to the newest page, and the older page still has
    // exactly one copy of "2".
    expect(result?.pages[0].messages.map((m) => m.id)).toEqual(["3"])
    expect(result?.pages[1].messages.map((m) => m.id)).toEqual(["2", "1"])
  })

  it("patches an existing message in place for a message_updated event", () => {
    const seeded: MessagesInfiniteData = {
      pages: [{ messages: [makeMessage("2"), makeMessage("1")], next_cursor: null }],
      pageParams: [undefined],
    }

    const updated = makeMessage("1", { content: "edited", status: "completed" })
    const result = mergeMessageEvent(seeded, makeUpdatedEvent(updated))

    expect(result?.pages[0].messages.map((m) => m.id)).toEqual(["2", "1"])
    expect(result?.pages[0].messages.find((m) => m.id === "1")?.content).toBe("edited")
  })

  it("patches an existing message in an older page (not just the newest) for a message_updated event", () => {
    const seeded: MessagesInfiniteData = {
      pages: [
        { messages: [makeMessage("3")], next_cursor: "2" },
        { messages: [makeMessage("2"), makeMessage("1")], next_cursor: null },
      ],
      pageParams: [undefined, "2"],
    }

    const updated = makeMessage("1", { content: "regenerated" })
    const result = mergeMessageEvent(seeded, makeUpdatedEvent(updated))

    expect(result?.pages[1].messages.find((m) => m.id === "1")?.content).toBe(
      "regenerated",
    )
    // Page order/length is untouched.
    expect(result?.pages.map((p) => p.messages.length)).toEqual([1, 2])
  })

  it("no-ops a message_updated event for an id not present anywhere in the cache", () => {
    const seeded: MessagesInfiniteData = {
      pages: [{ messages: [makeMessage("1")], next_cursor: null }],
      pageParams: [undefined],
    }

    const result = mergeMessageEvent(seeded, makeUpdatedEvent(makeMessage("unknown")))

    expect(result).toBe(seeded)
  })

  it("preserves arrival order for out-of-order message_created events instead of re-sorting by sequence", () => {
    // Two create events arrive with an out-of-order `sequence` (e.g. a
    // network reordering between two independent senders); the merge must
    // not attempt to reconcile the numeric ordering itself.
    let data: MessagesInfiniteData | undefined = {
      pages: [{ messages: [], next_cursor: null }],
      pageParams: [undefined],
    }

    data = mergeMessageEvent(data, makeCreatedEvent(makeMessage("5")))
    data = mergeMessageEvent(data, makeCreatedEvent(makeMessage("4")))

    // "4" (arrived second) is prepended in front of "5" (arrived first),
    // i.e. arrival order is preserved even though 4 < 5 numerically.
    expect(data?.pages[0].messages.map((m) => m.id)).toEqual(["4", "5"])
  })
})
