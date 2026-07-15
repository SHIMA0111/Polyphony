import { expect, test } from "@playwright/test"

/**
 * End-to-end coverage for the Kratos-driven auth flip (Step 30): registers
 * a brand-new user through the rendered Kratos-flow `RegisterForm`, logs
 * out via the UI, logs back in through `LoginForm` with the same
 * credentials, confirms the session survives a reload, and confirms a
 * wrong password is rejected with a toast error without leaving `/login`.
 *
 * Runs against the E2E test-profile stack (`api-e2e`/`web-e2e`, now
 * `AUTH_MODE=kratos`, plus the shared, non-profiled `kratos`/`kratos-db`
 * services), so registration/login genuinely round-trip through Kratos's
 * self-service flows and set its own `ory_kratos_session` cookie — not a
 * mocked network layer.
 *
 * Uses a unique email/username per run (not the shared seed fixture, see
 * `e2e/support/fixtures.ts`) so this spec has no dependency on seed
 * ordering and can run standalone, matching `smoke.spec.ts`'s convention.
 */
test.describe("Kratos-driven auth: registration, login, session persistence, logout", () => {
  test("registers, logs out, logs back in, persists the session across reload, and rejects a wrong password", async ({
    page,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
    const email = `auth-${runId}@polyphony.test`
    // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
    // rejects hyphens, which would otherwise block registration client-side
    // before any request is even sent.
    const username = `auth_${runId}`
    const password = "auth-test-password-123"

    await page.goto("/register")
    await page.getByPlaceholder("you@example.com").fill(email)
    await page.getByPlaceholder("johndoe").fill(username)
    await page.getByPlaceholder("Create a password").fill(password)
    await page.getByPlaceholder("Confirm your password").fill(password)
    await page.getByRole("button", { name: "Create account" }).click()

    await expect(page).toHaveURL(/\/rooms$/)

    // A successful Kratos flow submission sets Kratos's own session cookie
    // on this app's origin — not a bespoke `access_token`.
    const cookiesAfterRegister = await page.context().cookies()
    expect(
      cookiesAfterRegister.some((c) => c.name === "ory_kratos_session"),
    ).toBe(true)

    // Log out via the UI (top-bar account menu -> Log out).
    await page.getByRole("button", { name: "Account menu" }).click()
    await page.getByRole("menuitem", { name: "Log out" }).click()
    await expect(page).toHaveURL(/\/login$/)

    const cookiesAfterLogout = await page.context().cookies()
    expect(
      cookiesAfterLogout.some((c) => c.name === "ory_kratos_session"),
    ).toBe(false)

    // Log back in via the UI with the same credentials.
    await page.getByPlaceholder("you@example.com").fill(email)
    await page.getByPlaceholder("Enter your password").fill(password)
    await page.getByRole("button", { name: "Sign in" }).click()
    await expect(page).toHaveURL(/\/rooms$/)

    // Reloading keeps the session: no bounce back to /login.
    await page.reload()
    await expect(page).toHaveURL(/\/rooms$/)

    // Log out again so the next step exercises a clean login attempt.
    await page.getByRole("button", { name: "Account menu" }).click()
    await page.getByRole("menuitem", { name: "Log out" }).click()
    await expect(page).toHaveURL(/\/login$/)

    // Wrong password: a toast error is shown and the page stays on /login.
    await page.getByPlaceholder("you@example.com").fill(email)
    await page
      .getByPlaceholder("Enter your password")
      .fill("definitely-the-wrong-password")
    await page.getByRole("button", { name: "Sign in" }).click()

    await expect(
      page
        .getByRole("region", { name: /notifications/i })
        .getByText("Sign in failed"),
    ).toBeVisible()
    await expect(page).toHaveURL(/\/login$/)
  })
})
