import { renderHook, waitFor } from "@testing-library/react"
import { delay, http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import { fixtureAiStreamResponse } from "@/features/messages/api/handlers"
import { mergeMessageEvent } from "@/features/messages/lib/merge-message-event"
import type { MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import { toaster } from "@/components/ui/toaster"
import { useSendAIMessage } from "./use-send-ai-message"

const queryKey = ["rooms", "room-1", "messages"] as const

describe("useSendAIMessage", () => {
  it("appends a sending human echo and a sending AI placeholder before the round-trip resolves", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", async () => {
        await delay(50)
        return HttpResponse.json(fixtureAiStreamResponse, { status: 202 })
      }),
    )

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    result.current.mutate({ content: "Hello, AI!", model: "gpt-5-mini" })

    await waitFor(() => {
      const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
      expect(data?.pages[0]?.messages).toHaveLength(2)
    })

    const pendingData = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    const [aiPlaceholder, humanEcho] = pendingData?.pages[0]?.messages ?? []

    // The AI placeholder is the newer of the two (front of the newest-first
    // page), so it flattens to *after* the human echo for display.
    expect(humanEcho.type).toBe("human")
    expect(humanEcho.status).toBe("sending")
    expect(humanEcho.content).toBe("Hello, AI!")

    expect(aiPlaceholder.type).toBe("ai")
    expect(aiPlaceholder.status).toBe("sending")
    expect(aiPlaceholder.content).toBe("")
    expect(aiPlaceholder.in_response_to_message_id).toBe(humanEcho.id)
  })

  it("reconciles both optimistic entries to the real messages on success", async () => {
    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await result.current.mutateAsync({ content: "Hello, AI!" })

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    // Newest-first: the AI reply landed after the human message. The AI
    // entry is the `202` response's `status: "streaming"` placeholder, not
    // finished text -- that only arrives via `token_chunk`/`message_updated`
    // WS frames (see `merge-message-event.test.ts`).
    expect(data?.pages[0]?.messages).toEqual([
      fixtureAiStreamResponse.ai_message,
      fixtureAiStreamResponse.user_message,
    ])
  })

  it("drops both optimistic entries instead of duplicating them when their WS echoes merge into the cache before the POST resolves", async () => {
    // Regression test (wave-5 review): the WS `message_created` frames for a
    // just-sent human message and its AI reply routinely arrive before this
    // mutation's own HTTP response locally. `mergeMessageEvent` can't
    // recognize either optimistic entry as "the same message" (different,
    // client-generated ids), so it prepends both server copies as new
    // entries; `onSuccess` must then remove the optimistic entries rather
    // than swap them for a *third* copy of each.
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", async () => {
        await delay(50)
        return HttpResponse.json(fixtureAiStreamResponse, { status: 202 })
      }),
    )

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    const mutatePromise = result.current.mutateAsync({ content: "Hello, AI!" })

    await waitFor(() => {
      const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
      expect(data?.pages[0]?.messages).toHaveLength(2)
    })
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) => {
      const withHuman = mergeMessageEvent(old, {
        type: "message_created",
        room_id: "room-1",
        message: fixtureAiStreamResponse.user_message,
      })
      return mergeMessageEvent(withHuman, {
        type: "message_created",
        room_id: "room-1",
        message: fixtureAiStreamResponse.ai_message,
      })
    })

    await mutatePromise

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[0]?.messages).toHaveLength(2)
    expect(data?.pages[0]?.messages).toEqual(
      expect.arrayContaining([
        fixtureAiStreamResponse.ai_message,
        fixtureAiStreamResponse.user_message,
      ]),
    )
  })

  it("streams token_chunk deltas onto the real AI message id and finalizes on message_updated (Step 54)", async () => {
    // End-to-end through the actual mutation + `mergeMessageEvent`, matching
    // how `use-room-socket.ts` really drives the cache: the AI placeholder
    // arrives via the mutation's own `202` response (not a WS
    // `message_created` echo, to keep this test focused on the chunk/finalize
    // path this step adds), then two `token_chunk` frames grow its content,
    // then a `message_updated` finalize frame replaces it with the
    // authoritative final message.
    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await result.current.mutateAsync({ content: "Hello, AI!" })

    const aiId = fixtureAiStreamResponse.ai_message.id

    queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
      mergeMessageEvent(old, {
        type: "token_chunk",
        room_id: "room-1",
        chunk: { message_id: aiId, delta: "Hello", summary_used: false },
      }),
    )
    let aiMessage = queryClient
      .getQueryData<MessagesInfiniteData>(queryKey)
      ?.pages[0]?.messages.find((m) => m.id === aiId)
    expect(aiMessage?.status).toBe("streaming")
    expect(aiMessage?.content).toBe("Hello")

    queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
      mergeMessageEvent(old, {
        type: "token_chunk",
        room_id: "room-1",
        chunk: { message_id: aiId, delta: "!", summary_used: false },
      }),
    )
    aiMessage = queryClient
      .getQueryData<MessagesInfiniteData>(queryKey)
      ?.pages[0]?.messages.find((m) => m.id === aiId)
    expect(aiMessage?.content).toBe("Hello!")

    const finalMessage = {
      ...fixtureAiStreamResponse.ai_message,
      content: "Hello!",
      status: "completed" as const,
    }
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
      mergeMessageEvent(old, {
        type: "message_updated",
        room_id: "room-1",
        message: finalMessage,
      }),
    )
    aiMessage = queryClient
      .getQueryData<MessagesInfiniteData>(queryKey)
      ?.pages[0]?.messages.find((m) => m.id === aiId)
    expect(aiMessage).toEqual(finalMessage)
  })

  it("routes stream: false sends through the non-streaming endpoint (attachment flow)", async () => {
    // `useChatRoom.handleSendWithAI` passes `stream: false` for
    // send-with-attachments so the follow-up regenerate never races a
    // still-in-flight stream's finalize (see `SendAIMessageInput.stream`).
    // The MSW handler set below would 404 the default streaming endpoint,
    // so reaching a resolved mutation proves the non-streaming route was
    // taken; its response also carries a settled `"completed"` AI message.
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", () => {
        return HttpResponse.json(
          { message: "streaming endpoint must not be called for stream: false" },
          { status: 500 },
        )
      }),
    )

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    const res = await result.current.mutateAsync({
      content: "Hello, AI!",
      stream: false,
    })

    expect(res.ai_message.status).toBe("completed")
  })

  it("rolls the human echo back to status: 'failed' and drops the AI placeholder when the request fails", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", () => {
        return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
      }),
    )

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await expect(
      result.current.mutateAsync({ content: "Hello, AI!" }),
    ).rejects.toThrow()

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[0]?.messages).toHaveLength(1)
    expect(data?.pages[0]?.messages[0]?.type).toBe("human")
    expect(data?.pages[0]?.messages[0]?.status).toBe("failed")
    expect(data?.pages[0]?.messages[0]?.content).toBe("Hello, AI!")
  })

  it("shows a generic failed-to-send toast for a non-402 failure", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", () => {
        return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
      }),
    )
    const createSpy = vi.spyOn(toaster, "create")

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await expect(
      result.current.mutateAsync({ content: "Hello, AI!" }),
    ).rejects.toThrow()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Message failed to send" }),
    )
    createSpy.mockRestore()
  })

  it("M1 post-review: suppresses the generic failed-to-send toast for a 402 rejection (the inline aiError alert covers it instead)", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai/stream", () => {
        return HttpResponse.json(
          { message: "insufficient token balance" },
          { status: 402 },
        )
      }),
    )
    const createSpy = vi.spyOn(toaster, "create")

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await expect(
      result.current.mutateAsync({ content: "Hello, AI!" }),
    ).rejects.toThrow()

    // The optimistic rollback still happens...
    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[0]?.messages[0]?.status).toBe("failed")
    // ...but no toast, since useChatRoom's inline aiError alert already
    // covers this specific rejection.
    expect(createSpy).not.toHaveBeenCalled()
    createSpy.mockRestore()
  })
})
