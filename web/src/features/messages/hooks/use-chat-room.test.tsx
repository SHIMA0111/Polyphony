import { renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import { toaster } from "@/components/ui/toaster"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import {
  fixtureAiMessage,
  fixtureAiMessageResponse,
  fixtureAttachmentResponse,
  fixtureHumanMessage,
} from "@/features/messages/api/handlers"
import type { AttachmentResponse, Message, MessagePage } from "@/features/messages/types"
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
      visibility: "public",
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
      visibility: "public",
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

  it("retains a failed AI send's retry intent across a remount of useChatRoom, since it lives in the QueryClient rather than a component ref", async () => {
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

    // A single QueryClient shared across the unmount/remount below -- unlike
    // a fresh QueryClient per render, this is what proves the retry intent
    // outlives the component instance, since it is only the QueryClient (not
    // any component-local ref) that persists across the remount.
    const queryClient = createTestQueryClient()

    const first = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })
    await waitFor(() => expect(first.result.current.isLoading).toBe(false))

    await expect(
      first.result.current.handleSendWithAI("Hello, AI!", "gpt-5"),
    ).rejects.toThrow()
    await waitFor(() => {
      const failedHuman = first.result.current.messages.find((m) => m.status === "failed")
      expect(failedHuman).toBeDefined()
    })
    const failedHuman = first.result.current.messages.find((m) => m.status === "failed")
    if (!failedHuman) throw new Error("expected a failed human message")

    // Unmount (simulating navigating away from the room) before retrying --
    // a `useRef`-backed intent store would be discarded here.
    first.unmount()

    const second = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })
    await waitFor(() => expect(second.result.current.isLoading).toBe(false))

    await second.result.current.handleRetry(failedHuman.id, "Hello, AI!")

    await waitFor(() => expect(aiCallCount).toBe(2))
    // The retry after remount still went through the AI mutation with the
    // original model -- proof the intent was read from the QueryClient, not
    // lost with the first render's now-unmounted component.
    expect(capturedRetryBody).toEqual({ content: "Hello, AI!", model: "gpt-5" })
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

    const queryClient = createTestQueryClient()
    const invalidateQueriesSpy = vi.spyOn(queryClient, "invalidateQueries")

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.aiError).toBeNull()

    await expect(
      result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini"),
    ).rejects.toThrow()

    await waitFor(() => expect(result.current.aiError).toBe("Insufficient token balance."))
    expect(invalidateQueriesSpy).not.toHaveBeenCalledWith({
      queryKey: ["billing", "balance"],
    })
  })

  it("clears any prior aiError and invalidates the balance query on a successful send", async () => {
    const queryClient = createTestQueryClient()
    const invalidateQueriesSpy = vi.spyOn(queryClient, "invalidateQueries")

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini")

    expect(result.current.aiError).toBeNull()
    await waitFor(() =>
      expect(invalidateQueriesSpy).toHaveBeenCalledWith({
        queryKey: ["billing", "balance"],
      }),
    )
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

/**
 * Regression tests for item [37]: `linkAttachments` previously swallowed
 * every `attachToMessage` failure silently (`console.error` only) and the
 * send still resolved, so staged files could vanish with no feedback and
 * regenerate would still fire against a message with zero attached images.
 * `linkAttachments` now reports `failedCount` back to its caller, which
 * must surface a toaster error and only skip the Vision-aware regenerate
 * call when *every* attachment failed to link -- a partial failure still
 * regenerates, since the model can still see whichever attachments did
 * link.
 */
describe("useChatRoom handleSendWithAI attachment linking", () => {
  it("shows a toaster error but still regenerates when only some attachment links fail", async () => {
    const toasterCreateSpy = vi.spyOn(toaster, "create").mockClear()
    let regenerateCallCount = 0

    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        async ({ request, params }) => {
          const body = (await request.json()) as { attachment_id: string }
          if (body.attachment_id === "attachment-fail") {
            return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
          }
          return HttpResponse.json<AttachmentResponse>({
            ...fixtureAttachmentResponse,
            message_id: String(params.messageId),
          })
        },
      ),
      http.post("/api/proxy/rooms/:roomId/messages/:messageId/regenerate", () => {
        regenerateCallCount += 1
        return HttpResponse.json<Message>(fixtureAiMessage)
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini", [
      "attachment-ok",
      "attachment-fail",
    ])

    await waitFor(() => expect(regenerateCallCount).toBe(1))
    expect(toasterCreateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "error",
        description: "1 attachment(s) could not be attached.",
      }),
    )
  })

  it("shows a toaster error and skips the regenerate call when every attachment link fails", async () => {
    const toasterCreateSpy = vi.spyOn(toaster, "create").mockClear()
    let regenerateCallCount = 0

    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        () => HttpResponse.json({ message: "Internal Server Error" }, { status: 500 }),
      ),
      http.post("/api/proxy/rooms/:roomId/messages/:messageId/regenerate", () => {
        regenerateCallCount += 1
        return HttpResponse.json<Message>(fixtureAiMessage)
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini", ["attachment-fail"])

    await waitFor(() =>
      expect(toasterCreateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "error",
          description: "1 attachment(s) could not be attached.",
        }),
      ),
    )
    expect(regenerateCallCount).toBe(0)
  })

  it("does not show a toaster error when every attachment links successfully", async () => {
    const toasterCreateSpy = vi.spyOn(toaster, "create").mockClear()
    let regenerateCallCount = 0

    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/:messageId/regenerate", () => {
        regenerateCallCount += 1
        return HttpResponse.json<Message>(fixtureAiMessage)
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini", ["attachment-ok"])

    await waitFor(() => expect(regenerateCallCount).toBe(1))
    expect(toasterCreateSpy).not.toHaveBeenCalled()
  })

  it("surfaces a toaster error for a plain (non-AI) send's failed attachment link too", async () => {
    const toasterCreateSpy = vi.spyOn(toaster, "create").mockClear()

    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        () => HttpResponse.json({ message: "Internal Server Error" }, { status: 500 }),
      ),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSend("Hello", ["attachment-fail"])

    await waitFor(() =>
      expect(toasterCreateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "error",
          description: "1 attachment(s) could not be attached.",
        }),
      ),
    )
  })
})
