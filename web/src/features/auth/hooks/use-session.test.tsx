import { renderHook, waitFor } from "@testing-library/react"
import { HttpResponse, http } from "msw"
import { describe, expect, it } from "vitest"
import { server } from "@/test/msw/server"
import { createQueryClientWrapper } from "@/test/render"
import { fixtureUser } from "@/features/auth/api/handlers"
import { useSession } from "./use-session"

/**
 * RTL + MSW test for `useSession`, exercised against the cookie-based flow:
 * the browser never holds a token, so `GET /api/proxy/users/me` (via
 * `apiRequest`) is the only session source. Covers both the authenticated
 * `200` path (the default handler, from `../api/handlers.ts`'s fixture) and
 * the unauthenticated `401` path (per-test `server.use()` override).
 */
describe("useSession", () => {
  it("resolves the authenticated user from GET /api/proxy/users/me", async () => {
    const { result } = renderHook(() => useSession(), {
      wrapper: createQueryClientWrapper(),
    })

    expect(result.current.isPending).toBe(true)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(fixtureUser)
  })

  it("maps a 401 response to a signed-out (null) session instead of an error", async () => {
    server.use(
      http.get("/api/proxy/users/me", () => {
        return HttpResponse.json({ message: "unauthorized" }, { status: 401 })
      }),
    )

    const { result } = renderHook(() => useSession(), {
      wrapper: createQueryClientWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toBeNull()
    expect(result.current.isError).toBe(false)
  })
})
