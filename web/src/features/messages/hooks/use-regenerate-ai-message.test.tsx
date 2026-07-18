import { renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import { fixtureAiMessage, fixtureHumanMessage } from "@/features/messages/api/handlers"
import { toaster } from "@/components/ui/toaster"
import type { MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import type { Message } from "@/features/messages/types"
import { useRegenerateAIMessage } from "./use-regenerate-ai-message"

/**
 * M1 post-review finding: `useRegenerateAIMessage` previously had no
 * `onError` at all (a regenerate failure was a completely silent no-op from
 * this hook's own perspective) and never invalidated the balance query on
 * success even though a regenerate debits tokens exactly like a send. These
 * tests cover the hook-level half of that fix; `use-chat-room.test.tsx`
 * covers the caller-level 402 -> `aiError` half.
 */
describe("useRegenerateAIMessage", () => {
  it("replaces the target message in the cache and invalidates the balance query on success, even when it lives in an older page rather than pages[0]", async () => {
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => HttpResponse.json<Message>({ ...fixtureAiMessage, content: "Regenerated" }),
      ),
    )

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    // Two pages (newest-first): pages[0] is a *different*, newer page that
    // never contained the target AI message; `fixtureAiMessage` lives only
    // in pages[1], the older page. `useRegenerateAIMessage` searches every
    // loaded page (`replaceMessageInAnyPage`), not just pages[0] -- a
    // single-page fixture couldn't distinguish that from a bug that only
    // ever checked pages[0].
    queryClient.setQueryData(["rooms", "room-1", "messages"], {
      pages: [
        { messages: [fixtureHumanMessage], next_cursor: "2" },
        { messages: [fixtureAiMessage], next_cursor: null },
      ],
      pageParams: [undefined, "2"],
    })

    const { result } = renderHook(() => useRegenerateAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await result.current.mutateAsync({
      aiMessageId: fixtureAiMessage.id,
      humanMessageId: "message-1",
    })

    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["billing", "balance"] }),
    )

    const data = queryClient.getQueryData<MessagesInfiniteData>(["rooms", "room-1", "messages"])
    expect(data?.pages[1]?.messages[0]?.content).toBe("Regenerated")
    // pages[0] must be left untouched by the replace.
    expect(data?.pages[0]?.messages[0]?.id).toBe(fixtureHumanMessage.id)
  })

  it("shows a generic toast for a non-402 regenerate failure", async () => {
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => HttpResponse.json({ message: "Internal Server Error" }, { status: 500 }),
      ),
    )
    const createSpy = vi.spyOn(toaster, "create")

    const { result } = renderHook(() => useRegenerateAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await expect(
      result.current.mutateAsync({
        aiMessageId: fixtureAiMessage.id,
        humanMessageId: "message-1",
      }),
    ).rejects.toThrow()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Failed to regenerate response" }),
    )
    createSpy.mockRestore()
  })

  it("suppresses the generic toast for a 402 regenerate failure (the caller's inline aiError alert covers it instead)", async () => {
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () =>
          HttpResponse.json(
            { message: "insufficient token balance" },
            { status: 402 },
          ),
      ),
    )
    const createSpy = vi.spyOn(toaster, "create")

    const { result } = renderHook(() => useRegenerateAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await expect(
      result.current.mutateAsync({
        aiMessageId: fixtureAiMessage.id,
        humanMessageId: "message-1",
      }),
    ).rejects.toThrow()

    expect(createSpy).not.toHaveBeenCalled()
    createSpy.mockRestore()
  })
})
