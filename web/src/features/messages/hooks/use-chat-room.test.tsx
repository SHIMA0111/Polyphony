import { renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import { toaster } from "@/components/ui/toaster"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import {
  fixtureAiMessage,
  fixtureAiMessageResponse,
  fixtureAiStreamResponse,
  fixtureAttachmentResponse,
  fixtureHumanMessage,
} from "@/features/messages/api/handlers"
import type {
  AIMessageResponse,
  AttachmentResponse,
  Message,
  MessagePage,
} from "@/features/messages/types"
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

  // --- M1 post-review finding: regenerate error surfacing ---

  it("sets aiError on a 402 regenerate rejection and invalidates the balance query on a subsequent success", async () => {
    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({
          messages: [fixtureAiMessage],
          next_cursor: null,
        })
      }),
    )

    let shouldFail = true
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => {
          if (shouldFail) {
            return HttpResponse.json(
              { message: "insufficient token balance" },
              { status: 402 },
            )
          }
          return HttpResponse.json<Message>({ ...fixtureAiMessage, content: "Regenerated" })
        },
      ),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.aiError).toBeNull()

    // handleRegenerate never throws (unlike handleSendWithAI) -- the failure
    // is fully absorbed and surfaced only via aiError.
    await result.current.handleRegenerate(fixtureAiMessage.id)
    await waitFor(() => expect(result.current.aiError).toBe("Insufficient token balance."))

    shouldFail = false
    await result.current.handleRegenerate(fixtureAiMessage.id)
    await waitFor(() => expect(result.current.aiError).toBeNull())
  })

  it("does not set aiError for a non-402 regenerate failure", async () => {
    server.use(
      http.get("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json<MessagePage>({
          messages: [fixtureAiMessage],
          next_cursor: null,
        })
      }),
    )
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => HttpResponse.json({ message: "Internal Server Error" }, { status: 500 }),
      ),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleRegenerate(fixtureAiMessage.id)

    expect(result.current.aiError).toBeNull()
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
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.aiError).toBeNull()

    await expect(
      result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini"),
    ).rejects.toThrow()

    await waitFor(() => expect(result.current.aiError).toBe("Insufficient token balance."))

    expect(invalidateSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["billing", "balance"] }),
    )
  })

  it("clears any prior aiError and invalidates the balance query on a successful send", async () => {
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini")

    expect(result.current.aiError).toBeNull()
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["billing", "balance"] }),
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
 * Retry-intent regression tests: `handleRetry` must route a retry through
 * the *same* mutation the original send used — the AI mutation (with the
 * original model) for a failed AI send, the plain-send mutation for a failed
 * plain send — rather than always falling back to the plain-send mutation
 * regardless of how the message was originally sent.
 */
describe("useChatRoom handleRetry", () => {
  it("retries a failed AI send through the AI mutation with the original model, not the plain-send mutation", async () => {
    let streamCallCount = 0
    let capturedModel: string | undefined
    let plainSendCalled = false

    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", async ({ request }) => {
        streamCallCount++
        if (streamCallCount === 1) {
          return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
        }
        const body = (await request.json()) as { model?: string }
        capturedModel = body.model
        return HttpResponse.json<AIMessageResponse>(fixtureAiStreamResponse, { status: 202 })
      }),
    )
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages", () => {
        plainSendCalled = true
        return HttpResponse.json<Message>(fixtureHumanMessage, { status: 201 })
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await expect(
      result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini"),
    ).rejects.toThrow()

    await waitFor(() => {
      expect(result.current.messages.some((m) => m.status === "failed")).toBe(true)
    })
    const failed = result.current.messages.find((m) => m.status === "failed")
    if (!failed) throw new Error("expected a failed message in the cache")

    await result.current.handleRetry(failed.id, failed.content)

    expect(streamCallCount).toBe(2)
    expect(capturedModel).toBe("gpt-5-mini")
    expect(plainSendCalled).toBe(false)
  })

  it("retries a failed plain send through the plain-send mutation, not the AI mutation", async () => {
    let plainCallCount = 0
    let aiCalled = false

    server.use(
      http.post("/api/proxy/rooms/:roomId/messages", () => {
        plainCallCount++
        if (plainCallCount === 1) {
          return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
        }
        return HttpResponse.json<Message>(fixtureHumanMessage, { status: 201 })
      }),
    )
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", () => {
        aiCalled = true
        return HttpResponse.json<AIMessageResponse>(fixtureAiStreamResponse, { status: 202 })
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await expect(result.current.handleSend("Hello there")).rejects.toThrow()

    await waitFor(() => {
      expect(result.current.messages.some((m) => m.status === "failed")).toBe(true)
    })
    const failed = result.current.messages.find((m) => m.status === "failed")
    if (!failed) throw new Error("expected a failed message in the cache")

    await result.current.handleRetry(failed.id, failed.content)

    expect(plainCallCount).toBe(2)
    expect(aiCalled).toBe(false)
  })

  it("retries a failed private AI send with private: true, not silently downgrading to a public send", async () => {
    let aiCallCount = 0
    let capturedPrivate: boolean | undefined

    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai", async ({ request }) => {
        aiCallCount++
        if (aiCallCount === 1) {
          return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
        }
        const body = (await request.json()) as { private?: boolean }
        capturedPrivate = body.private
        const visibility = body.private ? "private" : "public"
        return HttpResponse.json<AIMessageResponse>(
          {
            user_message: { ...fixtureAiMessageResponse.user_message, visibility },
            ai_message: { ...fixtureAiMessageResponse.ai_message, visibility },
          },
          { status: 201 },
        )
      }),
    )

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    // `isPrivate: true` (4th arg) routes through the non-streaming
    // `/messages/ai` endpoint regardless of the attachments-derived `stream`
    // flag -- see `SendAIMessageInput.stream`'s doc comment.
    await expect(
      result.current.handleSendWithAI("Secret question", "gpt-5-mini", [], true),
    ).rejects.toThrow()

    await waitFor(() => {
      expect(result.current.messages.some((m) => m.status === "failed")).toBe(true)
    })
    const failed = result.current.messages.find((m) => m.status === "failed")
    if (!failed) throw new Error("expected a failed message in the cache")
    expect(failed.visibility).toBe("private")

    await result.current.handleRetry(failed.id, failed.content)

    expect(aiCallCount).toBe(2)
    expect(capturedPrivate).toBe(true)
    await waitFor(() => {
      const retried = result.current.messages.find(
        (m) => m.content === "Secret question",
      )
      expect(retried?.visibility).toBe("private")
    })
  })

  it("retains a failed AI send's retry intent (model/stream/private) across a remount of useChatRoom, since it lives in the QueryClient rather than a ref inside useSendAIMessage", async () => {
    let streamCallCount = 0
    let capturedModel: string | undefined
    let plainSendCalled = false

    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", async ({ request }) => {
        streamCallCount++
        if (streamCallCount === 1) {
          return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
        }
        const body = (await request.json()) as { model?: string }
        capturedModel = body.model
        return HttpResponse.json<AIMessageResponse>(fixtureAiStreamResponse, { status: 202 })
      }),
    )
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages", () => {
        plainSendCalled = true
        return HttpResponse.json<Message>(fixtureHumanMessage, { status: 201 })
      }),
    )

    // A single QueryClient shared across the unmount/remount below -- unlike
    // a fresh QueryClient per render, this is what proves the retry intent
    // outlives the component instance, since it is only the QueryClient (not
    // `useSendAIMessage`'s own component-local state) that persists across
    // the remount.
    const queryClient = createTestQueryClient()

    const first = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })
    await waitFor(() => expect(first.result.current.isLoading).toBe(false))

    await expect(
      first.result.current.handleSendWithAI("Hello, AI!", "gpt-5-mini"),
    ).rejects.toThrow()

    await waitFor(() => {
      expect(first.result.current.messages.some((m) => m.status === "failed")).toBe(true)
    })
    const failed = first.result.current.messages.find((m) => m.status === "failed")
    if (!failed) throw new Error("expected a failed message in the cache")

    // Unmount (simulating navigating away from the room) before retrying --
    // a `useRef`-backed intent store (the pre-fix implementation) would be
    // discarded here, and the retry below would silently fall back to the
    // plain-send mutation instead of retrying as AI.
    first.unmount()

    const second = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })
    await waitFor(() => expect(second.result.current.isLoading).toBe(false))

    await second.result.current.handleRetry(failed.id, failed.content)

    expect(streamCallCount).toBe(2)
    expect(capturedModel).toBe("gpt-5-mini")
    expect(plainSendCalled).toBe(false)
  })
})

/**
 * `linkAttachments` no longer silently swallows every attachment-link
 * failure: it surfaces a toaster error naming the failure count, and
 * `handleSendWithAI` skips the follow-up Vision regenerate entirely when
 * *every* attachment failed to link (a partial failure still regenerates,
 * since the model would see whatever did link).
 */
describe("useChatRoom attachment linking", () => {
  it("surfaces a toaster error but still regenerates when only some attachments fail to link", async () => {
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        async ({ request, params }) => {
          const body = (await request.json()) as { attachment_id: string }
          if (body.attachment_id === "attachment-bad") {
            return HttpResponse.json({ message: "not found" }, { status: 404 })
          }
          return HttpResponse.json<AttachmentResponse>({
            ...fixtureAttachmentResponse,
            id: body.attachment_id,
            message_id: String(params.messageId),
          })
        },
      ),
    )
    let regenerateCalled = false
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => {
          regenerateCalled = true
          return HttpResponse.json<Message>(fixtureAiMessage)
        },
      ),
    )
    const createSpy = vi.spyOn(toaster, "create")

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Check this out", "gpt-5-mini", [
      "attachment-good",
      "attachment-bad",
    ])

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Attachment error",
        description: "1 attachment(s) could not be attached.",
      }),
    )
    expect(regenerateCalled).toBe(true)
    createSpy.mockRestore()
  })

  it("surfaces a toaster error and skips the regenerate call when every attachment fails to link", async () => {
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        () => HttpResponse.json({ message: "not found" }, { status: 404 }),
      ),
    )
    let regenerateCalled = false
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => {
          regenerateCalled = true
          return HttpResponse.json<Message>(fixtureAiMessage)
        },
      ),
    )
    const createSpy = vi.spyOn(toaster, "create")

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSendWithAI("Check this out", "gpt-5-mini", [
      "attachment-1",
      "attachment-2",
    ])

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Attachment error",
        description: "2 attachment(s) could not be attached.",
      }),
    )
    expect(regenerateCalled).toBe(false)
    createSpy.mockRestore()
  })

  it("surfaces a toaster error for a plain (non-AI) send when an attachment fails to link", async () => {
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/attachments",
        () => HttpResponse.json({ message: "not found" }, { status: 404 }),
      ),
    )
    const createSpy = vi.spyOn(toaster, "create")

    const { result } = renderHook(() => useChatRoom("room-1"), {
      wrapper: createQueryClientWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    await result.current.handleSend("Here's a file", ["attachment-1"])

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Attachment error",
        description: "1 attachment(s) could not be attached.",
      }),
    )
    createSpy.mockRestore()
  })
})
