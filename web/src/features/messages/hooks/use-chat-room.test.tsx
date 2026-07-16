import { renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper } from "@/test/render"
import { fixtureAiMessage } from "@/features/messages/api/handlers"
import type { Message, MessagePage } from "@/features/messages/types"
import { useChatRoom } from "./use-chat-room"

/**
 * `handleRegenerate` correctness test: proves the target human message id is
 * resolved from the clicked AI message's own `in_response_to_message_id`
 * link, not from scanning `messages` backwards by array position — a fixture
 * where the two approaches disagree (the AI message being regenerated is
 * *not* immediately preceded by its own human message, because a second
 * human message was interleaved before the AI reply landed) proves the old
 * bug class is actually fixed.
 */
describe("useChatRoom handleRegenerate", () => {
  it("resolves the human message id from in_response_to_message_id rather than array position", async () => {
    // Chronological (oldest-first) order: human-1, human-2, ai-1 — ai-1
    // responds to human-1, but is *positionally* preceded by human-2. The
    // API returns pages newest-first, so this is [ai-1, human-2, human-1].
    const humanOne: Message = {
      id: "human-1",
      room_id: "room-1",
      sender_id: "user-1",
      content: "First question",
      type: "human",
      status: "completed",
      sequence: 1,
      in_response_to_message_id: null,
      is_deleted: false,
      exclude_from_ai: false,
      used_context_summary: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    }
    const humanTwo: Message = {
      id: "human-2",
      room_id: "room-1",
      sender_id: "user-1",
      content: "Second question, sent before the first got a reply",
      type: "human",
      status: "completed",
      sequence: 2,
      in_response_to_message_id: null,
      is_deleted: false,
      exclude_from_ai: false,
      used_context_summary: false,
      created_at: "2026-01-01T00:00:01Z",
      updated_at: "2026-01-01T00:00:01Z",
    }
    const aiOne: Message = {
      ...fixtureAiMessage,
      id: "ai-1",
      in_response_to_message_id: "human-1",
      is_deleted: false,
      exclude_from_ai: false,
      used_context_summary: false,
      sequence: 3,
      created_at: "2026-01-01T00:00:02Z",
      updated_at: "2026-01-01T00:00:02Z",
    }

    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({
          messages: [aiOne, humanTwo, humanOne],
          next_cursor: null,
        })
      }),
    )

    let capturedMessageId: string | undefined
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        ({ params }) => {
          capturedMessageId = String(params.messageId)
          return HttpResponse.json<Message>({ ...aiOne, content: "Regenerated" })
        },
      ),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.messages.map((m) => m.id)).toEqual([
      "human-1",
      "human-2",
      "ai-1",
    ])

    await result.current.handleRegenerate("ai-1")

    await waitFor(() => expect(capturedMessageId).toBe("human-1"))
  })

  it("no-ops when the target AI message has no in_response_to_message_id", async () => {
    const orphanAiMessage: Message = {
      ...fixtureAiMessage,
      id: "ai-orphan",
      in_response_to_message_id: null,
      is_deleted: false,
      exclude_from_ai: false,
      used_context_summary: false,
    }

    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({
          messages: [orphanAiMessage],
          next_cursor: null,
        })
      }),
    )

    let regenerateCalled = false
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => {
          regenerateCalled = true
          return HttpResponse.json<Message>(orphanAiMessage)
        },
      ),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleRegenerate("ai-orphan")

    expect(regenerateCalled).toBe(false)
  })
})

/**
 * Step 48's 402 (insufficient token balance) handling: `handleSendWithAI`
 * must distinguish a `402` rejection from any other failure, surfacing it
 * via `aiError` (for `MessageInput`'s inline error) while still re-throwing
 * so `useSendAIMessage`'s own optimistic-rollback `onError` still runs, and
 * must invalidate `["billing", "balance"]` after a *successful* send.
 */
describe("useChatRoom handleSendWithAI", () => {
  it("sets aiError and does not invalidate the balance query on a 402 response", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", () => {
        return HttpResponse.json(
          { message: "insufficient token balance" },
          { status: 402 },
        )
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.aiError).toBeNull()

    await expect(
      result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini"),
    ).rejects.toThrow()

    await waitFor(() => expect(result.current.aiError).toBe("Insufficient token balance."))
  })

  it("clears any prior aiError and invalidates the balance query on a successful send", async () => {
    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini")

    expect(result.current.aiError).toBeNull()
  })

  it("does not set aiError for a non-402 failure", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", () => {
        return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await expect(
      result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini"),
    ).rejects.toThrow()

    expect(result.current.aiError).toBeNull()
  })
})
