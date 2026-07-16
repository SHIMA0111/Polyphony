import { act, renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import type { Message } from "@/features/messages/types"
import type { MessagesInfiniteData } from "../lib/message-cache"
import { useRoomSocket } from "./use-room-socket"

/**
 * Hand-rolled fake `WebSocket`, installed via `vi.stubGlobal` (no
 * WS-mocking dependency exists in `web/package.json` — see `docs/tasks/step35.md`).
 * Exposes `simulateOpen`/`simulateMessage`/`simulateAbnormalClose` so tests
 * can drive `useRoomSocket`'s connection lifecycle deterministically without
 * a real network socket.
 */
class FakeWebSocket {
  static instances: FakeWebSocket[] = []

  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3

  readyState = FakeWebSocket.CONNECTING
  url: string
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  send(): void {
    // No client-to-server frames are sent by useRoomSocket; present only to
    // satisfy anything that might call it.
  }

  close(): void {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.()
  }

  /** Simulates the server accepting the handshake. */
  simulateOpen(): void {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.()
  }

  /** Simulates an inbound WS frame (already-serialized as the wire JSON string). */
  simulateMessage(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) })
  }

  /** Simulates the connection dropping (server restart, network blip, ...). */
  simulateAbnormalClose(): void {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.()
  }
}

const TICKET_RESPONSE = { ticket: "ticket-1", expires_in: 60 }

/** Registers a one-off `/ws/ticket` MSW handler (not part of the shared fixtures). */
function mockTicketEndpoint(): void {
  server.use(
    http.post("/api/proxy/ws/ticket", () => HttpResponse.json(TICKET_RESPONSE)),
  )
}

function makeMessage(id: string, overrides: Partial<Message> = {}): Message {
  return {
    id,
    room_id: "room-1",
    sender_id: "user-1",
    content: `content-${id}`,
    type: "human",
    status: "completed",
    sequence: Number(id.replace(/\D/g, "")) || 1,
    in_response_to_message_id: null,
    is_deleted: false,
    exclude_from_ai: false,
    used_context_summary: false,
    visibility: "public",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }
}

describe("useRoomSocket", () => {
  beforeEach(() => {
    FakeWebSocket.instances = []
    vi.stubGlobal("WebSocket", FakeWebSocket)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetches a ticket, opens a WebSocket, and reports "connected" on open', async () => {
    mockTicketEndpoint()

    const { result } = renderHook(() => useRoomSocket("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    expect(result.current).toBe("connecting")

    await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
    expect(FakeWebSocket.instances[0].url).toContain("/rooms/room-1/ws?ticket=ticket-1")

    act(() => {
      FakeWebSocket.instances[0].simulateOpen()
    })

    expect(result.current).toBe("connected")
  })

  it("merges an inbound message_created event into the room's query cache, deduping by id", async () => {
    mockTicketEndpoint()

    const queryClient = createTestQueryClient()
    const queryKey = ["rooms", "room-1", "messages"] as const
    const existing = makeMessage("message-1")

    queryClient.setQueryData<MessagesInfiniteData>(queryKey, {
      pages: [{ messages: [existing], next_cursor: null }],
      pageParams: [undefined],
    })

    renderHook(() => useRoomSocket("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
    act(() => FakeWebSocket.instances[0].simulateOpen())

    // A brand-new message id is prepended to the newest page.
    act(() => {
      FakeWebSocket.instances[0].simulateMessage({
        type: "message_created",
        room_id: "room-1",
        message: makeMessage("message-2", { content: "new message" }),
      })
    })

    let cache = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(cache?.pages[0].messages.map((m) => m.id)).toEqual([
      "message-2",
      "message-1",
    ])

    // A message_created event for an id already in the cache (e.g. this
    // client's own optimistic send, already reconciled by the REST
    // response) is deduped — reconciled in place, not appended again.
    act(() => {
      FakeWebSocket.instances[0].simulateMessage({
        type: "message_created",
        room_id: "room-1",
        message: makeMessage("message-1", { content: "reconciled" }),
      })
    })

    cache = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(cache?.pages[0].messages).toHaveLength(2)
    expect(cache?.pages[0].messages.find((m) => m.id === "message-1")?.content).toBe(
      "reconciled",
    )
  })

  it("appends a token_chunk event's delta onto the message with the matching real id (Step 54 streaming)", async () => {
    mockTicketEndpoint()

    const queryClient = createTestQueryClient()
    const queryKey = ["rooms", "room-1", "messages"] as const
    const placeholder = makeMessage("ai-1", {
      type: "ai",
      content: "",
      status: "streaming",
    })
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, {
      pages: [{ messages: [placeholder], next_cursor: null }],
      pageParams: [undefined],
    })

    renderHook(() => useRoomSocket("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
    act(() => FakeWebSocket.instances[0].simulateOpen())

    act(() => {
      FakeWebSocket.instances[0].simulateMessage({
        type: "token_chunk",
        room_id: "room-1",
        chunk: { message_id: "ai-1", delta: "Hello", summary_used: false },
      })
    })
    act(() => {
      FakeWebSocket.instances[0].simulateMessage({
        type: "token_chunk",
        room_id: "room-1",
        chunk: { message_id: "ai-1", delta: "!", summary_used: false },
      })
    })

    const cache = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    const aiMessage = cache?.pages[0].messages.find((m) => m.id === "ai-1")
    expect(aiMessage?.content).toBe("Hello!")
    expect(aiMessage?.status).toBe("streaming")
  })

  it("patches an existing message in place for a message_updated event", async () => {
    mockTicketEndpoint()

    const queryClient = createTestQueryClient()
    const queryKey = ["rooms", "room-1", "messages"] as const
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, {
      pages: [{ messages: [makeMessage("message-1")], next_cursor: null }],
      pageParams: [undefined],
    })

    renderHook(() => useRoomSocket("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
    act(() => FakeWebSocket.instances[0].simulateOpen())

    act(() => {
      FakeWebSocket.instances[0].simulateMessage({
        type: "message_updated",
        room_id: "room-1",
        message: makeMessage("message-1", { content: "edited", status: "completed" }),
      })
    })

    const cache = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(cache?.pages[0].messages).toHaveLength(1)
    expect(cache?.pages[0].messages[0].content).toBe("edited")
  })

  it("invalidates the billing balance query on a message_updated finalize event for an AI message (L1 post-review finding)", async () => {
    mockTicketEndpoint()

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const queryKey = ["rooms", "room-1", "messages"] as const
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, {
      pages: [
        {
          messages: [makeMessage("ai-1", { type: "ai", status: "streaming", content: "partial" })],
          next_cursor: null,
        },
      ],
      pageParams: [undefined],
    })

    renderHook(() => useRoomSocket("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
    act(() => FakeWebSocket.instances[0].simulateOpen())

    // A token_chunk (not a finalize) must not trigger the balance
    // invalidation.
    act(() => {
      FakeWebSocket.instances[0].simulateMessage({
        type: "token_chunk",
        room_id: "room-1",
        chunk: { message_id: "ai-1", delta: "!", summary_used: false },
      })
    })
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: ["billing", "balance"] })

    act(() => {
      FakeWebSocket.instances[0].simulateMessage({
        type: "message_updated",
        room_id: "room-1",
        message: makeMessage("ai-1", {
          type: "ai",
          status: "completed",
          content: "partial!",
        }),
      })
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["billing", "balance"] })
  })

  it(
    "reconnects with exponential backoff after an abnormal close",
    async () => {
      mockTicketEndpoint()

      const { result } = renderHook(() => useRoomSocket("room-1"), {
        wrapper: createQueryClientWrapper(),
      })

      await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
      act(() => FakeWebSocket.instances[0].simulateOpen())
      expect(result.current).toBe("connected")

      act(() => {
        FakeWebSocket.instances[0].simulateAbnormalClose()
      })
      expect(result.current).toBe("reconnecting")

      // Base backoff is 500ms (+ up to 25% jitter); allow a generous margin
      // for CI scheduling noise.
      await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(2), {
        timeout: 3000,
      })
      expect(FakeWebSocket.instances[1].url).toContain("/rooms/room-1/ws")

      act(() => FakeWebSocket.instances[1].simulateOpen())
      expect(result.current).toBe("connected")
    },
    10_000,
  )

  it(
    "invalidates the messages query on a reconnect's onopen, but not on the initial connect (M3 post-review finding)",
    async () => {
      mockTicketEndpoint()

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")

      const { result } = renderHook(() => useRoomSocket("room-1"), {
        wrapper: createQueryClientWrapper(queryClient),
      })

      await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
      act(() => FakeWebSocket.instances[0].simulateOpen())
      expect(result.current).toBe("connected")

      // The initial connect's onopen must not invalidate -- useMessages
      // already fetched on mount, so this would just be a redundant
      // duplicate fetch.
      expect(invalidateSpy).not.toHaveBeenCalled()

      act(() => {
        FakeWebSocket.instances[0].simulateAbnormalClose()
      })
      await waitFor(() => expect(FakeWebSocket.instances).toHaveLength(2), {
        timeout: 3000,
      })

      act(() => FakeWebSocket.instances[1].simulateOpen())
      expect(result.current).toBe("connected")

      // A reconnect's onopen invalidates the room's messages query so
      // whatever was published while disconnected reconciles via a fresh
      // REST fetch.
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ["rooms", "room-1", "messages"],
      })
    },
    10_000,
  )
})
