import { expect, test } from "@playwright/test"

/**
 * End-to-end coverage for Step 53's plan-selection -> Stripe test-mode
 * Checkout -> webhook-driven subscription-update flow.
 *
 * This spec requires two things Step 49's server-side territory only wires
 * up when live Stripe test credentials are configured on the E2E stack:
 * - Stripe test-mode secret key + Price IDs on `api-e2e` (so
 *   `POST /billing/checkout-session` returns a real `checkout_url` instead
 *   of erroring), and
 * - a `stripe listen --forward-to <api-e2e webhook route>` process running
 *   alongside `task test:e2e:up`, forwarding Checkout-completion events so
 *   the webhook-driven subscription update actually lands.
 *
 * Per this plan's "external SaaS is test-mode only" note, `api-e2e` has no
 * `STRIPE_*` env configured by default (that wiring is explicitly Step 49's
 * territory, not this step's), so this spec's first act is to probe
 * whether a real Checkout Session can be created at all — if not, it calls
 * `test.skip()` with a clear reason instead of failing, keeping this spec
 * runnable (and green) in environments without Stripe configured, and only
 * exercising the full flow where an operator has manually run
 * `stripe login` + `stripe listen` per this file's own doc comment above.
 *
 * Setting `STRIPE_E2E=1` opts out of that skip-when-unconfigured leniency:
 * every probe below that would otherwise call `test.skip()` instead hard-
 * fails via `expect(...).toBeTruthy()`. Set it on any run where Stripe test-
 * mode credentials and `stripe listen` are known to be wired up (e.g. a
 * dedicated CI job), so a real regression in that wiring shows up as a
 * failure instead of a silently-green skip.
 */
const STRIPE_E2E_ENABLED = process.env.STRIPE_E2E === "1"
test("plan selection through Stripe test Checkout to a webhook-driven subscription update", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `billing-checkout-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens, which would otherwise block registration client-side
  // before any request is even sent. Short prefix: registerSchema also caps
  // usernames at 32 characters, and `billing_checkout_${runId}` (36 chars)
  // was rejected client-side.
  const username = `billing_${runId}`
  const password = "billing-checkout-test-password-123"

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()

  await expect(page).toHaveURL(/\/rooms$/)

  // 1. Probe the plan catalog. `page.request` shares the browser context's
  // cookies (the `ory_kratos_session` cookie just minted by registration),
  // so this authenticated call succeeds the same way the app's own
  // `apiRequest` calls do.
  const plansRes = await page.request.get("/api/proxy/billing/plans")
  if (!plansRes.ok()) {
    const reason = `GET /billing/plans returned HTTP ${plansRes.status()} — skipping (Stripe test-mode plan catalog is not configured on this stack)`
    if (STRIPE_E2E_ENABLED) expect(plansRes.ok(), reason).toBeTruthy()
    test.skip(true, reason)
    return
  }

  const { plans } = (await plansRes.json()) as {
    plans: Array<{ code: string; name: string; interval: string }>
  }
  const monthlyPlan = plans.find((plan) => plan.interval === "month")
  if (!monthlyPlan) {
    const reason = "No monthly subscription plan in the catalog — skipping"
    if (STRIPE_E2E_ENABLED) expect(Boolean(monthlyPlan), reason).toBeTruthy()
    test.skip(true, reason)
    return
  }

  // 2. Probe that a real Checkout Session can actually be created for it —
  // this is what actually distinguishes "Stripe test keys/Prices are
  // configured" from "the catalog exists but Stripe itself is not wired
  // up", since Step 49's usecase can list a plan catalog from its own DB
  // seed independent of whether `STRIPE_SECRET_KEY` is set.
  const probeRes = await page.request.post("/api/proxy/billing/checkout-session", {
    data: { type: "subscription", plan_code: monthlyPlan.code },
  })
  if (!probeRes.ok()) {
    const reason = `POST /billing/checkout-session returned HTTP ${probeRes.status()} — skipping (Stripe test-mode keys are not configured on this stack)`
    if (STRIPE_E2E_ENABLED) expect(probeRes.ok(), reason).toBeTruthy()
    test.skip(true, reason)
    return
  }
  const probeBody = (await probeRes.json()) as { checkout_url?: string }
  if (!probeBody.checkout_url) {
    const reason = "POST /billing/checkout-session did not return a checkout_url — skipping"
    if (STRIPE_E2E_ENABLED) expect(Boolean(probeBody.checkout_url), reason).toBeTruthy()
    test.skip(true, reason)
    return
  }

  // 3. The real flow: navigate to /billing/plans, click "Subscribe" on
  // `monthlyPlan`'s own card, and confirm the browser actually reaches
  // Stripe's hosted test-mode Checkout page. Scoped to the card carrying
  // `[data-testid="plan-card-<code>"]` (see `PlanCard.tsx`) rather than
  // `.first()`-ing every "Subscribe" button on the page: the catalog can
  // list more than one monthly plan, and `.first()` would silently click
  // whichever one rendered first instead of the plan this spec actually
  // probed and asserts on below.
  await page.goto("/billing/plans")
  await expect(page.getByRole("heading", { name: "Plans & Token Packs" })).toBeVisible()

  const monthlyPlanCard = page.locator(`[data-testid="plan-card-${monthlyPlan.code}"]`)
  await monthlyPlanCard.getByRole("button", { name: "Subscribe" }).click()
  await page.waitForURL(/^https:\/\/checkout\.stripe\.com\//)

  // 4. Fill Stripe's hosted test-mode Checkout form with the documented
  // always-succeeds test card and submit.
  await page.getByPlaceholder("1234 1234 1234 1234").fill("4242424242424242")
  await page.getByPlaceholder("MM / YY").fill("12/34")
  await page.getByPlaceholder("CVC").fill("123")
  const nameField = page.getByLabel("Cardholder name")
  if (await nameField.isVisible().catch(() => false)) {
    await nameField.fill(`Billing Checkout Test ${runId}`)
  }
  const emailField = page.getByLabel("Email")
  if (await emailField.isVisible().catch(() => false)) {
    await emailField.fill(email)
  }
  await page.getByRole("button", { name: /^(Subscribe|Pay)/ }).click()

  // 5. Stripe redirects back to this app's success route once the payment
  // is confirmed client-side; the subscription itself updates
  // asynchronously once `stripe listen` forwards the webhook, which this
  // route polls for internally (see its own doc comment).
  await page.waitForURL(/\/billing\/checkout\/success/, { timeout: 30_000 })
  await expect(page.getByText(monthlyPlan.name)).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(/renews on/i)).toBeVisible()

  // 6. The same plan/status is reflected on /billing/subscription...
  await page.goto("/billing/subscription")
  await expect(page.getByText(monthlyPlan.name)).toBeVisible()
  await expect(page.getByText("active")).toBeVisible()

  // 7. ...and /billing/history shows at least one succeeded payment row.
  await page.goto("/billing/history")
  await expect(page.getByText("succeeded").first()).toBeVisible()
})
