import { NextResponse, type NextRequest } from "next/server"

import { getKratosSessionFromCookie } from "@/lib/kratos-session"
import { acceptLoginRequest, getLoginRequest } from "@/lib/hydra-admin"

/**
 * Route handlers must not be statically optimized: every request carries a
 * distinct `login_challenge` and (potentially) a distinct Kratos session
 * cookie.
 */
export const dynamic = "force-dynamic"

/**
 * Hydra's login-provider endpoint (`urls.login` in `ory/hydra/hydra.yml`).
 *
 * Hydra redirects the browser here with a `login_challenge` whenever an
 * OAuth2 authorization request needs to establish *who* is logging in.
 * This app has no separate authentication mechanism of its own — Hydra is
 * purely a protocol layer on top of the existing Ory Kratos session
 * (Step 30): this handler resolves the challenge by checking the caller's
 * `ory_kratos_session` cookie and telling Hydra which Kratos identity to
 * bind as the OAuth2 `subject`.
 *
 * Three cases:
 * 1. Hydra reports `skip: true` (it already has a remembered subject for
 *    this challenge, e.g. re-authorizing the same client) — immediately
 *    accept with that subject, no Kratos lookup needed.
 * 2. No active Kratos session — bounce to the plain `/login` page. There is
 *    no `return_to` chaining back into this challenge (see
 *    `docs/tasks/step55.md` "Out of scope"): the user must re-initiate the
 *    OAuth2 authorization request after logging in.
 * 3. An active Kratos session exists — accept the login challenge with the
 *    Kratos identity's UUID as `subject`, remembering the decision for one
 *    hour so a second authorization request against the same client
 *    doesn't require hitting this route again.
 *
 * @param request - The incoming request; the `login_challenge` query
 *   parameter and the `Cookie` header are the only parts read.
 * @returns A redirect to whatever `redirect_to` Hydra (or `/login`)
 *   specifies. Never renders a page.
 */
export async function GET(request: NextRequest): Promise<NextResponse> {
  const loginChallenge = request.nextUrl.searchParams.get("login_challenge")
  if (!loginChallenge) {
    return NextResponse.json(
      { error: "missing login_challenge query parameter" },
      { status: 400 },
    )
  }

  const loginRequest = await getLoginRequest(loginChallenge)

  if (loginRequest.skip) {
    const { redirect_to } = await acceptLoginRequest(loginChallenge, {
      subject: loginRequest.subject,
    })
    return NextResponse.redirect(redirect_to)
  }

  const session = await getKratosSessionFromCookie(request.headers.get("cookie"))

  if (!session) {
    return NextResponse.redirect(new URL("/login", request.url))
  }

  const { redirect_to } = await acceptLoginRequest(loginChallenge, {
    subject: session.identity.id,
    remember: true,
    remember_for: 3600,
    context: { traits: session.identity.traits },
  })

  return NextResponse.redirect(redirect_to)
}
