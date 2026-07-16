import { describe, it, expect, afterEach, vi } from "vitest"
import { apiRequest, ApiRequestError } from "./http-client"

/**
 * Unit tests for the shared BFF fetch core (`apiFetch`, exercised
 * indirectly through `apiRequest`). No real network or server is used —
 * `global.fetch` is replaced per test.
 *
 * Originally written against Bun's built-in test runner per
 * `docs/tasks/step4.md`, using only `describe`/`it`/`expect` and assigning
 * `global.fetch` (no Bun-only mock APIs); ported to Vitest (`mock` ->
 * `vi.fn`) once Step 5 introduced the project-wide Vitest harness; updated
 * by `docs/tasks/step30.md`'s Kratos flip, which retired the auth-plane BFF
 * helper this file used to also test, and changed the dead-session clearing
 * call on `401` from the (now-deleted) BFF logout route to Kratos's own
 * self-service logout flow.
 */

const originalFetch = global.fetch

function jsonResponse(status: number, body: unknown): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

describe("http-client", () => {
  afterEach(() => {
    global.fetch = originalFetch
  })

  it("returns a decoded JSON response on success", async () => {
    global.fetch = vi.fn(async () => jsonResponse(200, { hello: "world" }))

    const result = await apiRequest<{ hello: string }>("/rooms")

    expect(result).toEqual({ hello: "world" })
  })

  it("throws ApiRequestError with status and parsed body on a non-OK response", async () => {
    global.fetch = vi.fn(async () => jsonResponse(400, { message: "bad request" }))

    let caught: unknown
    try {
      await apiRequest("/rooms")
    } catch (err) {
      caught = err
    }

    expect(caught).toBeInstanceOf(ApiRequestError)
    const err = caught as ApiRequestError
    expect(err.status).toBe(400)
    expect(err.body).toEqual({ message: "bad request" })
    expect(err.message).toBe("bad request")
  })

  it("resolves undefined for a 204 response", async () => {
    global.fetch = vi.fn(async () => new Response(null, { status: 204 }))

    const result = await apiRequest<undefined>("/rooms/room-1")

    expect(result).toBeUndefined()
  })

  it("apiRequest prefixes paths with /api/proxy", async () => {
    let capturedUrl = ""
    global.fetch = vi.fn(async (input: RequestInfo | URL) => {
      capturedUrl = String(input)
      return jsonResponse(200, [])
    })

    await apiRequest("/rooms")

    expect(capturedUrl).toBe("/api/proxy/rooms")
  })

  it("on a 401, clears the Kratos session cookie via its self-service logout flow before redirecting to /login", async () => {
    const calledUrls: string[] = []
    const capturedSignals: (AbortSignal | null | undefined)[] = []
    global.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      calledUrls.push(url)
      capturedSignals.push(init?.signal)
      if (url === "/api/kratos/self-service/logout/browser") {
        return jsonResponse(200, {
          logout_url: "http://localhost:4433/self-service/logout?token=abc",
        })
      }
      if (url === "/api/kratos/self-service/logout?token=abc") {
        return jsonResponse(200, {})
      }
      return jsonResponse(401, { message: "unauthorized" })
    })

    const assign = vi.fn()
    const originalLocation = window.location
    // jsdom does not implement real navigation, and `window.location` is
    // non-configurable in some environments, so replace it wholesale for
    // this test only.
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...originalLocation, assign },
    })

    try {
      await expect(apiRequest("/rooms")).rejects.toBeInstanceOf(ApiRequestError)

      // The dead cookie must be cleared (via Kratos's own logout flow)
      // before the browser is redirected — otherwise middleware.ts's
      // presence-only check would immediately bounce /login back to
      // /rooms, producing an infinite redirect loop.
      expect(calledUrls).toEqual([
        "/api/proxy/rooms",
        "/api/kratos/self-service/logout/browser",
        "/api/kratos/self-service/logout?token=abc",
      ])
      expect(assign).toHaveBeenCalledWith("/login")

      // Both Kratos logout-flow fetches (indices 1 and 2 — index 0 is the
      // original /api/proxy/rooms call, which isn't bounded by this timeout)
      // must be bounded by an AbortSignal so a stalled request can't hang
      // the redirect indefinitely.
      expect(capturedSignals[1]).toBeInstanceOf(AbortSignal)
      expect(capturedSignals[2]).toBeInstanceOf(AbortSignal)
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      })
    }
  })

  it("still redirects to /login on a 401 even if clearing the Kratos session times out or fails", async () => {
    global.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === "/api/kratos/self-service/logout/browser") {
        // Simulate what an AbortSignal.timeout abort looks like: fetch
        // rejects rather than resolving.
        throw new DOMException("The operation was aborted.", "AbortError")
      }
      return jsonResponse(401, { message: "unauthorized" })
    })

    const assign = vi.fn()
    const originalLocation = window.location
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...originalLocation, assign },
    })

    try {
      await expect(apiRequest("/rooms")).rejects.toBeInstanceOf(ApiRequestError)

      // The redirect must happen unconditionally, even though clearing the
      // session failed/timed out.
      expect(assign).toHaveBeenCalledWith("/login")
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      })
    }
  })
})
