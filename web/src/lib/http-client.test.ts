import { describe, it, expect, afterEach, mock } from "bun:test"
import { apiRequest, authRequest, ApiRequestError } from "./http-client"

/**
 * Unit tests for the shared BFF fetch core (`apiFetch`, exercised indirectly
 * through `apiRequest`/`authRequest`). No real network or server is used —
 * `global.fetch` is replaced per test.
 *
 * Written against Bun's built-in test runner per `docs/tasks/step4.md`.
 * Only `describe`/`it`/`expect` and assigning `global.fetch` are used (no
 * Bun-only mock APIs) so this file can be ported to Vitest by swapping the
 * first import line once Step 5 introduces a project-wide test harness.
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
    global.fetch = mock(async () => jsonResponse(200, { hello: "world" }))

    const result = await apiRequest<{ hello: string }>("/rooms")

    expect(result).toEqual({ hello: "world" })
  })

  it("throws ApiRequestError with status and parsed body on a non-OK response", async () => {
    global.fetch = mock(async () => jsonResponse(400, { message: "bad request" }))

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
    global.fetch = mock(async () => new Response(null, { status: 204 }))

    const result = await apiRequest<undefined>("/rooms/room-1")

    expect(result).toBeUndefined()
  })

  it("apiRequest prefixes paths with /api/proxy", async () => {
    let capturedUrl = ""
    global.fetch = mock(async (input: RequestInfo | URL) => {
      capturedUrl = String(input)
      return jsonResponse(200, [])
    })

    await apiRequest("/rooms")

    expect(capturedUrl).toBe("/api/proxy/rooms")
  })

  it("authRequest prefixes paths with /api/auth", async () => {
    let capturedUrl = ""
    global.fetch = mock(async (input: RequestInfo | URL) => {
      capturedUrl = String(input)
      return jsonResponse(200, { ok: true })
    })

    await authRequest("/login")

    expect(capturedUrl).toBe("/api/auth/login")
  })
})
