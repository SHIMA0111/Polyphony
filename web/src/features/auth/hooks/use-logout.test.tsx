import { renderHook, waitFor } from "@testing-library/react"
import { HttpResponse, http } from "msw"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper, createTestQueryClient } from "@/test/render"
import { useLogout } from "./use-logout"

const pushMock = vi.fn()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}))

/**
 * RTL + MSW tests for `useLogout`.
 *
 * Covers both the happy path (Kratos logout succeeds) and the failure path
 * (the Kratos logout call errors out) to guard the `onSettled`-based
 * clearing/navigation: a failed logout must still clear the entire query
 * cache and navigate to `/login`, since stranding the user on the current
 * page after a failed logout attempt is worse than navigating away with a
 * possibly-stale server-side session.
 */
describe("useLogout", () => {
  beforeEach(() => {
    pushMock.mockClear()
  })

  it("clears the query cache and navigates to /login on success", async () => {
    const queryClient = createTestQueryClient()
    const clearSpy = vi.spyOn(queryClient, "clear")

    const { result } = renderHook(() => useLogout(), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    result.current.mutate()

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(clearSpy).toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith("/login")
  })

  it("still clears the query cache and navigates to /login when the Kratos logout call fails", async () => {
    server.use(
      http.get("/api/kratos/self-service/logout/browser", () => {
        return HttpResponse.json({ error: "boom" }, { status: 500 })
      }),
    )

    const queryClient = createTestQueryClient()
    const clearSpy = vi.spyOn(queryClient, "clear")

    const { result } = renderHook(() => useLogout(), {
      wrapper: createQueryClientWrapper(queryClient),
    })

    result.current.mutate()

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(clearSpy).toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith("/login")
  })
})
