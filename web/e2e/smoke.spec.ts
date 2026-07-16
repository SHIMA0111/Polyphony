import { expect, test } from "@playwright/test"

/**
 * End-to-end smoke test: register a brand-new user, create a room, open it,
 * and send a plain (non-AI) message — driven entirely through the UI so it
 * exercises the Step-4 BFF proxy (`app/api/proxy/[...path]`) end to end,
 * from the browser through the Next.js server to the Go API.
 *
 * Uses a unique email/username per run instead of the shared seed fixture
 * (see `e2e/support/fixtures.ts`) so this spec has no dependency on seed
 * ordering and can run standalone.
 */
test("register, create room, and send a message", async ({ page }) => {
  const runId = `${Date.now()}-${Math.floor(Math.random() * 100_000)}`
  const email = `smoke-${runId}@polyphony.test`
  const username = `smoke-${runId}`
  const password = "smoke-test-password-123"
  const roomName = `Smoke Test Room ${runId}`
  const messageContent = `Hello from the smoke spec ${runId}`

  await page.goto("/register")

  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()

  await expect(page).toHaveURL(/\/rooms$/)

  // A brand-new user's /rooms page renders "New Room" twice (the header
  // button and the empty-state CTA); `.first()` disambiguates since either
  // one opens the same CreateRoomForm dialog.
  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()

  await page.getByRole("heading", { name: roomName }).click()

  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  await page.getByPlaceholder("Ask me anything...").fill(messageContent)
  await page.getByRole("button", { name: "Send", exact: true }).click()

  await expect(page.getByText(messageContent)).toBeVisible()
})
