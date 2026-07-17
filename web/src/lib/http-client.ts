/**
 * Shared fetch core used by every browser-side data call (see
 * `docs/tasks/step4.md`, updated by `docs/tasks/step30.md`'s Kratos flip).
 *
 * The browser never holds a bespoke token: `apiRequest` talks to the
 * Next.js catch-all proxy at `/api/proxy/*`, which forwards the browser's
 * `ory_kratos_session` cookie straight through to the Go API. Auth itself
 * (login/registration/logout) is driven through Kratos's own self-service
 * flows via `/api/kratos/*` (see `@/features/auth/api/*`), not through this
 * module.
 */

/**
 * Error thrown by {@link apiFetch} for any non-2xx response from the BFF.
 *
 * Mirrors the `ErrorResponse` shape (`{ message: string }`) already returned
 * by `server/internal/interface/handler`, but keeps the parsed body around
 * verbatim (in `body`) in case a caller needs more than just `message`.
 */
export class ApiRequestError extends Error {
  /** HTTP status code of the upstream response. */
  readonly status: number
  /** Parsed JSON error body, or `{ message: "Unknown error" }` if parsing failed. */
  readonly body: unknown

  constructor(status: number, body: unknown, message: string) {
    super(message)
    this.name = "ApiRequestError"
    this.status = status
    this.body = body
  }
}

/**
 * Extracts a human-readable message from a parsed error body, falling back
 * to a generic message that includes the HTTP status code.
 */
function extractMessage(body: unknown, status: number): string {
  if (
    typeof body === "object" &&
    body !== null &&
    "message" in body &&
    typeof (body as { message: unknown }).message === "string"
  ) {
    return (body as { message: string }).message
  }
  return `Request failed: ${status}`
}

/**
 * Upper bound, in milliseconds, on each of {@link clearKratosSession}'s two
 * fetch calls. Without this, a stalled network request (rather than a clean
 * failure) could hang indefinitely and delay the `/login` redirect that must
 * follow it — see {@link apiFetch}'s 401 handling.
 */
const CLEAR_SESSION_FETCH_TIMEOUT_MS = 3000

/**
 * Best-effort clears the Kratos session cookie via Kratos's own
 * self-service logout flow, both calls proxied through `/api/kratos/*` so
 * the browser's cookies (and Kratos's `Set-Cookie` response clearing them)
 * stay on this app's origin.
 *
 * Each fetch is bounded by {@link CLEAR_SESSION_FETCH_TIMEOUT_MS} via
 * `AbortSignal.timeout` so a stalled request can't hang this indefinitely.
 *
 * Used only by {@link apiFetch}'s `401` handling, to avoid the
 * `middleware.ts` bounce-back loop described there: if this fails or times
 * out for any reason (e.g. the session was already dead at Kratos too, or
 * the network stalls), the browser is redirected to `/login` regardless —
 * worst case, a stale-but-present cookie causes one extra bounce through the
 * middleware before Kratos's own cookie expiry resolves it. Callers must not
 * skip the redirect based on this function's outcome.
 */
async function clearKratosSession(): Promise<void> {
  try {
    const res = await fetch("/api/kratos/self-service/logout/browser", {
      headers: { Accept: "application/json" },
      signal: AbortSignal.timeout(CLEAR_SESSION_FETCH_TIMEOUT_MS),
    })
    if (!res.ok) return
    const { logout_url } = (await res.json()) as { logout_url: string }
    const relativeLogoutUrl = `/api/kratos${new URL(logout_url).pathname}${new URL(logout_url).search}`
    await fetch(relativeLogoutUrl, {
      headers: { Accept: "application/json" },
      signal: AbortSignal.timeout(CLEAR_SESSION_FETCH_TIMEOUT_MS),
    })
  } catch {
    // Best-effort only — see docstring.
  }
}

/**
 * Core fetch wrapper shared by every `apiRequest` call.
 *
 * Sets `Content-Type: application/json` by default (callers may override via
 * `options.headers`), never sets `Authorization` (the browser never holds a
 * bespoke token — the Kratos session cookie is forwarded by `/api/proxy/*`
 * itself), and:
 * - throws {@link ApiRequestError} on any non-2xx response, parsing the JSON
 *   error body (falling back to `{ message: "Unknown error" }` if the body
 *   isn't valid JSON);
 * - on a `401` response, clears the (dead) Kratos session cookie via
 *   {@link clearKratosSession} and then redirects the browser to `/login`
 *   (guarded by `typeof window !== "undefined"` so this is a no-op during
 *   SSR), since a 401 means the session cookie is missing, expired, or
 *   otherwise invalid. Clearing the cookie first matters: `middleware.ts`
 *   only checks cookie *presence*, so leaving a dead-but-present cookie in
 *   place would make the middleware redirect straight back out of `/login`
 *   (present cookie -> assumed logged in), producing an infinite
 *   401 -> redirect -> bounce-back loop instead of landing on the login
 *   page;
 * - resolves `undefined` for a `204 No Content` response;
 * - otherwise resolves the decoded JSON body.
 */
async function apiFetch<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((options.headers as Record<string, string>) ?? {}),
  }

  const res = await fetch(url, { ...options, headers })

  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: "Unknown error" }))

    if (res.status === 401 && typeof window !== "undefined") {
      await clearKratosSession()
      window.location.assign("/login")
    }

    throw new ApiRequestError(res.status, body, extractMessage(body, res.status))
  }

  if (res.status === 204) {
    return undefined as T
  }

  return res.json() as Promise<T>
}

/**
 * Calls the data-plane BFF proxy at `/api/proxy${path}`, which forwards the
 * request to the Go API with the browser's `ory_kratos_session` cookie
 * carried straight through (no bespoke bearer token is ever minted).
 */
export function apiRequest<T>(path: string, options?: RequestInit): Promise<T> {
  return apiFetch<T>(`/api/proxy${path}`, options)
}
