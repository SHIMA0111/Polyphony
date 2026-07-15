import { describe, it, expect, afterEach, vi } from "vitest"
import { apiRequest, authRequest, ApiRequestError } from "./http-client"

/**
 * Unit tests for the shared BFF fetch core (`apiFetch`, exercised indirectly
 * through `apiRequest`/`authRequest`). No real network or server is used —
 * `global.fetch` is replaced per test.
 *
 * Originally written against Bun's built-in test runner per
 * `docs/tasks/step4.md`, using only `describe`/`it`/`expect` and assigning
 * `global.fetch` (no Bun-only mock APIs); ported to Vitest (`mock` ->
 * `vi.fn`) once Step 5 introduced the project-wide Vitest harness.
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

  it("authRequest prefixes paths with /api/auth", async () => {
    let capturedUrl = ""
    global.fetch = vi.fn(async (input: RequestInfo | URL) => {
      capturedUrl = String(input)
      return jsonResponse(200, { ok: true })
    })

    await authRequest("/login")

    expect(capturedUrl).toBe("/api/auth/login")
  })

  it("on a 401, clears the session cookie via /api/auth/logout before redirecting to /login", async () => {
    const calledUrls: string[] = []
    global.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      calledUrls.push(url)
      if (url === "/api/auth/logout") {
        return jsonResponse(200, { ok: true })
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

      // The dead cookie must be cleared (via the logout route) before the
      // browser is redirected — otherwise middleware.ts's presence-only
      // check would immediately bounce /login back to /rooms, producing an
      // infinite redirect loop.
      expect(calledUrls).toEqual(["/api/proxy/rooms", "/api/auth/logout"])
      expect(assign).toHaveBeenCalledWith("/login")
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      })
    }
  })

  it("on a 401 from an auth-plane call (e.g. wrong login credentials), propagates ApiRequestError without clearing cookies or redirecting", async () => {
    const calledUrls: string[] = []
    global.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      calledUrls.push(url)
      return jsonResponse(401, { message: "invalid credentials" })
    })

    const assign = vi.fn()
    const originalLocation = window.location
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...originalLocation, assign },
    })

    try {
      let caught: unknown
      try {
        await authRequest("/login", { method: "POST" })
      } catch (err) {
        caught = err
      }

      expect(caught).toBeInstanceOf(ApiRequestError)
      expect((caught as ApiRequestError).status).toBe(401)
      expect((caught as ApiRequestError).message).toBe("invalid credentials")

      // Only the login request itself should have gone out — no
      // /api/auth/logout call, and no redirect to /login (which would wipe
      // the form before the caller's catch block can render a toast).
      expect(calledUrls).toEqual(["/api/auth/login"])
      expect(assign).not.toHaveBeenCalled()
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      })
    }
  })
})
