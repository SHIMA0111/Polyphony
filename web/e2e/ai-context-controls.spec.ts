import { expect, test } from "@playwright/test"

/**
 * End-to-end coverage for Step 38's per-message AI context controls: the
 * exclude/include toggle, soft-delete (behind a confirmation dialog), and
 * the composer's live token meter — driven entirely through the UI against
 * Step 10's isolated `test` profile compose stack.
 *
 * Registers a brand-new user (like `smoke.spec.ts`) rather than reusing the
 * shared fixture user/room, so this spec's message/token-count assertions
 * never have to account for messages left behind by other specs sharing
 * the fixture room. A fresh room's creator is always its `master` (see
 * `server/internal/usecase/room`), so both the exclude toggle
 * (`ActionInvokeAI`, member-or-above) and delete (owner-or-admin) are
 * visible on every message this spec's own user sends.
 *
 * Deliberately never uses "Send with AI": a brand-new user has a zero token
 * balance by default (see this wave's billing seed), so any AI invocation
 * would 402 — this spec only needs plain sends plus the exclude/delete/
 * estimate loop, none of which touch billing.
 */
test("exclude and delete a message and observe the token meter and message list update", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `ai-context-${runId}@polyphony.test`
  const username = `ai_context_${runId}`
  const password = "ai-context-test-password-123"
  const roomName = `AI Context Test Room ${runId}`
  const firstMessage = `First message ${runId}`
  const secondMessage = `Second message ${runId}`

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

  const composer = page.getByPlaceholder("Ask me anything...")
  const sendButton = page.getByRole("button", { name: "Send", exact: true })
  const tokenMeter = page.getByText(/^~\d+ tokens$/)

  // Send two plain messages; the meter should reflect a nonzero count that
  // includes both of them once it settles past its debounce.
  await composer.fill(firstMessage)
  await sendButton.click()
  await expect(page.getByText(firstMessage).first()).toBeVisible()

  await composer.fill(secondMessage)
  await sendButton.click()
  await expect(page.getByText(secondMessage).first()).toBeVisible()

  await expect(tokenMeter).toBeVisible()
  const countAfterBothSends = Number(
    (await tokenMeter.textContent())?.match(/\d+/)?.[0],
  )
  expect(countAfterBothSends).toBeGreaterThan(0)

  // Exclude the first message from AI context via its action menu; the
  // meter's count should drop since it's no longer part of the payload the
  // meter estimates against.
  const firstMessageBubble = page
    .locator("div", { hasText: firstMessage })
    .last()
  await firstMessageBubble.hover()
  await page
    .getByRole("button", { name: "Message actions" })
    .first()
    .click()
  await page.getByRole("menuitem", { name: "Exclude from AI" }).click()

  // The visual indicator (tooltip-wrapped EyeOff icon) appears immediately.
  await expect(
    page.getByLabel("Excluded from AI context").first(),
  ).toBeVisible()

  await expect
    .poll(async () => {
      const text = await tokenMeter.textContent()
      return Number(text?.match(/\d+/)?.[0])
    })
    .toBeLessThan(countAfterBothSends)

  const countAfterExclude = Number(
    (await tokenMeter.textContent())?.match(/\d+/)?.[0],
  )

  // Delete the second message via its menu + confirmation dialog; it must
  // disappear from the list and the meter must drop further.
  const secondMessageBubble = page
    .locator("div", { hasText: secondMessage })
    .last()
  await secondMessageBubble.hover()
  await page
    .getByRole("button", { name: "Message actions" })
    .last()
    .click()
  await page.getByRole("menuitem", { name: "Delete message" }).click()
  await page.getByRole("button", { name: "Delete", exact: true }).click()

  await expect(page.getByText(secondMessage)).toHaveCount(0)

  await expect
    .poll(async () => {
      const text = await tokenMeter.textContent()
      return Number(text?.match(/\d+/)?.[0])
    })
    .toBeLessThan(countAfterExclude)
})
