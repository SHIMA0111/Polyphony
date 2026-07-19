import { expect, test } from "@playwright/test"
import { FIXTURE_USER } from "../support/fixtures"
import { creditTokenBalance } from "../support/credit-token-balance"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because `web/package.json` has no
// `"type": "module"` (see `attachments.spec.ts`'s identical note).

/**
 * End-to-end coverage proving that Step 36's room-level AI context cutoff
 * and Step 38's per-message exclude/delete controls do not break the AI
 * send pipeline they sit in front of: this spec does not inspect the exact
 * prompt reaching the gateway (the stub is content-blind besides the
 * `[[fixture:NAME]]` marker) -- the externally observable proof is that
 * exclude, delete, and cutoff all succeed and are reflected in the UI, and
 * that a subsequent AI send still completes correctly against the same
 * stub afterward.
 *
 * Logs in as the seeded fixture user but creates a brand-new room within
 * the spec (rather than reusing the shared fixture room) so this spec's
 * message history and cutoff setting never interact with other specs
 * sharing the fixture room -- the fixture user is still that new room's
 * creator, hence its `master`, so the settings-drawer cutoff control (which
 * Step 36 gates to `admin`/`master`) is visible and usable.
 */
test("exclude, delete, and an AI context cutoff all still allow a subsequent AI send to complete", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const roomName = `Context Controls AI Effect Room ${runId}`
  const firstMessage = `Excluded message ${runId}`
  const secondMessage = `Deleted message ${runId}`
  const thirdMessage = `Kept message ${runId}`
  const afterCutoffMessage = `After cutoff message ${runId}`

  creditTokenBalance(FIXTURE_USER.email, 1_000_000)

  await page.goto("/login")
  await page.getByPlaceholder("you@example.com").fill(FIXTURE_USER.email)
  await page.getByPlaceholder("Enter your password").fill(FIXTURE_USER.password)
  await page.getByRole("button", { name: "Sign in" }).click()
  // Longer timeout (wave-7 deflake, same as members/groups/rooms'
  // registerUser carryover fix): under full-suite parallelism this
  // post-auth navigation can exceed Playwright's default 5s.
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  const composer = page.getByPlaceholder("Ask me anything...")

  // Three plain messages: one to exclude, one to delete, one to keep.
  await composer.fill(firstMessage)
  await page.getByRole("button", { name: "Send", exact: true }).click()
  await expect(page.getByText(firstMessage).first()).toBeVisible()

  await composer.fill(secondMessage)
  await page.getByRole("button", { name: "Send", exact: true }).click()
  await expect(page.getByText(secondMessage).first()).toBeVisible()

  await composer.fill(thirdMessage)
  await page.getByRole("button", { name: "Send", exact: true }).click()
  await expect(page.getByText(thirdMessage).first()).toBeVisible()

  // (a) Exclude the first message from AI context via its per-message menu
  // (Step 38) and assert its persistent visual indicator appears.
  const firstMessageBubble = page.locator("div", { hasText: firstMessage }).last()
  await firstMessageBubble.hover()
  await page.getByRole("button", { name: "Message actions" }).first().click()
  await page.getByRole("menuitem", { name: "Exclude from AI" }).click()
  await expect(page.getByLabel("Excluded from AI context").first()).toBeVisible()

  // (b) Delete the second message via its menu + confirmation dialog (Step
  // 38) and assert it disappears from the rendered list.
  const secondMessageBubble = page.locator("div", { hasText: secondMessage }).last()
  await secondMessageBubble.hover()
  await page.getByRole("button", { name: "Message actions" }).nth(1).click()
  // Scoped to the currently-open menu (`data-state="open"`): the first
  // message's already-used menu stays mounted in its own portal (and can
  // still be mid-close-animation, i.e. transiently "visible"), so an
  // unscoped `getByRole("menuitem")` resolves to two "Delete message" items
  // and trips strict mode (caught live by the wave-7 integration run).
  await page
    .locator('[data-scope="menu"][data-part="content"][data-state="open"]')
    .getByRole("menuitem", { name: "Delete message" })
    .click()
  await page.getByRole("button", { name: "Delete", exact: true }).click()
  await expect(page.getByText(secondMessage)).toHaveCount(0)

  // (c) Open the room settings drawer (Step 36) as the fixture user, who is
  // this fresh room's master, and set the AI context cutoff to "now".
  await page.getByRole("button", { name: "Room settings" }).click()
  await expect(page.getByRole("heading", { name: "Room settings" })).toBeVisible()
  await expect(page.getByText("No cutoff set")).toBeVisible()

  await page.getByRole("button", { name: "Set cutoff to now" }).click()
  await expect(page.getByText("No cutoff set")).toHaveCount(0)

  // `exact: true`: the drawer also has an icon close-trigger whose
  // accessible name is "Close room settings", which non-exact (substring)
  // role matching would also hit, tripping strict mode.
  await page.getByRole("button", { name: "Close", exact: true }).click()
  await expect(page.getByRole("heading", { name: "Room settings" })).toHaveCount(0)

  // (d) Send a further plain message after the cutoff, then send an AI
  // message with a distinct marker and assert the AI response is the new
  // `context-check.json`/`context-check.sse` fixture's canned text --
  // proving the request reached the gateway successfully with the
  // excluded/deleted/pre-cutoff messages filtered out server-side, without
  // needing to inspect the exact prompt sent.
  await composer.fill(afterCutoffMessage)
  await page.getByRole("button", { name: "Send", exact: true }).click()
  await expect(page.getByText(afterCutoffMessage).first()).toBeVisible()

  const aiMessageContent = `[[fixture:context-check]] Are you still there? ${runId}`
  await composer.fill(aiMessageContent)
  await page.getByRole("button", { name: "Send with AI" }).click()
  await expect(page.getByText(aiMessageContent).first()).toBeVisible()

  await expect(
    page.getByText(
      "This canned reply confirms the AI send pipeline still completed after the exclude, delete, and cutoff context controls were applied.",
    ),
  ).toBeVisible()
})
