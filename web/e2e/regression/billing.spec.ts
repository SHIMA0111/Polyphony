import { expect, test, type Page } from "@playwright/test"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because `web/package.json` has no
// `"type": "module"` — see `attachments.spec.ts`'s identical note.

/**
 * Step 59's dedicated billing regression suite: the full token-balance +
 * usage-history + Stripe billing journey exercised end to end on top of
 * every wave-5/6/7 UI change layered onto Step 48/49/53's original,
 * narrower feature specs (`web/e2e/billing.spec.ts`,
 * `web/e2e/billing-checkout.spec.ts`). Every identity here is a brand-new,
 * ad hoc user registered through the UI in its own test — no dependency on
 * the shared fixture user/room, matching `smoke.spec.ts`'s convention.
 *
 * ## Running the Stripe Checkout portion locally
 *
 * `api-e2e` (this stack's Go API, see `docker-compose.yml`) has **no**
 * `STRIPE_*` environment configured by default — unlike the dev-stack `api`
 * service, which reads `STRIPE_SECRET_KEY`/`STRIPE_WEBHOOK_SECRET`/
 * `STRIPE_PLANS_JSON`/`STRIPE_TOKEN_PACKAGES_JSON` from `.env` (see
 * `.env.example`), and unlike the dev-only `stripe-cli` compose service that
 * forwards webhooks to it. Wiring the same env vars (plus an
 * `stripe-cli`-equivalent forwarder) into the `test` profile's `api-e2e`
 * block is out of this step's scope (see `docs/tasks/step59.md`'s Scope and
 * Out-of-scope notes) — it is a transitive dependency this step inherited
 * from Step 49's server-side Stripe integration, not something this
 * Playwright-only step is allowed to build.
 *
 * To exercise the third test below's full Checkout -> webhook ->
 * subscription -> cancel -> billing-history journey locally, until that
 * infra lands:
 * 1. Add `STRIPE_SECRET_KEY`/`STRIPE_WEBHOOK_SECRET`/`STRIPE_PLANS_JSON`/
 *    `STRIPE_TOKEN_PACKAGES_JSON` (see `.env.example` for the shape) to
 *    `api-e2e`'s `environment:` block in `docker-compose.yml` (a real Stripe
 *    test-mode `sk_test_...` key, plus a real test-mode Price ID per plan).
 * 2. Run `stripe login` once, then
 *    `stripe listen --forward-to localhost:8090/webhooks/stripe --print-secret`
 *    to obtain a `whsec_...` value; put it in `STRIPE_WEBHOOK_SECRET` above
 *    and restart `api-e2e`.
 * 3. Keep `stripe listen` running for the duration of the test run, so
 *    Checkout-completion webhooks actually reach `api-e2e`.
 *
 * Without that setup, the third test below probes the contract, asserts
 * everything that is testable without live Stripe credentials (the
 * catalog's Stripe-API-free 200, the `/billing/plans` page's empty state,
 * and the `POST /billing/checkout-session` error path), then calls
 * `test.skip()` with a clear reason instead of failing.
 */

const AI_PLACEHOLDER = "Ask me anything..."

interface RegisteredUser {
  email: string
  username: string
}

/**
 * Registers a brand-new user (unique email/username per run, matching
 * `smoke.spec.ts`'s convention) and creates a room, landing on that room's
 * page.
 */
async function registerAndCreateRoom(
  page: Page,
  prefix: string,
): Promise<{ user: RegisteredUser; roomName: string }> {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `${prefix}-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens, which would otherwise block registration client-side
  // before any request is even sent.
  const username = `${prefix}_${runId}`
  const password = `${prefix}-test-password-123`
  const roomName = `Billing Regression Room ${runId}`

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  return { user: { email, username }, roomName }
}

test.describe("billing regression", () => {
  test("balance badge, usage history, 402 insufficient-balance guard, and empty billing history", async ({
    page,
  }) => {
    const { roomName } = await registerAndCreateRoom(page, "billreg")

    // The top-bar balance badge renders a numeric balance -- a brand-new
    // user's `token_balances` row is lazily created at 0 (`GetOrCreateBalance`).
    const balanceLink = page.getByRole("link", { name: "View token usage history" })
    await expect(balanceLink).toBeVisible()
    await expect(balanceLink).toContainText(/\d/)

    // Clicking it navigates to /billing/usage and renders the usage history
    // list -- populated rows or the explicit empty state.
    await balanceLink.click()
    await expect(page).toHaveURL(/\/billing\/usage$/)
    await expect(page.getByRole("heading", { name: "Token Usage" })).toBeVisible()
    await expect(
      page.getByText("No usage yet").or(page.getByRole("table")),
    ).toBeVisible()

    // The billing-history view also has an explicit empty state for a
    // brand-new user with no payments yet.
    await page.goto("/billing/history")
    await expect(page.getByRole("heading", { name: "Billing History" })).toBeVisible()
    await expect(page.getByText("No payments yet")).toBeVisible()

    // Back to the room to drive the AI-send balance guard.
    await page.getByRole("button", { name: "Rooms" }).click()
    await expect(page).toHaveURL(/\/rooms$/)
    await page.getByRole("heading", { name: roomName }).click()
    await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

    const aiMessageContent = `Balance guard probe ${Date.now()}`
    await page.getByPlaceholder(AI_PLACEHOLDER).fill(aiMessageContent)
    await page.getByRole("button", { name: "Send with AI" }).click()

    // A fresh user's balance is 0, so `CheckBalance` rejects this first AI
    // send with 402 -- `MessageInput` surfaces the distinct inline error.
    // `.filter({ hasText: ... })` disambiguates from Next.js's own
    // `role="alert"` route announcer.
    await expect(
      page.getByRole("alert").filter({ hasText: "Insufficient token balance" }),
    ).toHaveText(/Insufficient token balance/)

    // The plain (non-AI) send path is unaffected by the guard.
    const plainMessageContent = `Plain send still works ${Date.now()}`
    await page.getByPlaceholder(AI_PLACEHOLDER).fill(plainMessageContent)
    await page.getByRole("button", { name: "Send", exact: true }).click()
    await expect(page.getByText(plainMessageContent).first()).toBeVisible()
  })

  test("GET /billing/subscription 204 maps to the 'no active subscription' state", async ({
    page,
  }) => {
    await registerAndCreateRoom(page, "billsub")

    // A brand-new user has no subscription row at all: `GetSubscription`
    // returns `domain.ErrNotFound`, which the handler maps to a documented
    // `204 No Content` (`billing_handler.go`'s `GetSubscription`) --
    // independent of whether Stripe is configured on this stack, since it
    // is purely "does a `subscriptions` row exist for this user", not a
    // Stripe API call.
    const subRes = await page.request.get("/api/proxy/billing/subscription")
    expect(subRes.status()).toBe(204)

    await page.goto("/billing/subscription")
    await expect(page.getByRole("heading", { name: "Your Subscription" })).toBeVisible()
    await expect(
      page.getByText("You don't have an active subscription"),
    ).toBeVisible()
    await expect(page.getByRole("button", { name: "View plans" })).toBeVisible()
  })

  test("plans catalog, checkout-session error path, and (if Stripe is configured) the full Checkout -> webhook -> subscription -> cancel -> billing-history journey", async ({
    page,
  }) => {
    const { user } = await registerAndCreateRoom(page, "billco")

    // 1. The plan/token-package catalog is documented as "the one billing
    // endpoint that never errors when Stripe is unconfigured"
    // (`ListPlanCatalog`'s own docstring): it is a pure, DB-free,
    // Stripe-API-free read of configured env-var JSON. This assertion holds
    // regardless of whether this stack has Stripe test keys wired in.
    const plansRes = await page.request.get("/api/proxy/billing/plans")
    expect(plansRes.status()).toBe(200)
    const { plans } = (await plansRes.json()) as {
      plans: Array<{ code: string; name: string; interval: string }>
    }

    await page.goto("/billing/plans")
    await expect(page.getByRole("heading", { name: "Plans & Token Packs" })).toBeVisible()
    if (plans.length === 0) {
      // The documented, unconfigured-by-default state on this stack (see
      // this file's doc comment): `STRIPE_PLANS_JSON`/
      // `STRIPE_TOKEN_PACKAGES_JSON` are unset on `api-e2e`, so the catalog
      // is empty and `PlanList` renders its explicit empty state.
      await expect(page.getByText("No plans available")).toBeVisible()
    } else {
      await expect(
        page.getByRole("button", { name: /^(Subscribe|Buy tokens)$/ }).first(),
      ).toBeVisible()
    }

    const monthlyPlan = plans.find((plan) => plan.interval === "month")

    // 2. Probe whether a real Checkout Session can actually be created --
    // this is what distinguishes "Stripe test keys/Prices are configured"
    // from "the catalog exists but Stripe itself is not wired up" (the
    // catalog can be non-empty from env-var JSON alone, independent of
    // `STRIPE_SECRET_KEY`).
    const probeBody = monthlyPlan
      ? { type: "subscription", plan_code: monthlyPlan.code }
      : { type: "subscription", plan_code: "billing-regression-probe" }
    const probeRes = await page.request.post("/api/proxy/billing/checkout-session", {
      data: probeBody,
    })

    if (!probeRes.ok()) {
      if (monthlyPlan) {
        // A known-valid plan code was probed, so `findPlan` would have
        // succeeded -- the only remaining failure mode is
        // `CreateSubscriptionCheckoutSession`'s `domain.ErrStripeNotConfigured`
        // guard, mapped to HTTP 503 by `billing_handler.go`. Assert it
        // explicitly (not just "not ok"), so a future regression that
        // silently changed the status code (rather than genuinely wiring up
        // Stripe) would still fail this spec.
        expect(probeRes.status()).toBe(503)

        // The same error path is exercised through the UI too: `PlanCard`'s
        // mutation surfaces it via a toast instead of silently failing.
        await page.getByRole("button", { name: "Subscribe" }).first().click()
        await expect(page.getByText("Could not start checkout")).toBeVisible()
      }
      // Otherwise (`plans` has no monthly entry -- most commonly an empty
      // catalog, the default on this stack) the probe's own fallback plan
      // code is not expected to resolve to a real plan, so its exact
      // status code is not asserted here -- only that it is not `ok()`,
      // which is already established by this branch.

      test.skip(
        true,
        `POST /billing/checkout-session returned HTTP ${probeRes.status()} -- Stripe test-mode keys/webhook forwarding are not configured on this stack's api-e2e (see this file's doc comment for the missing env vars and how to wire them in locally). Skipping the Checkout/webhook/subscription/cancel/billing-history portion.`,
      )
      return
    }

    // Stripe IS configured on this stack: drive the full journey.
    const probeBodyJson = (await probeRes.json()) as { checkout_url?: string }
    expect(probeBodyJson.checkout_url).toBeTruthy()

    await page.getByRole("button", { name: "Subscribe" }).first().click()
    await page.waitForURL(/^https:\/\/checkout\.stripe\.com\//)

    // Fill Stripe's hosted test-mode Checkout form with the documented
    // always-succeeds test card and submit.
    await page.getByPlaceholder("1234 1234 1234 1234").fill("4242424242424242")
    await page.getByPlaceholder("MM / YY").fill("12/34")
    await page.getByPlaceholder("CVC").fill("123")
    const nameField = page.getByLabel("Cardholder name")
    if (await nameField.isVisible().catch(() => false)) {
      await nameField.fill(`Billing Regression ${Date.now()}`)
    }
    const emailField = page.getByLabel("Email")
    if (await emailField.isVisible().catch(() => false)) {
      await emailField.fill(user.email)
    }
    await page.getByRole("button", { name: /^(Subscribe|Pay)/ }).click()

    // Stripe redirects back to this app's success route once the payment
    // is confirmed client-side; the subscription itself updates
    // asynchronously once `stripe listen` forwards the webhook.
    await page.waitForURL(/\/billing\/checkout\/success/, { timeout: 30_000 })
    await expect(page.getByText(monthlyPlan!.name)).toBeVisible({ timeout: 30_000 })

    // The subscription-status surface reflects the newly active plan --
    // this is the assertion that the webhook actually reached the API and
    // updated the subscription record, not just that Checkout redirected.
    await page.goto("/billing/subscription")
    await expect(page.getByText(monthlyPlan!.name)).toBeVisible()
    await expect(page.locator("[data-status='active']")).toBeVisible({ timeout: 30_000 })

    // The billing-history view lists at least one entry for the completed
    // Checkout payment.
    await page.goto("/billing/history")
    await expect(page.locator("[data-status='succeeded']").first()).toBeVisible()

    // Exercise the "Manage subscription" action (Stripe's hosted billing
    // portal) and cancel from there; the app reflects the resulting
    // cancel-at-period-end state once the corresponding webhook lands.
    await page.goto("/billing/subscription")
    await page.getByRole("button", { name: "Manage subscription" }).click()
    await page.waitForURL(/^https:\/\/billing\.stripe\.com\//, { timeout: 30_000 })

    await page.getByRole("button", { name: /cancel/i }).first().click()
    const confirmCancel = page.getByRole("button", { name: /cancel subscription|confirm/i })
    if (await confirmCancel.isVisible().catch(() => false)) {
      await confirmCancel.click()
    }

    // Return to the app (Stripe's hosted portal links back to the
    // `return_url` `CreateBillingPortalSession` was given) and poll the
    // subscription-status surface until it reflects the cancellation --
    // either `cancel_at_period_end: true` ("Ends on <date>") or an
    // immediate `canceled` status, whichever Stripe/Step 49 actually
    // produce for this plan's billing-portal cancellation flow.
    await page.goto("/billing/subscription")
    await expect(
      page
        .getByText(/Ends on|will not renew/i)
        .or(page.locator("[data-status='canceled']")),
    ).toBeVisible({ timeout: 30_000 })
  })
})
