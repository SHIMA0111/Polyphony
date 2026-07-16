import { describe, it, expect } from "vitest"
import { getQueryClient } from "./query-client"
import { ApiRequestError } from "./http-client"

/**
 * Unit tests for the shared `QueryClient` factory's `retry` policy.
 *
 * Regression coverage for the wave-2 review fix: a `401` from
 * {@link ApiRequestError} must never be retried, since `apiFetch` already
 * redirects to `/login` (after clearing the dead session cookie) on a 401 —
 * retrying would only re-issue requests guaranteed to fail identically while
 * that redirect is in flight.
 */
describe("query-client retry policy", () => {
  it("never retries a 401 ApiRequestError", () => {
    const queryClient = getQueryClient()
    const retry = queryClient.getDefaultOptions().queries?.retry

    expect(typeof retry).toBe("function")
    const shouldRetry = retry as (failureCount: number, error: unknown) => boolean

    expect(shouldRetry(0, new ApiRequestError(401, {}, "unauthorized"))).toBe(false)
  })

  it("retries other errors up to 3 times", () => {
    const queryClient = getQueryClient()
    const retry = queryClient.getDefaultOptions().queries?.retry as (
      failureCount: number,
      error: unknown,
    ) => boolean

    const otherError = new ApiRequestError(500, {}, "server error")
    expect(retry(0, otherError)).toBe(true)
    expect(retry(2, otherError)).toBe(true)
    expect(retry(3, otherError)).toBe(false)

    const genericError = new Error("network error")
    expect(retry(0, genericError)).toBe(true)
    expect(retry(3, genericError)).toBe(false)
  })
})
