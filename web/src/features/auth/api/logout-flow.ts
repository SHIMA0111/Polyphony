import { toRelativeKratosAction } from "../utils/kratos-flow"

/** Shape of `GET /self-service/logout/browser`'s response body. */
interface LogoutFlowResponse {
  logout_url: string
}

/**
 * Drives Kratos's self-service logout flow: fetches a logout URL via
 * `GET /api/kratos/self-service/logout/browser` (the session cookie is
 * forwarded automatically since this proxy is same-origin), then follows it
 * via `GET toRelativeKratosAction(logout_url)`, which clears the session
 * cookie through Kratos's own `Set-Cookie` response.
 *
 * A missing/expired session (`401`/`404` from the first call) is treated as
 * already-logged-out rather than an error, since that is functionally the
 * desired end state of calling this function at all.
 *
 * @throws If the first call fails with any other status, or the follow-up
 *   `GET` to `logout_url` fails.
 */
export async function logout(): Promise<void> {
  const res = await fetch("/api/kratos/self-service/logout/browser", {
    headers: { Accept: "application/json" },
  })

  if (res.status === 401 || res.status === 404) {
    return
  }

  if (!res.ok) {
    throw new Error(`Failed to start logout flow: HTTP ${res.status}`)
  }

  const { logout_url } = (await res.json()) as LogoutFlowResponse

  const logoutRes = await fetch(toRelativeKratosAction(logout_url), {
    headers: { Accept: "application/json" },
  })

  if (
    !logoutRes.ok &&
    logoutRes.status !== 401 &&
    logoutRes.status !== 404
  ) {
    throw new Error(`Logout failed: HTTP ${logoutRes.status}`)
  }
}
