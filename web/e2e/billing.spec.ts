import { expect, test } from "@playwright/test"

/**
 * End-to-end coverage for Step 48's token balance + usage history UI:
 * registers a brand-new user (fresh ad hoc identity, matching
 * `smoke.spec.ts`'s convention — no dependency on the seeded fixture user),
 * creates a room, and proves:
 * - the top-bar `BalanceBadge` renders a numeric balance;
 * - clicking it navigates to `/billing/usage`, which renders the usage
 *   history list (populated rows or the explicit empty state);
 * - sending an AI message trips the 402 insufficient-balance guard
 *   (`BillingUsecase.CheckBalance`, `server/internal/usecase/billing`) —
 *   a brand-new user's `token_balances` row is lazily created at zero
 *   balance (`GetOrCreateBalance`), so the very first "Send with AI"
 *   already 402s; no depletion loop is needed — and that `MessageInput`
 *   surfaces the distinct inline error instead of a silent failure;
 * - the plain (non-AI) "Send" path is unaffected by the guard.
 */
test("balance badge, usage history, and the 402 insufficient-balance guard", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `billing-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens, which would otherwise block registration client-side
  // before any request is even sent.
  const username = `billing_${runId}`
  const password = "billing-test-password-123"
  const roomName = `Billing Test Room ${runId}`

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()

  await expect(page).toHaveURL(/\/rooms$/)

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()

  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  // The top-bar balance badge (`BalanceBadge`) renders a numeric balance —
  // whatever the stack's default provisioning yields (a brand-new user
  // starts at 0, per `GetOrCreateBalance`'s lazy zero-balance creation).
  const balanceLink = page.getByRole("link", { name: "View token usage history" })
  await expect(balanceLink).toBeVisible()
  await expect(balanceLink).toContainText(/\d/)

  // Clicking it navigates to /billing/usage and renders the usage history
  // list — either populated rows or the explicit empty state, depending on
  // the stack's default provisioning.
  await balanceLink.click()
  await expect(page).toHaveURL(/\/billing\/usage$/)
  await expect(page.getByRole("heading", { name: "Token Usage" })).toBeVisible()
  await expect(
    page.getByText("No usage yet").or(page.getByRole("table")),
  ).toBeVisible()

  // Back to the room to drive the AI-send balance guard.
  await page.getByRole("button", { name: "Rooms" }).click()
  await expect(page).toHaveURL(/\/rooms$/)
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  const aiMessageContent = `Balance guard probe ${runId}`
  await page.getByPlaceholder("Ask me anything...").fill(aiMessageContent)
  await page.getByRole("button", { name: "Send with AI" }).click()

  // A fresh user's balance is 0, so `CheckBalance` rejects this first AI
  // send with 402 — `MessageInput` renders the distinct inline error
  // instead of silently discarding the rejection.
  // `.filter({ hasText: ... })` disambiguates from Next.js's own
  // `role="alert"` route announcer (`#__next-route-announcer__`), which
  // otherwise trips Playwright's strict mode — same pattern as the
  // wave-4 smoke.spec.ts fix.
  await expect(
    page.getByRole("alert").filter({ hasText: "Insufficient token balance" }),
  ).toHaveText(/Insufficient token balance/)
  // `exact: true` disambiguates from the BalanceBadge's own
  // "View token usage history" link, which also matches a fuzzy "Usage".
  await expect(page.getByRole("link", { name: "Usage", exact: true })).toBeVisible()

  // The plain (non-AI) send path is unaffected by the guard: it still
  // works normally afterward.
  const plainMessageContent = `Plain send still works ${runId}`
  await page.getByPlaceholder("Ask me anything...").fill(plainMessageContent)
  await page.getByRole("button", { name: "Send", exact: true }).click()

  await expect(page.getByText(plainMessageContent).first()).toBeVisible()
})
