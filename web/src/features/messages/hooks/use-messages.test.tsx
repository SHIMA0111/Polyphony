import { renderHook, waitFor } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import { createQueryClientWrapper } from "@/test/render"
import {
  fixtureAiMessage,
  fixtureHumanMessage,
} from "@/features/messages/api/handlers"
import { useMessages } from "./use-messages"

/**
 * MSW-backed test for `useMessages`, proving the
 * `/api/proxy/rooms/:roomId/messages` handler contract (from
 * `../api/handlers.ts`) end to end, including the descending-to-ascending
 * reversal `getMessagesQueryOptions` applies for display.
 */
describe("useMessages", () => {
  it("resolves messages for a room, reversed to oldest-first for display", async () => {
    const { result } = renderHook(() => useMessages("room-1"), {
      wrapper: createQueryClientWrapper(),
    })

    expect(result.current.isPending).toBe(true)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // The fixture page is descending (AI message first); the hook's cached
    // data should be reversed to ascending (human message first).
    expect(result.current.data).toEqual([
      fixtureHumanMessage,
      fixtureAiMessage,
    ])
  })
})
