import { renderHook, waitFor } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import { createQueryClientWrapper } from "@/test/render"
import {
  fixtureAiMessage,
  fixtureHumanMessage,
  fixtureMessagePage,
} from "@/features/messages/api/handlers"
import { flattenMessagePages } from "@/features/messages/lib/flatten-message-pages"
import { useMessages } from "./use-messages"

/**
 * MSW-backed test for `useMessages`, proving the
 * `/api/proxy/rooms/:roomId/messages` handler contract (from
 * `../api/handlers.ts`) end to end for the `useInfiniteQuery`-based hook:
 * the first page lands as `data.pages[0]`, unreversed (still the API's
 * newest-first order) — `flattenMessagePages` (exercised separately in
 * `../lib/flatten-message-pages.test.ts`) is what reverses it for display.
 */
describe("useMessages", () => {
  it("resolves the first page of messages for a room", async () => {
    const { result } = renderHook(() => useMessages("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    expect(result.current.isPending).toBe(true)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.pages).toEqual([fixtureMessagePage])
    expect(result.current.hasNextPage).toBe(false)
  })

  it("flattens to oldest-first display order via flattenMessagePages", async () => {
    const { result } = renderHook(() => useMessages("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(flattenMessagePages(result.current.data?.pages ?? [])).toEqual([
      fixtureHumanMessage,
      fixtureAiMessage,
    ])
  })
})
