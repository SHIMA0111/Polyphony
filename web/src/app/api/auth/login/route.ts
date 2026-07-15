import { forwardAuthRequest } from "../_forward-auth"

/**
 * `POST /api/auth/login` — exchanges credentials for a session by forwarding
 * the request body verbatim to the Go API's `POST /auth/login` and, on
 * success, setting the httpOnly `access_token` cookie. See
 * {@link forwardAuthRequest} for the full behavior.
 */
export async function POST(request: Request) {
  return forwardAuthRequest(request, "/auth/login")
}
