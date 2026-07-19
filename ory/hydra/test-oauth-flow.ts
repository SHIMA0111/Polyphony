#!/usr/bin/env bun
/**
 * End-to-end verification of the Hydra OAuth2/OIDC authorization-code flow
 * (Step 55), run via `task oauth:hydra:test`.
 *
 * A self-contained Bun script — deliberately not a test-framework spec, so
 * it can be run standalone against a live `docker compose up` dev stack
 * (ports 3000/4444/4445) with a single command and a plain process exit
 * code, matching this project's bun-first tooling preference for one-off
 * verification scripts (see `web/e2e/seed/cli.ts` for the same pattern).
 *
 * What it does, in order:
 *   1. Registers a brand-new throwaway Kratos identity and establishes a
 *      session cookie via the web app's `/api/kratos/*` proxy (the same
 *      `sessions/whoami`-adjacent self-service contract the real
 *      `RegisterForm.tsx` uses).
 *   2. Requests Hydra's `/oauth2/auth` authorization endpoint for the
 *      registered demo client and follows the resulting redirect chain
 *      (Hydra -> this app's `/oauth/login` -> Hydra -> this app's
 *      `/oauth/consent` -> Hydra -> the demo client's redirect URI) one hop
 *      at a time, until a `Location` header pointing at the demo client's
 *      redirect URI is observed.
 *   3. Parses `code`/`state` off that final redirect and asserts `state`
 *      round-tripped unchanged.
 *   4. Exchanges the code for tokens at `/oauth2/token` (HTTP Basic client
 *      auth).
 *   5. Calls `/userinfo` with the issued access token and asserts the
 *      returned `email` claim matches the throwaway user registered in
 *      step 1.
 *
 * Bun's `fetch` (like Node's) does not maintain a cookie jar across
 * requests the way a browser does — {@link CookieJar} below reads every
 * `Set-Cookie` response header (there may be several per hop: this app's
 * Kratos session cookie, Kratos's own flow CSRF cookie, and Hydra's
 * internal login/consent-challenge session cookie) and re-attaches them as
 * a single `Cookie` request header on every subsequent request, the same
 * technique `curl -c/-b cookiejar` uses. Cookies are tracked by name only
 * (not scoped per-origin): every cookie involved here is set without a
 * `Domain` attribute against a bare `localhost` host differing only by
 * port, and browsers do not scope cookies by port, so a single flat jar
 * matches real browser behavior.
 *
 * Exits non-zero with a clear message identifying which numbered step
 * failed. Safely repeatable: every run registers a fresh throwaway user, so
 * no manual cleanup is required between runs.
 */

const HYDRA_PUBLIC_URL = process.env.HYDRA_PUBLIC_URL ?? "http://localhost:4444"
const HYDRA_DEMO_CLIENT_ID = process.env.HYDRA_DEMO_CLIENT_ID
const HYDRA_DEMO_CLIENT_SECRET = process.env.HYDRA_DEMO_CLIENT_SECRET
// 127.0.0.1, not localhost: on a host where something else is already
// listening on [::1]:3000 (localhost's IPv6 resolution), a bare
// "localhost:3000" default would silently hit that unrelated service
// instead of the Docker-published web container bound to the IPv4 loopback
// (see ory/README.md's "IPv6 localhost shadowing" note).
const WEB_BASE_URL = process.env.WEB_BASE_URL ?? "http://127.0.0.1:3000"

/** The demo client's registered redirect URI (see `task oauth:hydra:register-demo-client`). No real server needs to listen here — the flow never actually reaches it, it is only ever observed as a `Location` header value. */
const REDIRECT_URI = "http://localhost:9999/callback"

/** Maximum number of redirect hops to follow before concluding the flow is stuck (a generous bound — a real run takes 3-5 hops). */
const MAX_REDIRECT_HOPS = 10

/** Prints an error message prefixed for this script and exits with a non-zero status. */
function fail(message: string): never {
  console.error(`[test-oauth-flow] ERROR: ${message}`)
  process.exit(1)
}

if (!HYDRA_DEMO_CLIENT_ID || !HYDRA_DEMO_CLIENT_SECRET) {
  fail(
    "HYDRA_DEMO_CLIENT_ID and HYDRA_DEMO_CLIENT_SECRET must be set. Run " +
      "`task oauth:hydra:register-demo-client` and copy the printed " +
      "client_id/client_secret into .env, then re-run `task oauth:hydra:test`.",
  )
}

/**
 * Minimal manual cookie jar. Bun/Node's `fetch` does not persist cookies
 * across requests the way a browser does, so every hop in this script reads
 * response `Set-Cookie` headers into this jar and re-serializes them onto
 * the next request's `Cookie` header by hand.
 */
class CookieJar {
  private readonly cookies = new Map<string, string>()

  /**
   * Records every `Set-Cookie` header from a response. A later value for
   * the same cookie name overwrites an earlier one, matching how a real
   * cookie jar handles a server re-issuing the same cookie.
   *
   * @param res - The response to read `Set-Cookie` headers from.
   */
  absorb(res: Response): void {
    for (const setCookie of res.headers.getSetCookie()) {
      const pair = setCookie.split(";")[0]
      const eq = pair.indexOf("=")
      if (eq === -1) continue
      this.cookies.set(pair.slice(0, eq).trim(), pair.slice(eq + 1).trim())
    }
  }

  /** Serializes every currently-held cookie as a single `Cookie` request header value. */
  header(): string {
    return [...this.cookies.entries()].map(([name, value]) => `${name}=${value}`).join("; ")
  }
}

/** The subset of a Kratos self-service flow's `ui` container this script reads. */
interface KratosUiContainer {
  action: string
  nodes: Array<{ attributes: { name?: string; value?: unknown } }>
}

/** Reads a named hidden field's value (e.g. `csrf_token`) out of a Kratos flow's `ui.nodes`. */
function getNodeValue(ui: KratosUiContainer, name: string): string | undefined {
  const node = ui.nodes.find((n) => n.attributes.name === name)
  const value = node?.attributes.value
  return typeof value === "string" ? value : undefined
}

/**
 * Rewrites an absolute Kratos `ui.action` into a same-origin path through
 * this app's `/api/kratos/*` proxy, mirroring
 * `web/src/features/auth/utils/kratos-flow.ts`'s `toRelativeKratosAction` —
 * Kratos returns `ui.action` as an absolute URL against its own
 * `serve.public.base_url`, which this script (like a real browser) reaches
 * only through the web app's same-origin proxy.
 */
function toProxiedKratosAction(action: string): string {
  const url = new URL(action, WEB_BASE_URL)
  return `${WEB_BASE_URL}/api/kratos${url.pathname}${url.search}`
}

/**
 * Step 1: registers a brand-new throwaway Kratos identity via the web app's
 * `/api/kratos/*` proxy and establishes its session cookie in `jar`.
 *
 * @param jar - The cookie jar to record the registration flow's CSRF cookie
 *   and the resulting Kratos session cookie into.
 * @returns The throwaway identity's registered email (used in step 5 to
 *   verify `/userinfo`'s returned claim).
 */
async function registerThrowawayUser(jar: CookieJar): Promise<string> {
  const unique = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
  const email = `oauth-flow-test-${unique}@example.com`
  const username = `oauth_flow_test_${unique}`
  // Deliberately unrelated to the email/username: Kratos's password policy
  // rejects passwords "too similar to the identifier" (error 4000031), so the
  // password must not embed the same `oauth-flow-test-${unique}` stem.
  const password = `Zx9!vQ${Math.random().toString(36).slice(2, 12)}#${Date.now() % 100000}`

  console.log(`[1/5] Registering throwaway Kratos user ${email} ...`)

  const flowRes = await fetch(`${WEB_BASE_URL}/api/kratos/self-service/registration/browser`, {
    headers: { Accept: "application/json" },
  })
  jar.absorb(flowRes)
  if (!flowRes.ok) {
    fail(`step 1: fetching the registration flow failed: HTTP ${flowRes.status}`)
  }
  const flow = (await flowRes.json()) as { id: string; ui: KratosUiContainer }
  const csrfToken = getNodeValue(flow.ui, "csrf_token")
  if (!csrfToken) {
    fail("step 1: registration flow response carried no csrf_token node")
  }

  const submitRes = await fetch(toProxiedKratosAction(flow.ui.action), {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      Cookie: jar.header(),
    },
    body: JSON.stringify({
      method: "password",
      password,
      csrf_token: csrfToken,
      traits: { email, username },
    }),
  })
  jar.absorb(submitRes)
  if (!submitRes.ok) {
    const body = await submitRes.text()
    fail(`step 1: submitting registration failed: HTTP ${submitRes.status}: ${body}`)
  }
  const result = (await submitRes.json()) as { session?: unknown; identity?: { id?: string } }
  if (!result.session) {
    fail(
      "step 1: registration succeeded but no session was returned (check " +
        "ory/kratos/kratos.yml's selfservice.flows.registration.after hooks)",
    )
  }

  console.log(`      registered identity ${result.identity?.id} and established a Kratos session`)
  return email
}

/**
 * Step 2-3: drives Hydra's `/oauth2/auth` authorization request for the
 * registered demo client and follows the login/consent redirect chain one
 * hop at a time until the demo client's redirect URI is observed.
 *
 * @param jar - The cookie jar carrying the Kratos session cookie from step
 *   1 (needed by `/oauth/login`) and accumulating Hydra's own
 *   login/consent-challenge session cookies along the way.
 * @returns The authorization code and the `state` value Hydra echoed back.
 */
async function runAuthorizationCodeFlow(jar: CookieJar): Promise<{ code: string; state: string }> {
  const state = Math.random().toString(36).slice(2, 15)

  const authorizeUrl = new URL(`${HYDRA_PUBLIC_URL}/oauth2/auth`)
  authorizeUrl.searchParams.set("client_id", HYDRA_DEMO_CLIENT_ID!)
  authorizeUrl.searchParams.set("response_type", "code")
  authorizeUrl.searchParams.set("scope", "openid offline_access profile email")
  authorizeUrl.searchParams.set("redirect_uri", REDIRECT_URI)
  authorizeUrl.searchParams.set("state", state)

  console.log("[2/5] Requesting Hydra's /oauth2/auth and following the login/consent redirect chain ...")

  let currentUrl = authorizeUrl.toString()

  for (let hop = 1; hop <= MAX_REDIRECT_HOPS; hop++) {
    const res = await fetch(currentUrl, {
      redirect: "manual",
      headers: { Cookie: jar.header() },
    })
    jar.absorb(res)

    const location = res.headers.get("location")
    if (!location) {
      const body = await res.text().catch(() => "<unreadable body>")
      fail(
        `step 2: expected a redirect at hop ${hop} (${currentUrl}) but got ` +
          `HTTP ${res.status} with no Location header. Body: ${body.slice(0, 500)}`,
      )
    }

    const resolved = new URL(location, currentUrl)

    if (resolved.origin === "http://localhost:9999" && resolved.pathname === "/callback") {
      const code = resolved.searchParams.get("code")
      const returnedState = resolved.searchParams.get("state")
      if (!code) {
        fail(`step 3: final redirect to ${REDIRECT_URI} carried no 'code' query parameter (${resolved.toString()})`)
      }
      if (returnedState !== state) {
        fail(`step 3: state mismatch: sent "${state}", got back "${returnedState}"`)
      }
      console.log(`      authorization code obtained after ${hop} redirect hop(s); state round-tripped correctly`)
      return { code, state: returnedState! }
    }

    console.log(`      hop ${hop}: -> ${resolved.origin}${resolved.pathname}`)
    currentUrl = resolved.toString()
  }

  fail(`step 2: redirect chain did not reach ${REDIRECT_URI} within ${MAX_REDIRECT_HOPS} hops`)
}

/**
 * Step 4: exchanges an authorization code for tokens at `/oauth2/token`
 * using HTTP Basic client authentication (`client_secret_basic`, matching
 * the demo client's registered token-endpoint auth method).
 *
 * @param code - The authorization code obtained in step 2-3.
 * @returns The issued access token.
 */
async function exchangeCodeForTokens(code: string): Promise<string> {
  console.log("[4/5] Exchanging the authorization code for tokens at /oauth2/token ...")

  const basicAuth = Buffer.from(`${HYDRA_DEMO_CLIENT_ID}:${HYDRA_DEMO_CLIENT_SECRET}`).toString("base64")
  const res = await fetch(`${HYDRA_PUBLIC_URL}/oauth2/token`, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Authorization: `Basic ${basicAuth}`,
    },
    body: new URLSearchParams({
      grant_type: "authorization_code",
      code,
      redirect_uri: REDIRECT_URI,
    }),
  })

  if (!res.ok) {
    const body = await res.text()
    fail(`step 4: token exchange failed: HTTP ${res.status}: ${body}`)
  }

  const tokens = (await res.json()) as { access_token?: string }
  if (!tokens.access_token) {
    fail("step 4: token response carried no access_token")
  }

  console.log("      tokens issued")
  return tokens.access_token
}

/**
 * Step 5: calls `/userinfo` with the issued access token and asserts the
 * returned `email` claim matches the throwaway user registered in step 1.
 *
 * @param accessToken - The access token obtained in step 4.
 * @param expectedEmail - The throwaway user's registered email.
 */
async function verifyUserinfo(accessToken: string, expectedEmail: string): Promise<void> {
  console.log("[5/5] Calling /userinfo and verifying the returned email claim ...")

  const res = await fetch(`${HYDRA_PUBLIC_URL}/userinfo`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })

  if (!res.ok) {
    const body = await res.text()
    fail(`step 5: /userinfo failed: HTTP ${res.status}: ${body}`)
  }

  const claims = (await res.json()) as { email?: string }
  if (claims.email !== expectedEmail) {
    fail(`step 5: /userinfo email mismatch: expected "${expectedEmail}", got "${claims.email}"`)
  }

  console.log(`      /userinfo email matches: ${claims.email}`)
}

async function main(): Promise<void> {
  const jar = new CookieJar()
  const email = await registerThrowawayUser(jar)
  const { code } = await runAuthorizationCodeFlow(jar)
  const accessToken = await exchangeCodeForTokens(code)
  await verifyUserinfo(accessToken, email)
  console.log(`\nSUCCESS: Hydra authorization-code flow verified end to end for ${email}.`)
}

main().catch((err) => {
  fail(`unexpected failure: ${err instanceof Error ? err.stack ?? err.message : String(err)}`)
})
