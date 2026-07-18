import { expect, test, type Page } from "@playwright/test"
import { FIXTURE_ROOM_NAME, FIXTURE_USER } from "../support/fixtures"

/**
 * End-to-end coverage of realtime message delivery over the Redis-backed
 * `MessageHub` (Step 31) plus the web WebSocket client + live cache merge
 * (Step 35), driven against the real compose stack rather than either
 * step's own narrower unit/component tests.
 *
 * Uses two entirely independent Playwright `BrowserContext`s (separate
 * cookie jars, separate WebSocket connections), both authenticating as the
 * same seeded fixture user/room (`e2e/support/fixtures.ts`) -- mirroring
 * Step 35's own manual verification of "two different users (or two
 * browser profiles) in the same room". Only plain (non-AI) sends are used,
 * so this spec needs no token-balance top-up.
 */

/** Logs into the seeded fixture user and opens the fixture room, waiting
 * for the WebSocket connection indicator to reach "connected" -- a message
 * sent before the socket is open cannot be delivered live, so this ordering
 * matters for the spec's reliability, not just its readability. */
async function loginAndOpenFixtureRoom(page: Page): Promise<void> {
  await page.goto("/login")
  await page.getByPlaceholder("you@example.com").fill(FIXTURE_USER.email)
  await page.getByPlaceholder("Enter your password").fill(FIXTURE_USER.password)
  await page.getByRole("button", { name: "Sign in" }).click()
  // Longer timeout (wave-7 deflake, same as members/groups/rooms'
  // registerUser carryover fix): under full-suite parallelism this
  // post-auth navigation can exceed Playwright's default 5s.
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  await page.getByRole("heading", { name: FIXTURE_ROOM_NAME }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  await expect(
    page.locator('[aria-label="Connection status: Connected"]'),
  ).toBeVisible()
}

test.describe("two-client realtime delivery", () => {
  test("a plain message sent in one browser context appears in another without a reload, with no duplicate in the sender's own view", async ({
    browser,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
    const messageContent = `Realtime delivery probe ${runId}`

    const contextA = await browser.newContext()
    const contextB = await browser.newContext()

    try {
      const pageA = await contextA.newPage()
      const pageB = await contextB.newPage()

      // Both contexts log in independently (same underlying account, two
      // separate sessions/WS connections) and reach a connected socket
      // before either one sends anything.
      await loginAndOpenFixtureRoom(pageA)
      await loginAndOpenFixtureRoom(pageB)

      await pageA.getByPlaceholder("Ask me anything...").fill(messageContent)
      await pageA.getByRole("button", { name: "Send", exact: true }).click()

      // Delivered to context B purely via the WS `message_created` event +
      // `mergeMessageEvent`'s cache merge -- context B never navigates or
      // reloads.
      await expect(pageB.getByText(messageContent).first()).toBeVisible()

      // Dedup contract (`mergeMessageEvent`): the sender's own optimistic
      // entry is reconciled in place against its own WS echo, not appended
      // a second time -- exactly one rendered copy in context A's view.
      await expect(pageA.getByText(messageContent)).toHaveCount(1)
    } finally {
      await contextA.close()
      await contextB.close()
    }
  })
})
