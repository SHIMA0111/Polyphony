import { renderHook, waitFor } from "@testing-library/react"
import { delay, http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import { fixtureAiMessageResponse } from "@/features/messages/api/handlers"
import { mergeMessageEvent } from "@/features/messages/lib/merge-message-event"
import type { MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import { useSendAIMessage } from "./use-send-ai-message"

const queryKey = ["rooms", "room-1", "messages"] as const

describe("useSendAIMessage", () => {
  it("appends a sending human echo and a sending AI placeholder before the round-trip resolves", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai", async () => {
        await delay(50)
        return HttpResponse.json(fixtureAiMessageResponse, { status: 201 })
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
    // Newest-first: the AI reply landed after the human message.
    expect(data?.pages[0]?.messages).toEqual([
      fixtureAiMessageResponse.ai_message,
      fixtureAiMessageResponse.user_message,
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
      http.post("/api/proxy/rooms/:roomId/messages/ai", async () => {
        await delay(50)
        return HttpResponse.json(fixtureAiMessageResponse, { status: 201 })
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
        message: fixtureAiMessageResponse.user_message,
      })
      return mergeMessageEvent(withHuman, {
        type: "message_created",
        room_id: "room-1",
        message: fixtureAiMessageResponse.ai_message,
      })
    })

    await mutatePromise

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[0]?.messages).toHaveLength(2)
    expect(data?.pages[0]?.messages).toEqual(
      expect.arrayContaining([
        fixtureAiMessageResponse.ai_message,
        fixtureAiMessageResponse.user_message,
      ]),
    )
  })

  it("rolls the human echo back to status: 'failed' and drops the AI placeholder when the request fails", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai", () => {
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

  it("invokes options.onSendFailed with the failed human echo's id and the requested model", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages/ai", () => {
        return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
      }),
    )

    const onSendFailed = vi.fn()
    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendAIMessage("room-1", { onSendFailed }), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await expect(
      result.current.mutateAsync({ content: "Hello, AI!", model: "gpt-5" }),
    ).rejects.toThrow()

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    const failedHumanId = data?.pages[0]?.messages[0]?.id

    expect(onSendFailed).toHaveBeenCalledTimes(1)
    expect(onSendFailed).toHaveBeenCalledWith(failedHumanId, "gpt-5")
  })
})
