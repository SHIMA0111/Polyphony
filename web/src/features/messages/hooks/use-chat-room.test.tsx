import { renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper } from "@/test/render"
import {
  fixtureAiMessage,
  fixtureAiMessageResponse,
  fixtureHumanMessage,
} from "@/features/messages/api/handlers"
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
      created_at: "2026-01-01T00:00:01Z",
      updated_at: "2026-01-01T00:00:01Z",
    }
    const aiOne: Message = {
      ...fixtureAiMessage,
      id: "ai-1",
      in_response_to_message_id: "human-1",
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
 * `handleRetry` correctness tests (the regression fix for item [16+22]):
 * `handleRetry` must route a retry back through whichever mutation the
 * original, now-failed send actually used — the plain send mutation for a
 * plain-send failure, but the AI send mutation (with the original model)
 * for an AI-send failure — rather than always falling back to a plain
 * resend regardless of the message's original intent.
 */
describe("useChatRoom handleRetry", () => {
  it("retries a failed plain send through the plain send mutation", async () => {
    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({ messages: [], next_cursor: null })
      }),
      http.post(
        "/api/proxy/rooms/:roomId/messages",
        () => HttpResponse.json({ message: "Internal Server Error" }, { status: 500 }),
        { once: true },
      ),
      http.post("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<Message>(fixtureHumanMessage, { status: 201 })
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await expect(result.current.handleSend("Hello")).rejects.toThrow()
    await waitFor(() => {
      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0]?.status).toBe("failed")
    })
    const failedId = result.current.messages[0]?.id
    if (!failedId) throw new Error("expected a failed message id")

    await result.current.handleRetry(failedId, "Hello")

    await waitFor(() => {
      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0]?.status).toBe("completed")
      expect(result.current.messages[0]?.id).toBe(fixtureHumanMessage.id)
    })
  })

  it("retries a failed AI send through the AI send mutation with the original model", async () => {
    let aiCallCount = 0
    let capturedRetryBody: { content?: string; model?: string } | undefined

    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({ messages: [], next_cursor: null })
      }),
      http.post("/api/proxy/rooms/:roomId/messages/ai", async ({ request }) => {
        aiCallCount += 1
        if (aiCallCount === 1) {
          return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
        }
        capturedRetryBody = (await request.json()) as { content?: string; model?: string }
        return HttpResponse.json(fixtureAiMessageResponse, { status: 201 })
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await expect(
      result.current.handleSendWithAI("Hello, AI!", "gpt-5"),
    ).rejects.toThrow()
    await waitFor(() => {
      const failedHuman = result.current.messages.find((m) => m.status === "failed")
      expect(failedHuman).toBeDefined()
    })
    const failedHuman = result.current.messages.find((m) => m.status === "failed")
    if (!failedHuman) throw new Error("expected a failed human message")

    await result.current.handleRetry(failedHuman.id, "Hello, AI!")

    await waitFor(() => expect(aiCallCount).toBe(2))
    // The retry must have gone through the AI mutation (not the plain send
    // mutation) carrying the *original* model, not silently dropping it.
    expect(capturedRetryBody).toEqual({ content: "Hello, AI!", model: "gpt-5" })
    await waitFor(() => {
      expect(result.current.messages.some((m) => m.status === "failed")).toBe(false)
    })
  })
})
