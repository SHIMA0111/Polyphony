import "server-only"

/**
 * Base URL of Hydra's admin API, read server-side only (never exposed as a
 * `NEXT_PUBLIC_*` variable — the admin API can mint arbitrary sessions and
 * must never be reachable from a browser). Container-internal by default
 * (`http://hydra:4445`), matching how `KRATOS_ADMIN_URL` is scoped for the
 * Go API (see `.env.example`).
 */
const HYDRA_ADMIN_URL = process.env.HYDRA_ADMIN_URL ?? "http://hydra:4445"

/**
 * Upper bound, in milliseconds, on each {@link hydraAdminRequest} call. A
 * stalled Hydra admin API (rather than a clean connection error) would
 * otherwise hang the login/consent Route Handlers indefinitely.
 */
const HYDRA_ADMIN_REQUEST_TIMEOUT_MS = 5000

/**
 * Shape of Hydra's `GET /admin/oauth2/auth/requests/login` response,
 * limited to the fields `web/src/app/(auth)/oauth/login/route.ts` reads.
 */
export interface HydraLoginRequest {
  /** `true` if Hydra already knows the subject for this challenge (e.g. an active Hydra login session) and no fresh authentication is needed. */
  skip: boolean
  /** The subject Hydra already associated with this challenge, only meaningful when `skip` is `true`. */
  subject: string
}

/**
 * Shape of Hydra's `GET /admin/oauth2/auth/requests/consent` response,
 * limited to the fields `web/src/app/(auth)/oauth/consent/route.ts` reads.
 */
export interface HydraConsentRequest {
  /** `true` if Hydra already has a remembered consent decision for this subject/client/scope combination. */
  skip: boolean
  /** The subject this consent challenge is being requested for. */
  subject: string
  /** The scopes the client is requesting. */
  requested_scope: string[]
  /** The token audiences the client is requesting. */
  requested_access_token_audience: string[]
}

/** Shape common to both accept-login and accept-consent responses. */
export interface HydraAcceptResponse {
  /** The URL the caller must redirect the end user's browser to next. */
  redirect_to: string
}

/** Request body for `PUT /admin/oauth2/auth/requests/login/accept`. */
export interface AcceptLoginRequestBody {
  subject: string
  remember?: boolean
  remember_for?: number
  context?: Record<string, unknown>
}

/** Request body for `PUT /admin/oauth2/auth/requests/consent/accept`. */
export interface AcceptConsentRequestBody {
  grant_scope: string[]
  grant_access_token_audience: string[]
  session?: {
    access_token?: Record<string, unknown>
    id_token?: Record<string, unknown>
  }
  remember?: boolean
  remember_for?: number
}

/**
 * Performs a JSON request against Hydra's admin API and returns the parsed
 * body, throwing on any non-2xx response so callers (the Route Handlers)
 * can propagate a clear error instead of silently mishandling a malformed
 * or nonexistent challenge.
 *
 * @param path - Path (including query string) relative to `HYDRA_ADMIN_URL`.
 * @param init - Optional `fetch` overrides (method, body).
 * @returns The parsed JSON response body.
 * @throws If Hydra responds with a non-2xx status, times out after
 *   {@link HYDRA_ADMIN_REQUEST_TIMEOUT_MS}, or is otherwise unreachable.
 */
async function hydraAdminRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${HYDRA_ADMIN_URL}${path}`, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
    cache: "no-store",
    signal: AbortSignal.timeout(HYDRA_ADMIN_REQUEST_TIMEOUT_MS),
  })

  if (!res.ok) {
    const body = await res.text().catch(() => "<failed to read response body>")
    throw new Error(
      `Hydra admin API request failed: ${init?.method ?? "GET"} ${path} -> HTTP ${res.status}: ${body}`,
    )
  }

  return (await res.json()) as T
}

/**
 * Fetches an in-flight Hydra login challenge.
 *
 * @param challenge - The `login_challenge` query parameter Hydra redirected
 *   the browser to `urls.login` with.
 * @returns The login request Hydra has recorded for this challenge.
 * @throws If the challenge is unknown/expired or Hydra is unreachable.
 */
export function getLoginRequest(challenge: string): Promise<HydraLoginRequest> {
  return hydraAdminRequest<HydraLoginRequest>(
    `/admin/oauth2/auth/requests/login?login_challenge=${encodeURIComponent(challenge)}`,
  )
}

/**
 * Accepts a Hydra login challenge, binding it to a Kratos identity as the
 * OAuth2 `subject`.
 *
 * @param challenge - The `login_challenge` to accept.
 * @param body - The subject (and optional remember/context) to record.
 * @returns Hydra's response, whose `redirect_to` the caller must redirect
 *   the browser to next (typically back into Hydra's own `/oauth2/auth`).
 * @throws If the challenge is unknown/expired/already handled.
 */
export function acceptLoginRequest(
  challenge: string,
  body: AcceptLoginRequestBody,
): Promise<HydraAcceptResponse> {
  return hydraAdminRequest<HydraAcceptResponse>(
    `/admin/oauth2/auth/requests/login/accept?login_challenge=${encodeURIComponent(challenge)}`,
    { method: "PUT", body: JSON.stringify(body) },
  )
}

/**
 * Fetches an in-flight Hydra consent challenge.
 *
 * @param challenge - The `consent_challenge` query parameter Hydra
 *   redirected the browser to `urls.consent` with.
 * @returns The consent request Hydra has recorded for this challenge,
 *   including the scopes/audiences the client requested.
 * @throws If the challenge is unknown/expired or Hydra is unreachable.
 */
export function getConsentRequest(challenge: string): Promise<HydraConsentRequest> {
  return hydraAdminRequest<HydraConsentRequest>(
    `/admin/oauth2/auth/requests/consent?consent_challenge=${encodeURIComponent(challenge)}`,
  )
}

/**
 * Accepts a Hydra consent challenge, granting the requested scopes/
 * audiences and optionally attaching extra claims to the issued
 * access/ID tokens.
 *
 * @param challenge - The `consent_challenge` to accept.
 * @param body - The scopes/audiences to grant and any extra token claims.
 * @returns Hydra's response, whose `redirect_to` the caller must redirect
 *   the browser to next (back to the OAuth2 client's redirect URI, code in
 *   hand).
 * @throws If the challenge is unknown/expired/already handled.
 */
export function acceptConsentRequest(
  challenge: string,
  body: AcceptConsentRequestBody,
): Promise<HydraAcceptResponse> {
  return hydraAdminRequest<HydraAcceptResponse>(
    `/admin/oauth2/auth/requests/consent/accept?consent_challenge=${encodeURIComponent(challenge)}`,
    { method: "PUT", body: JSON.stringify(body) },
  )
}
