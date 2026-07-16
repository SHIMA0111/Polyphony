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
 * Core fetch wrapper shared by {@link apiRequest} and {@link authRequest}.
 *
 * Sets `Content-Type: application/json` by default (callers may override via
 * `options.headers`), never sets `Authorization` (the browser never holds a
 * token — that only happens server-side, inside the route handlers), and:
 * - throws {@link ApiRequestError} on any non-2xx response, parsing the JSON
 *   error body (falling back to `{ message: "Unknown error" }` if the body
 *   isn't valid JSON);
 * - on a `401` response, redirects the browser to `/login` (guarded by
 *   `typeof window !== "undefined"` so this is a no-op during SSR) before
 *   throwing, since a 401 means the session cookie is missing or expired;
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
 */
export function authRequest<T>(path: string, options?: RequestInit): Promise<T> {
  return apiFetch<T>(`/api/auth${path}`, options)
}
