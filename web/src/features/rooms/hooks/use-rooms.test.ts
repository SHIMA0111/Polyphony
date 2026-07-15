import { renderHook, waitFor } from "@testing-library/react"
import { HttpResponse, http } from "msw"
import { describe, expect, it } from "vitest"
import { server } from "@/test/msw/server"
import { useRooms } from "./use-rooms"

/**
 * MSW-backed data-fetching-hook test for `useRooms`, proving the
 * `/api/proxy/rooms` handler contract end-to-end: the default (loading ->
 * success) path sourced from `src/test/msw/handlers.ts`'s fixture, and an
 * error path via a per-test `server.use()` override.
 */
describe("useRooms", () => {
  it("transitions from loading to a populated rooms array", async () => {
    const { result } = renderHook(() => useRooms())

    expect(result.current.isLoading).toBe(true)
    expect(result.current.rooms).toEqual([])

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.error).toBeNull()
    expect(result.current.rooms.length).toBeGreaterThan(0)
    expect(result.current.rooms[0]).toMatchObject({
      id: expect.any(String),
      name: expect.any(String),
    })
  })

  it("transitions to an error state and keeps rooms empty when the request fails", async () => {
    server.use(
      http.get("/api/proxy/rooms", () => {
        return HttpResponse.json(
          { message: "Internal Server Error" },
          { status: 500 },
        )
      }),
    )

    const { result } = renderHook(() => useRooms())

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.error).not.toBeNull()
    expect(result.current.rooms).toEqual([])
  })
})
