import { renderHook, waitFor } from "@testing-library/react"
import { HttpResponse, http } from "msw"
import { describe, expect, it } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper } from "@/test/render"
import { fixtureRooms } from "@/features/rooms/api/handlers"
import { useRooms } from "./use-rooms"

/**
 * TanStack Query version of the MSW-backed data-fetching-hook test for
 * `useRooms`, proving the `/api/proxy/rooms` handler contract (from
 * `../api/handlers.ts`) still works end to end after the Step 9 migration
 * off the Step 5 placeholder `useState`/`useEffect` hook.
 */
describe("useRooms", () => {
  it("resolves the rooms list from the mocked GET /api/proxy/rooms handler", async () => {
    const { result } = renderHook(() => useRooms(), {
      wrapper: createQueryClientWrapper(),
    })

    expect(result.current.isPending).toBe(true)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(fixtureRooms)
  })

  it("surfaces a query error when the request fails", async () => {
    server.use(
      http.get("/api/proxy/rooms", () => {
        return HttpResponse.json(
          { message: "Internal Server Error" },
          { status: 500 },
        )
      }),
    )

    const { result } = renderHook(() => useRooms(), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.data).toBeUndefined()
  })
})
