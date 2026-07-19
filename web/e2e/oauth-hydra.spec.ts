import { createServer, type Server } from "node:http"
import { expect, test } from "@playwright/test"
import { HYDRA_DEMO_FIXTURE_USER } from "./support/fixtures"

/**
 * End-to-end coverage for the Hydra OAuth2/OIDC authorization-code flow
 * Step 55 shipped (its own verification is a standalone Bun script,
 * `ory/hydra/test-oauth-flow.ts`, run via `task oauth:hydra:test` -- no
 * Playwright spec existed for it before this step). This spec drives the
 * same flow through a real browser instead: a fresh authorization-code
 * request against Hydra's public `/oauth2/auth` endpoint -> the app's
 * Hydra-login/-consent routes
 * (`web/src/app/(auth)/oauth/{login,consent}/route.ts`) -> redirect to the
 * demo client's callback with a `code` -> code exchange at Hydra's public
 * `/oauth2/token`.
 *
 * Deviation from a literal "unauthenticated browser" narrative (documented
 * per `docs/tasks/step56.md`'s Implementation notes / wave-7 planning):
 * `/oauth/login` has no `return_to` chaining back into an in-flight Hydra
 * challenge (Step 55's own documented "Out of scope") -- if the browser hits
 * `/oauth2/auth` before establishing a Kratos session, `/oauth/login` just
 * bounces to the plain `/login` page and the flow dead-ends. So this spec
 * logs in through the rendered Kratos-backed `LoginForm` *first*
 * (establishing the `ory_kratos_session` cookie), using the dedicated
 * `HYDRA_DEMO_FIXTURE_USER` seeded by `e2e/seed/seed.ts` rather than
 * registering a fresh user, and only then starts the Hydra authorization
 * request. With a session already present, `/oauth/login` (and
 * `/oauth/consent`, which auto-grants -- there is no consent UI page, see
 * that route's own doc comment) resolve the challenge immediately with no
 * further user interaction, exactly as Step 55 designed it for its one
 * first-party client.
 *
 * Cookie-scoping note: this app's Hydra login/consent URLs
 * (`URLS_LOGIN`/`URLS_CONSENT` in docker-compose.yml's `hydra-e2e` block)
 * are configured against `http://127.0.0.1:3001`, not `http://localhost:3001`
 * (avoiding IPv6-localhost-shadowing, same as the dev stack -- see
 * `ory/README.md`). Browsers scope host-only cookies (no `Domain`
 * attribute) exactly by hostname, treating `localhost` and `127.0.0.1` as
 * different hosts, so this spec deliberately logs in against an *absolute*
 * `http://127.0.0.1:3001` URL (not a relative path resolved against
 * `playwright.config.ts`'s `localhost`-based `baseURL`) -- otherwise the
 * Kratos session cookie set during login would never be sent when the
 * browser is later redirected to `/oauth/login` on the `127.0.0.1` origin.
 */

/** Host-published base URL of `web-e2e`, used as an absolute origin (see the cookie-scoping note above) rather than relying on `playwright.config.ts`'s `baseURL`. */
const WEB_E2E_BASE_URL = process.env.WEB_E2E_BASE_URL ?? "http://127.0.0.1:3001"

/** Host-published public/admin base URLs of the e2e-only `hydra-e2e` service (docker-compose.yml). */
const HYDRA_E2E_PUBLIC_URL = process.env.HYDRA_E2E_PUBLIC_URL ?? "http://localhost:8096"
const HYDRA_E2E_ADMIN_URL = process.env.HYDRA_E2E_ADMIN_URL ?? "http://localhost:8097"

/**
 * Fixed id/secret for a dedicated demo OAuth2 client this spec provisions
 * against `hydra-e2e`'s admin API (see `ensureDemoClient` below) -- distinct
 * from the dev stack's `HYDRA_DEMO_CLIENT_ID`/`HYDRA_DEMO_CLIENT_SECRET`
 * (`.env`, manually registered via `task oauth:hydra:register-demo-client`),
 * which are never set for the E2E profile.
 */
const DEMO_CLIENT_ID = "e2e-oauth-hydra-demo-client"
const DEMO_CLIENT_SECRET = "e2e-oauth-hydra-demo-client-secret"

/**
 * Registered redirect URI for the demo client, matching
 * `ory/hydra/test-oauth-flow.ts`'s identical `http://localhost:9999/callback`
 * convention. A minimal local stub server (see `beforeAll` below) actually
 * listens here for the duration of the spec: Hydra's final hop to this URI
 * is a *server-side redirect* within one browser navigation, and Playwright's
 * `page.route`/`route.fulfill` cannot fulfill a redirected request -- the
 * browser genuinely connects to this port, so something real must answer
 * (caught live by the wave-7 integration run: with only a `page.route` stub
 * the navigation dies with `net::ERR_CONNECTION_REFUSED`).
 */
const REDIRECT_URI = "http://localhost:9999/callback"
const REDIRECT_PORT = 9999

/** Overall deadline for {@link ensureDemoClient}'s retry loop -- mirrors `e2e/seed/seed.ts`'s `HEALTH_POLL_TIMEOUT_MS`. */
const DEMO_CLIENT_PROVISION_TIMEOUT_MS = 30_000
/** How long each individual provisioning attempt is allowed to hang before its own `AbortSignal.timeout` fires. */
const DEMO_CLIENT_PROVISION_ATTEMPT_TIMEOUT_MS = 5_000
/** Delay between retry attempts -- mirrors `e2e/seed/seed.ts`'s `HEALTH_POLL_INTERVAL_MS`. */
const DEMO_CLIENT_PROVISION_POLL_INTERVAL_MS = 1_000

/** Outcome of a single {@link ensureDemoClient} provisioning attempt. */
type EnsureDemoClientAttempt =
  | { ok: true }
  | { ok: false; transient: boolean; error: Error }

/**
 * A single attempt at `POST /admin/clients`, bounded by
 * `DEMO_CLIENT_PROVISION_ATTEMPT_TIMEOUT_MS` so a stalled connection (e.g.
 * `hydra-e2e` still finishing its own startup) cannot hang the caller
 * indefinitely. A `5xx` response or any network-level failure (connection
 * refused during that same startup race) is reported as transient and worth
 * retrying; any other non-`409` non-`ok` response is a genuine client-config
 * problem that will never succeed on retry, so it is reported as
 * non-transient instead.
 */
async function attemptEnsureDemoClient(): Promise<EnsureDemoClientAttempt> {
  try {
    const res = await fetch(`${HYDRA_E2E_ADMIN_URL}/admin/clients`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        client_id: DEMO_CLIENT_ID,
        client_secret: DEMO_CLIENT_SECRET,
        client_name: "E2E OAuth Hydra Demo Client",
        grant_types: ["authorization_code", "refresh_token"],
        response_types: ["code"],
        redirect_uris: [REDIRECT_URI],
        scope: "openid offline_access profile email",
        token_endpoint_auth_method: "client_secret_basic",
      }),
      signal: AbortSignal.timeout(DEMO_CLIENT_PROVISION_ATTEMPT_TIMEOUT_MS),
    })

    if (res.ok || res.status === 409) {
      return { ok: true }
    }

    const body = await res.text()
    const error = new Error(`failed to provision the e2e demo OAuth2 client: HTTP ${res.status}: ${body}`)
    return { ok: false, transient: res.status >= 500, error }
  } catch (err) {
    const error = err instanceof Error ? err : new Error(String(err))
    return { ok: false, transient: true, error }
  }
}

/**
 * Idempotently registers the fixed demo OAuth2 client against `hydra-e2e`'s
 * admin API (`POST /admin/clients`), so this spec never depends on the
 * dev-only, manual `task oauth:hydra:register-demo-client` step. A `409`
 * (client id already exists, e.g. from a previous run against a stack that
 * was never torn down) is treated as success, same idempotency convention
 * as `e2e/seed/seed.ts`'s fixture-user/fixture-room helpers.
 *
 * Retries transient failures (see {@link attemptEnsureDemoClient}) until
 * `DEMO_CLIENT_PROVISION_TIMEOUT_MS` has elapsed, mirroring `e2e/seed/
 * seed.ts`'s `waitForHealth` deadline-poll pattern -- `test:e2e:up`'s
 * detached Compose startup gives no guarantee `hydra-e2e` is actually ready
 * to serve its admin API the instant this spec's `beforeAll` runs. A
 * non-transient response is thrown immediately rather than retried.
 */
async function ensureDemoClient(): Promise<void> {
  const deadline = Date.now() + DEMO_CLIENT_PROVISION_TIMEOUT_MS
  let lastError: unknown

  while (Date.now() < deadline) {
    const result = await attemptEnsureDemoClient()
    if (result.ok) return
    if (!result.transient) throw result.error
    lastError = result.error
    await new Promise((resolve) => setTimeout(resolve, DEMO_CLIENT_PROVISION_POLL_INTERVAL_MS))
  }

  throw new Error(
    `failed to provision the e2e demo OAuth2 client within ${DEMO_CLIENT_PROVISION_TIMEOUT_MS}ms: ${String(lastError)}`,
  )
}

test.describe("Hydra OAuth2/OIDC authorization-code flow (Step 55, browser-level regression)", () => {
  let callbackServer: Server

  test.beforeAll(async () => {
    await ensureDemoClient()
    // See REDIRECT_URI's doc comment: a real listener is required because
    // the browser is server-redirected here and route interception cannot
    // fulfill a redirect hop.
    callbackServer = createServer((_req, res) => {
      res.writeHead(200, { "Content-Type": "text/html" })
      res.end("<html><body>oauth-hydra.spec.ts callback stub</body></html>")
    })
    await new Promise<void>((resolve, reject) => {
      callbackServer.once("error", reject)
      callbackServer.listen(REDIRECT_PORT, resolve)
    })
  })

  test.afterAll(async () => {
    // If `beforeAll` threw before `callbackServer` was ever assigned (e.g.
    // `ensureDemoClient` failed), there is nothing listening to close --
    // `close()`'s callback only fires once a listening server has actually
    // shut down, so calling it on an unassigned server would leave this
    // Promise pending forever.
    if (!callbackServer) return
    await new Promise<void>((resolve) => {
      callbackServer.close(() => resolve())
    })
  })

  test("logs in, completes the authorization-code round trip, and exchanges the code for tokens", async ({
    page,
    request,
  }) => {
    // Log in via the rendered UI *first* (see this file's header comment)
    // against the absolute 127.0.0.1 origin so the resulting Kratos session
    // cookie is scoped to the same host `hydra-e2e`'s login/consent URLs
    // redirect back to.
    await page.goto(`${WEB_E2E_BASE_URL}/login`)
    await page.getByPlaceholder("you@example.com").fill(HYDRA_DEMO_FIXTURE_USER.email)
    await page.getByPlaceholder("Enter your password").fill(HYDRA_DEMO_FIXTURE_USER.password)
    await page.getByRole("button", { name: "Sign in" }).click()
    await expect(page).toHaveURL(/\/rooms$/)

    const state = `e2e-state-${Date.now()}`
    const authorizeUrl = new URL(`${HYDRA_E2E_PUBLIC_URL}/oauth2/auth`)
    authorizeUrl.searchParams.set("client_id", DEMO_CLIENT_ID)
    authorizeUrl.searchParams.set("response_type", "code")
    authorizeUrl.searchParams.set("redirect_uri", REDIRECT_URI)
    authorizeUrl.searchParams.set("scope", "openid offline_access profile email")
    authorizeUrl.searchParams.set("state", state)

    // Drives the full hop chain in one navigation: Hydra -> this app's
    // `/oauth/login` (accepts immediately -- a Kratos session already
    // exists) -> Hydra -> this app's `/oauth/consent` (auto-grants, no
    // consent UI page) -> Hydra -> the stub callback server above.
    await page.goto(authorizeUrl.toString())
    expect(page.url().startsWith(REDIRECT_URI)).toBe(true)

    const callbackUrl = new URL(page.url())
    const code = callbackUrl.searchParams.get("code")
    const returnedState = callbackUrl.searchParams.get("state")

    expect(code).toBeTruthy()
    expect(returnedState).toBe(state)

    // Code-for-token exchange via Playwright's `request` fixture (not the
    // browser), against Hydra's public `/oauth2/token` endpoint, using HTTP
    // Basic client authentication (matching the demo client's registered
    // `client_secret_basic` token-endpoint auth method).
    const basicAuth = Buffer.from(`${DEMO_CLIENT_ID}:${DEMO_CLIENT_SECRET}`).toString("base64")
    const tokenRes = await request.post(`${HYDRA_E2E_PUBLIC_URL}/oauth2/token`, {
      headers: {
        Authorization: `Basic ${basicAuth}`,
      },
      form: {
        grant_type: "authorization_code",
        code: code!,
        redirect_uri: REDIRECT_URI,
      },
    })

    expect(tokenRes.ok()).toBe(true)
    const tokens = (await tokenRes.json()) as { access_token?: string; id_token?: string }
    expect(tokens.access_token).toBeTruthy()
    // `openid` scope was requested, so an ID token must also be issued.
    expect(tokens.id_token).toBeTruthy()
  })
})
