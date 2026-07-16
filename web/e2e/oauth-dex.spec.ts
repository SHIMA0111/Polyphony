import { expect, type Page, test } from "@playwright/test"
import { DEX_FIXTURE_USER } from "./support/fixtures"

/**
 * End-to-end coverage for OAuth social login via Kratos's `oidc` method
 * (Step 44), driven against dex — the mock OIDC provider standing in for a
 * real Google/GitHub app (see `ory/dex/config.yaml`, `ory/README.md`'s
 * "Social login (OIDC)" section).
 *
 * Runs against the E2E test-profile stack, same as `auth.spec.ts`. Unlike
 * that spec, this one leaves the E2E-only Compose network at all — dex's
 * OIDC authorization/callback redirects are real, full-page browser
 * navigations to a *different origin* (`dex:5556`, mapped to
 * `127.0.0.1:5556` by `playwright.config.ts`'s `--host-resolver-rules`
 * flag) and then back to `kratos-e2e`'s own dedicated host port (`8093`;
 * see docker-compose.yml's `kratos-e2e` block for why it needs one, unlike
 * every other E2E-only service).
 *
 * Both cases share the same dex fixture identity (`DEX_FIXTURE_USER`,
 * `ory/dex/config.yaml`'s one `staticPasswords` entry) and are asserted
 * against the *same*, persistent Kratos identity store, so they run
 * `serial` — not `fullyParallel`, this file's one exception — to keep the
 * "exactly one identity" assertion in the second case meaningful regardless
 * of worker scheduling.
 */
test.describe.serial("OAuth social login via dex (Step 44)", () => {
  /**
   * Kratos-e2e's admin API, published on its own dedicated host port so
   * this spec (running on the host, not inside the Compose network) can
   * confirm identity state directly — see docker-compose.yml's `kratos-e2e`
   * block and its comment on why this port exists only for Step 44.
   */
  const kratosE2eAdminUrl = process.env.KRATOS_E2E_ADMIN_URL ?? "http://localhost:8094"

  /**
   * Drives dex's static-password login form to completion from a given
   * entry point, asserting the browser lands authenticated on `/rooms`.
   *
   * @param page - The Playwright page (a fresh, isolated context per `test`).
   * @param entryPath - `/register` or `/login` — both render
   *   `SocialLoginButtons` identically; Kratos itself decides whether this
   *   is a first-time sign-up or a returning sign-in based on whether an
   *   identity already exists for dex's `(provider, subject)` pair, not on
   *   which page initiated the flow.
   */
  async function loginWithDex(page: Page, entryPath: "/register" | "/login") {
    await page.goto(entryPath)
    await page.getByRole("button", { name: "Continue with Dex" }).click()

    // dex's own local-connector login page (not this app's UI) — see
    // ory/dex/config.yaml's `staticPasswords`. Its <label for="userid">
    // doesn't actually match the <input id="login">, so this locates by
    // placeholder rather than accessible label.
    await page.getByPlaceholder("email address").fill(DEX_FIXTURE_USER.email)
    await page.getByPlaceholder("password").fill(DEX_FIXTURE_USER.password)
    await page.getByRole("button", { name: "Login" }).click()

    // dex (skipApprovalScreen: true, no consent screen) redirects straight
    // back to Kratos's OIDC callback, which redirects back into this app.
    await expect(page).toHaveURL(/\/rooms$/)
  }

  test("a brand-new dex sign-in creates an identity and lands authenticated on /rooms", async ({ page }) => {
    await loginWithDex(page, "/register")

    // A real Kratos session was established (not a mocked network layer),
    // matching auth.spec.ts's convention for asserting this.
    const cookies = await page.context().cookies()
    expect(cookies.some((cookie) => cookie.name === "ory_kratos_session")).toBe(true)
  })

  test("repeating the dex login from a fresh session reuses the same account, not a duplicate", async ({ page }) => {
    // Playwright gives every `test()` its own isolated context/cookie jar
    // already — this genuinely is a brand-new browser session, dex
    // included, with no carried-over dex or Kratos cookies from the first
    // case.
    await loginWithDex(page, "/login")

    const res = await fetch(`${kratosE2eAdminUrl}/admin/identities`)
    expect(res.ok).toBe(true)
    const identities = (await res.json()) as Array<{
      id: string
      traits: { email: string }
    }>

    const matches = identities.filter((identity) => identity.traits.email === DEX_FIXTURE_USER.email)
    // Confirms Kratos matched the second dex login to the *same* identity
    // (by dex's stable (provider, subject) pair) rather than creating a
    // duplicate — the core assertion of this spec.
    expect(matches).toHaveLength(1)

    // Confirms the identity actually has an `oidc` credential (i.e. the
    // Jsonnet mapper ran and this identity was genuinely created via dex,
    // not some unrelated password registration that happens to share the
    // fixture email).
    const detailRes = await fetch(
      `${kratosE2eAdminUrl}/admin/identities/${matches[0].id}?include_credential=oidc`,
    )
    const detail = (await detailRes.json()) as { credentials?: Record<string, unknown> }
    expect(Object.keys(detail.credentials ?? {})).toContain("oidc")
  })
})
