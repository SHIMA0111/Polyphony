import { describe, expect, it, vi } from "vitest"
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
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
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

function makeChunkEvent(
  messageId: string,
  delta: string,
  summaryUsed = false,
): RoomSocketEvent {
  return {
    type: "token_chunk",
    room_id: "room-1",
    chunk: { message_id: messageId, delta, summary_used: summaryUsed },
  }
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

  it("replaces the sending optimistic AI placeholder in place when a message_created event for the real AI message arrives first (wave-9 review finding)", () => {
    const optimisticAI = makeMessage("optimistic-ai-xyz", {
      type: "ai",
      content: "",
      status: "sending",
    })
    const seeded: MessagesInfiniteData = {
      pages: [{ messages: [optimisticAI], next_cursor: null }],
      pageParams: [undefined],
    }

    const realAIMessage = makeMessage("ai-real-3", {
      type: "ai",
      content: "",
      status: "streaming",
    })
    const result = mergeMessageEvent(seeded, makeCreatedEvent(realAIMessage))

    expect(result?.pages[0].messages.map((m) => m.id)).toEqual(["ai-real-3"])
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

  describe("token_chunk (Step 54 streaming)", () => {
    it("appends a chunk's delta to an existing message and marks it status: streaming", () => {
      const placeholder = makeMessage("ai-1", {
        type: "ai",
        content: "Hello",
        status: "streaming",
      })
      const seeded: MessagesInfiniteData = {
        pages: [{ messages: [placeholder], next_cursor: null }],
        pageParams: [undefined],
      }

      const result = mergeMessageEvent(seeded, makeChunkEvent("ai-1", ", world"))

      const updated = result?.pages[0].messages.find((m) => m.id === "ai-1")
      expect(updated?.content).toBe("Hello, world")
      expect(updated?.status).toBe("streaming")
    })

    it("creates a new streaming placeholder when the first chunk for a message id arrives before any other event", () => {
      const seeded: MessagesInfiniteData = {
        pages: [{ messages: [makeMessage("1")], next_cursor: null }],
        pageParams: [undefined],
      }

      const result = mergeMessageEvent(seeded, makeChunkEvent("ai-new", "First "))

      const created = result?.pages[0].messages.find((m) => m.id === "ai-new")
      expect(created).toMatchObject({
        id: "ai-new",
        type: "ai",
        status: "streaming",
        content: "First ",
      })
      // Prepended ahead of what was already cached.
      expect(result?.pages[0].messages.map((m) => m.id)).toEqual(["ai-new", "1"])
    })

    it("accumulates content across multiple chunks for the same message id", () => {
      let data: MessagesInfiniteData | undefined = {
        pages: [{ messages: [], next_cursor: null }],
        pageParams: [undefined],
      }

      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "The "))
      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "quick "))
      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "fox"))

      expect(data?.pages[0].messages.find((m) => m.id === "ai-1")?.content).toBe(
        "The quick fox",
      )
    })

    it("preserves accumulated chunk content when a late message_created echo of the AI placeholder arrives", () => {
      let data: MessagesInfiniteData | undefined = {
        pages: [{ messages: [], next_cursor: null }],
        pageParams: [undefined],
      }

      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "The "))
      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "quick fox"))
      expect(data?.pages[0].messages[0].content).toBe("The quick fox")

      // The server's own `message_created` echo of the placeholder it
      // persisted *before* streaming began -- content still empty, status
      // still "streaming" -- arrives after the chunks above (a routine
      // race: WS frame delivery order relative to another frame is not
      // guaranteed). Replacing wholesale would wipe the content already
      // accumulated above.
      const lateEcho = makeMessage("ai-1", { type: "ai", content: "", status: "streaming" })
      data = mergeMessageEvent(data, makeCreatedEvent(lateEcho))

      const merged = data?.pages[0].messages.find((m) => m.id === "ai-1")
      expect(merged?.content).toBe("The quick fox")
      expect(merged?.status).toBe("streaming")

      // The eventual finalize event still produces the completed message as
      // usual -- this hardening only affects the intermediate created-echo.
      const finalMessage = makeMessage("ai-1", {
        type: "ai",
        content: "The quick fox jumps.",
        status: "completed",
        sequence: 7,
      })
      data = mergeMessageEvent(data, makeUpdatedEvent(finalMessage))
      expect(data?.pages[0].messages).toEqual([finalMessage])
    })

    it("ORs used_context_summary instead of taking it verbatim when a late message_created echo arrives", () => {
      let data: MessagesInfiniteData | undefined = {
        pages: [{ messages: [], next_cursor: null }],
        pageParams: [undefined],
      }

      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "partial", true))
      expect(data?.pages[0].messages[0].used_context_summary).toBe(true)

      // The created-echo's own message would wipe the flag if taken
      // verbatim (simulating a server response that omitted it).
      const lateEcho = makeMessage("ai-1", {
        type: "ai",
        content: "",
        status: "streaming",
        used_context_summary: false,
      })
      data = mergeMessageEvent(data, makeCreatedEvent(lateEcho))

      expect(data?.pages[0].messages[0]).toMatchObject({
        content: "partial",
        used_context_summary: true,
      })
    })

    it("replaces the in-flight streamed entry wholesale on the terminating message_updated finalize event", () => {
      let data: MessagesInfiniteData | undefined = {
        pages: [{ messages: [], next_cursor: null }],
        pageParams: [undefined],
      }
      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "partial"))
      expect(data?.pages[0].messages[0].status).toBe("streaming")

      const finalMessage = makeMessage("ai-1", {
        type: "ai",
        content: "partial response, finished",
        status: "completed",
        sequence: 7,
      })
      data = mergeMessageEvent(data, makeUpdatedEvent(finalMessage))

      expect(data?.pages[0].messages).toEqual([finalMessage])
    })

    it("preserves a true used_context_summary on finalize even if the finalize event's own message carries false (H1 belt-and-suspenders)", () => {
      let data: MessagesInfiniteData | undefined = {
        pages: [{ messages: [], next_cursor: null }],
        pageParams: [undefined],
      }
      // A summarized-context stream: the first chunk carries summary_used:
      // true, which applyTokenChunk OR-accumulates onto the placeholder.
      data = mergeMessageEvent(data, makeChunkEvent("ai-1", "partial", true))
      expect(data?.pages[0].messages[0].used_context_summary).toBe(true)

      // The terminating message_updated event's own message would wipe the
      // flag if taken verbatim (simulating a server that omitted it).
      const finalMessage = makeMessage("ai-1", {
        type: "ai",
        content: "partial response, finished",
        status: "completed",
        sequence: 7,
        used_context_summary: false,
      })
      data = mergeMessageEvent(data, makeUpdatedEvent(finalMessage))

      expect(data?.pages[0].messages[0]).toMatchObject({
        content: "partial response, finished",
        status: "completed",
        used_context_summary: true,
      })
    })

    it("ignores a chunk that arrives for a message already finalized as completed (idempotent finalize/chunk race)", () => {
      const finalized = makeMessage("ai-1", {
        type: "ai",
        content: "Already done.",
        status: "completed",
      })
      const seeded: MessagesInfiniteData = {
        pages: [{ messages: [finalized], next_cursor: null }],
        pageParams: [undefined],
      }

      const result = mergeMessageEvent(seeded, makeChunkEvent("ai-1", " more text"))

      expect(result).toEqual(seeded)
    })

    it("ignores a chunk that arrives for a message already finalized as failed", () => {
      const finalized = makeMessage("ai-1", {
        type: "ai",
        content: "partial before failure",
        status: "failed",
      })
      const seeded: MessagesInfiniteData = {
        pages: [{ messages: [finalized], next_cursor: null }],
        pageParams: [undefined],
      }

      const result = mergeMessageEvent(seeded, makeChunkEvent("ai-1", " ignored"))

      expect(result).toEqual(seeded)
    })

    // --- Wave-9 review finding: duplicate optimistic/streaming placeholder ---

    it("replaces the sending optimistic AI placeholder in place when the first chunk arrives before the send POST resolves", () => {
      const optimisticHuman = makeMessage("optimistic-human-abc", {
        type: "human",
        status: "sending",
      })
      const optimisticAI = makeMessage("optimistic-ai-abc", {
        type: "ai",
        content: "",
        status: "sending",
      })
      const seeded: MessagesInfiniteData = {
        pages: [{ messages: [optimisticAI, optimisticHuman], next_cursor: null }],
        pageParams: [undefined],
      }

      const result = mergeMessageEvent(seeded, makeChunkEvent("ai-real-1", "Hello"))

      const ids = result?.pages[0].messages.map((m) => m.id)
      // The real streaming message replaces the optimistic AI placeholder
      // in place (same slot) instead of being prepended alongside it --
      // exactly one AI bubble, not two.
      expect(ids).toEqual(["ai-real-1", "optimistic-human-abc"])
      expect(
        result?.pages[0].messages.find((m) => m.id === "ai-real-1"),
      ).toMatchObject({ type: "ai", status: "streaming", content: "Hello" })
    })

    it("does not touch an already-completed/failed optimistic AI entry when a later chunk arrives for a different message", () => {
      // Only a `status: "sending"` placeholder should ever be treated as
      // "still pending" -- a settled optimistic entry (shouldn't normally
      // exist, but guards against a stale one lingering) must not be
      // clobbered by an unrelated chunk.
      const settledOptimistic = makeMessage("optimistic-ai-old", {
        type: "ai",
        status: "completed",
      })
      const seeded: MessagesInfiniteData = {
        pages: [{ messages: [settledOptimistic], next_cursor: null }],
        pageParams: [undefined],
      }

      const result = mergeMessageEvent(seeded, makeChunkEvent("ai-real-2", "Hi"))

      const ids = result?.pages[0].messages.map((m) => m.id)
      expect(ids).toEqual(["ai-real-2", "optimistic-ai-old"])
    })
  })

  // --- Step 47: private AI mode ---

  it("passes visibility through untouched for a message_created event", () => {
    const result = mergeMessageEvent(
      undefined,
      makeCreatedEvent(makeMessage("1", { visibility: "private" })),
      "user-1",
    )

    expect(result?.pages[0].messages[0].visibility).toBe("private")
  })

  it("passes visibility through untouched for a message_updated event", () => {
    const seeded: MessagesInfiniteData = {
      pages: [{ messages: [makeMessage("1", { visibility: "private" })], next_cursor: null }],
      pageParams: [undefined],
    }

    const result = mergeMessageEvent(
      seeded,
      makeUpdatedEvent(makeMessage("1", { visibility: "private", content: "edited" })),
      "user-1",
    )

    expect(result?.pages[0].messages[0]).toMatchObject({
      visibility: "private",
      content: "edited",
    })
  })

  it("drops (and logs) a private message_created event whose sender_id doesn't match the current user", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {})

    const result = mergeMessageEvent(
      undefined,
      makeCreatedEvent(
        makeMessage("1", { visibility: "private", sender_id: "someone-else" }),
      ),
      "user-1",
    )

    expect(result).toBeUndefined()
    expect(consoleErrorSpy).toHaveBeenCalledTimes(1)

    consoleErrorSpy.mockRestore()
  })

  it("does not drop a private event when no currentUserId is available (guard degrades to a no-op)", () => {
    const result = mergeMessageEvent(
      undefined,
      makeCreatedEvent(
        makeMessage("1", { visibility: "private", sender_id: "someone-else" }),
      ),
      // currentUserId omitted entirely
    )

    expect(result?.pages[0].messages[0].id).toBe("1")
  })

  it("does not drop a private AI event (null sender_id) even when a currentUserId is available", () => {
    const result = mergeMessageEvent(
      undefined,
      makeCreatedEvent(
        makeMessage("1", { visibility: "private", sender_id: null, type: "ai" }),
      ),
      "user-1",
    )

    expect(result?.pages[0].messages[0].id).toBe("1")
  })

  it("merges a private event normally when sender_id matches the current user", () => {
    const result = mergeMessageEvent(
      undefined,
      makeCreatedEvent(
        makeMessage("1", { visibility: "private", sender_id: "user-1" }),
      ),
      "user-1",
    )

    expect(result?.pages[0].messages[0].id).toBe("1")
  })
})
