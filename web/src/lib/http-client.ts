/**
 * Shared fetch core used by every browser-side data call after the JWT
 * moved server-side (see `docs/tasks/step4.md`).
 *
 * The browser never holds a token: `apiRequest` talks to the Next.js
 * catch-all proxy at `/api/proxy/*` (which attaches `Authorization` from an
 * httpOnly cookie server-side), and `authRequest` talks to the `/api/auth/*`
 * route handlers (which exchange credentials for a token and set the cookie
 * without ever returning the token to client JavaScript).
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
 * Upper bound, in milliseconds, on the best-effort `POST /api/auth/logout`
 * call made from {@link apiFetch}'s 401 handling. Without this, a stalled
 * network request (rather than a clean failure) could hang indefinitely and
 * delay the `/login` redirect that must follow it regardless of whether the
 * clear succeeded.
 */
const CLEAR_SESSION_FETCH_TIMEOUT_MS = 3000

/**
 * Core fetch wrapper shared by {@link apiRequest} and {@link authRequest}.
 *
 * Sets `Content-Type: application/json` by default (callers may override via
 * `options.headers`), never sets `Authorization` (the browser never holds a
 * token — that only happens server-side, inside the route handlers), and:
 * - throws {@link ApiRequestError} on any non-2xx response, parsing the JSON
 *   error body (falling back to `{ message: "Unknown error" }` if the body
 *   isn't valid JSON);
 * - on a `401` response, *unless* `skipAuthRedirect` is set, clears the
 *   (dead) session cookie via `POST /api/auth/logout` (bounded by
 *   {@link CLEAR_SESSION_FETCH_TIMEOUT_MS} via `AbortSignal.timeout` so a
 *   stalled request can't hang this indefinitely) and then redirects the
 *   browser to `/login` regardless of whether that call succeeded, failed,
 *   or timed out (guarded by `typeof window !== "undefined"` so this is a
 *   no-op during SSR), since a 401 means the session cookie is missing,
 *   expired, or otherwise invalid. Clearing the cookie first matters:
 *   `middleware.ts` only checks cookie *presence*, so leaving a
 *   dead-but-present cookie in place would make the middleware redirect
 *   straight back out of `/login` (present cookie -> assumed logged in),
 *   producing an infinite 401 -> redirect -> bounce-back loop instead of
 *   landing on the login page. The redirect must never be skipped based on
 *   the clear's outcome — worst case, a stale-but-present cookie causes one
 *   extra bounce before it expires naturally. `skipAuthRedirect` is set by
 *   {@link authRequest}, because a 401 from `/api/auth/login` itself means
 *   "wrong credentials", not "dead session" — there is no session cookie yet
 *   to be dead, and redirecting would wipe the login form before its `catch`
 *   can render a "Sign in failed" toast;
 * - resolves `undefined` for a `204 No Content` response;
 * - otherwise resolves the decoded JSON body.
 */
async function apiFetch<T>(
  url: string,
  options: RequestInit = {},
  { skipAuthRedirect = false }: { skipAuthRedirect?: boolean } = {},
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((options.headers as Record<string, string>) ?? {}),
  }

  const res = await fetch(url, { ...options, headers })

  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: "Unknown error" }))

    if (res.status === 401 && !skipAuthRedirect && typeof window !== "undefined") {
      await fetch("/api/auth/logout", {
        method: "POST",
        signal: AbortSignal.timeout(CLEAR_SESSION_FETCH_TIMEOUT_MS),
      }).catch(() => {
        // Best-effort: even if clearing the cookie fails or times out, still
        // redirect — worst case the middleware bounce-back loop resumes,
        // which is no worse than not attempting the clear at all.
      })
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
 * request to the Go API with the `access_token` cookie converted into an
 * `Authorization: Bearer <token>` header server-side.
 */
export function apiRequest<T>(path: string, options?: RequestInit): Promise<T> {
  return apiFetch<T>(`/api/proxy${path}`, options)
}

/**
 * Calls one of the auth BFF route handlers at `/api/auth${path}`
 * (`login`/`register`/`logout`), which exchange credentials with the Go API
 * and manage the httpOnly session cookie without ever exposing the token to
 * client JavaScript.
 *
 * A `401` from these routes (wrong credentials on login, or a downstream
 * auth failure) is never treated as a dead session — there is no session
 * cookie to have gone dead yet — so the dead-session logout+redirect in
 * {@link apiFetch} is skipped and the `ApiRequestError` propagates to the
 * caller (e.g. so `LoginForm` can render a "Sign in failed" toast).
 */
export function authRequest<T>(path: string, options?: RequestInit): Promise<T> {
  return apiFetch<T>(`/api/auth${path}`, options, { skipAuthRedirect: true })
}
