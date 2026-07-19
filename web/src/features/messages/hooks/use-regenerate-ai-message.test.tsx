import { renderHook, waitFor } from "@testing-library/react"
import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import { fixtureAiMessage, fixtureHumanMessage } from "@/features/messages/api/handlers"
import type { MessagesInfiniteData } from "@/features/messages/lib/message-cache"
import { useRegenerateAIMessage } from "./use-regenerate-ai-message"

const queryKey = ["rooms", "room-1", "messages"] as const

describe("useRegenerateAIMessage", () => {
  it("replaces the matching AI message in the cache on success, even when it lives in an older page rather than pages[0]", async () => {
    const regenerated = { ...fixtureAiMessage, content: "Regenerated content" }
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => HttpResponse.json(regenerated),
      ),
    )

    const queryClient = createTestQueryClient()
    // Two pages (newest-first): pages[0] is a *different*, newer page that
    // never contained the target AI message; `fixtureAiMessage` lives only
    // in pages[1], the older page. `useRegenerateAIMessage` searches every
    // loaded page (`replaceMessageInAnyPage`), not just pages[0] -- a
    // single-page fixture couldn't distinguish that from a bug that only
    // ever checked pages[0].
    queryClient.setQueryData<MessagesInfiniteData>(queryKey, {
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
      humanMessageId: "human-1",
    })

    const data = queryClient.getQueryData<MessagesInfiniteData>(queryKey)
    expect(data?.pages[1]?.messages[0]?.content).toBe("Regenerated content")
    // pages[0] must be left untouched by the replace.
    expect(data?.pages[0]?.messages[0]?.id).toBe(fixtureHumanMessage.id)
  })

  // Regression test for item [24]: onMutate must cancel any in-flight
  // refetch of the messages query, mirroring useSendMessage/
  // useSendAIMessage's sibling hooks -- otherwise a background refetch that
  // was already in flight when regenerate was triggered could resolve
  // after onSuccess's setQueryData call and clobber the just-regenerated
  // message with stale, pre-regeneration data.
  it("cancels in-flight queries for the messages key in onMutate", async () => {
    server.use(
      http.post(
        "/api/proxy/rooms/:roomId/messages/:messageId/regenerate",
        () => HttpResponse.json(fixtureAiMessage),
      ),
    )

    const queryClient = createTestQueryClient()
    const cancelQueriesSpy = vi.spyOn(queryClient, "cancelQueries")

    const { result } = renderHook(() => useRegenerateAIMessage("room-1"), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    await result.current.mutateAsync({
      aiMessageId: fixtureAiMessage.id,
      humanMessageId: "human-1",
    })

    await waitFor(() => {
      expect(cancelQueriesSpy).toHaveBeenCalledWith({ queryKey })
    })
  })
})
