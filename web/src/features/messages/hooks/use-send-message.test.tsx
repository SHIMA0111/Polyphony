import { renderHook, waitFor } from "@testing-library/react"
import { delay, http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import { fixtureHumanMessage } from "@/features/messages/api/handlers"
import { mergeMessageEvent } from "@/features/messages/lib/merge-message-event"
import type { MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import { useSendMessage } from "./use-send-message"

const queryKey = ["rooms", "room-1", "messages"] as const

describe("useSendMessage", () => {
  it("appends an optimistic status: 'sending' entry before the network round-trip resolves", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages", async () => {
        await delay(50)
        return HttpResponse.json(fixtureHumanMessage, { status: 201 })
      }),
    )

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    result.current.mutate("Hello, AI!")

    await waitFor(() => {
      const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
      expect(data?.pages[0]?.messages[0]?.status).toBe("sending")
    })

    // The optimistic entry carries the real content already, and a
    // client-generated id (not yet the server's real message id).
    const pendingData = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(pendingData?.pages[0]?.messages[0]?.content).toBe("Hello, AI!")
    expect(pendingData?.pages[0]?.messages[0]?.id).toMatch(/^optimistic-/)
  })

  it("reconciles the optimistic entry to the real message on success", async () => {
    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await result.current.mutateAsync("Hello, AI!")

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[0]?.messages).toEqual([fixtureHumanMessage])
  })

  it("drops the optimistic entry instead of duplicating it when the WS echo of the sent message merges into the cache before the POST resolves", async () => {
    // Regression test (wave-5 review): the WS `message_created` frame for a
    // just-sent message routinely arrives before this mutation's own HTTP
    // response locally. `mergeMessageEvent` can't recognize the optimistic
    // entry as "the same message" (different, client-generated id), so it
    // prepends the server copy as a second entry; `onSuccess` must then
    // remove the optimistic entry rather than swap it for a *third* copy.
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages", async () => {
        await delay(50)
        return HttpResponse.json(fixtureHumanMessage, { status: 201 })
      }),
    )

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    const mutatePromise = result.current.mutateAsync("Hello, AI!")

    // Wait for the optimistic entry to land, then simulate the WS echo
    // beating the POST response — exactly like `use-room-socket.ts` would
    // on `onmessage`.
    await waitFor(() => {
      const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
      expect(data?.pages[0]?.messages[0]?.status).toBe("sending")
    })
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, (old) =>
      mergeMessageEvent(old, {
        type: "message_created",
        room_id: "room-1",
        message: fixtureHumanMessage,
      }),
    )

    await mutatePromise

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[0]?.messages).toEqual([fixtureHumanMessage])
  })

  it("leaves a status: 'failed' entry in the cache (not removed) when the request fails", async () => {
    server.use(
      http.post("/api/proxy/rooms/:roomId/messages", () => {
        return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
      }),
    )

    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useSendMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await expect(result.current.mutateAsync("Hello, AI!")).rejects.toThrow()

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[0]?.messages).toHaveLength(1)
    expect(data?.pages[0]?.messages[0]?.status).toBe("failed")
    expect(data?.pages[0]?.messages[0]?.content).toBe("Hello, AI!")
  })
})
