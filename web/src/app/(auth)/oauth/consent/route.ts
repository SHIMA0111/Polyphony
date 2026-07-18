import { NextResponse, type NextRequest } from "next/server"

import { getKratosSessionFromCookie } from "@/lib/kratos-session"
import { acceptConsentRequest, getConsentRequest } from "@/lib/hydra-admin"

/**
 * Route handlers must not be statically optimized: every request carries a
 * distinct `consent_challenge`.
 */
export const dynamic = "force-dynamic"

/**
 * Hydra's consent-provider endpoint (`urls.consent` in
 * `ory/hydra/hydra.yml`).
 *
 * Hydra redirects the browser here (immediately after `/oauth/login`
 * accepts a login challenge) with a `consent_challenge`, asking this app
 * which scopes/audiences to grant the requesting client.
 *
 * No interactive consent screen is rendered: the only client registered
 * against this Hydra instance is this app's own first-party demo client
 * (`task oauth:hydra:register-demo-client`), so there is no untrusted third
 * party whose scope grant a human needs to review (see
 * `docs/tasks/step55.md` "Out of scope" and "Implementation notes" for the
 * rationale — a real per-scope consent UI is a future concern if/when
 * third-party clients are supported). Every requested scope and audience is
 * granted unconditionally, and — when the underlying Kratos session is
 * available — `traits.email`/`traits.username` are attached to the issued
 * ID token so downstream clients can read them via `/userinfo` without a
 * second round-trip.
 *
 * @param request - The incoming request; the `consent_challenge` query
 *   parameter and the `Cookie` header are the only parts read.
 * @returns A redirect to whatever `redirect_to` Hydra specifies (back to
 *   the OAuth2 client's redirect URI, code in hand). Never renders a page.
 */
export async function GET(request: NextRequest): Promise<NextResponse> {
  const consentChallenge = request.nextUrl.searchParams.get("consent_challenge")
  if (!consentChallenge) {
    return NextResponse.json(
      { error: "missing consent_challenge query parameter" },
      { status: 400 },
    )
  }

  const consentRequest = await getConsentRequest(consentChallenge)

  if (consentRequest.skip) {
    const { redirect_to } = await acceptConsentRequest(consentChallenge, {
      grant_scope: consentRequest.requested_scope,
      grant_access_token_audience: consentRequest.requested_access_token_audience,
    })
    return NextResponse.redirect(redirect_to)
  }

  const session = await getKratosSessionFromCookie(request.headers.get("cookie"))

  // Only attach traits when the Kratos session actually belongs to the
  // subject Hydra is requesting consent for. A stale or unrelated session
  // cookie must not leak another identity's claims into this token; treat
  // a mismatch exactly like no session (claims omitted, consent still
  // accepted with Hydra's authoritative subject).
  const matchingSession =
    session?.identity.id === consentRequest.subject ? session : undefined

  const { redirect_to } = await acceptConsentRequest(consentChallenge, {
    grant_scope: consentRequest.requested_scope,
    grant_access_token_audience: consentRequest.requested_access_token_audience,
    session: {
      id_token: matchingSession
        ? {
            email: matchingSession.identity.traits.email,
            preferred_username: matchingSession.identity.traits.username,
          }
        : undefined,
    },
    remember: true,
    remember_for: 3600,
  })

  return NextResponse.redirect(redirect_to)
}
