import { expect, test } from "@playwright/test"
import { FIXTURE_ROOM_NAME, FIXTURE_USER } from "../support/fixtures"
import { creditTokenBalance } from "../support/credit-token-balance"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because `web/package.json` has no
// `"type": "module"` (see `attachments.spec.ts`'s identical note).

/** Logs into the seeded fixture user via the real login form and waits for
 * the post-login redirect to `/rooms`. */
async function loginAsFixtureUser(page: import("@playwright/test").Page): Promise<void> {
  await page.goto("/login")
  await page.getByPlaceholder("you@example.com").fill(FIXTURE_USER.email)
  await page.getByPlaceholder("Enter your password").fill(FIXTURE_USER.password)
  await page.getByRole("button", { name: "Sign in" }).click()
  // Longer timeout (wave-7 deflake, same as members/groups/rooms'
  // registerUser carryover fix): under full-suite parallelism this
  // post-auth navigation can exceed Playwright's default 5s.
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })
}

/**
 * End-to-end coverage of Step 34's model-metadata grouping plus the
 * "Send with AI" model-selection path, driven through the real UI against
 * the Redis-backed compose stack and the Step 10 LLM stub.
 *
 * Uses the shared fixture user/room (`e2e/support/fixtures.ts`) rather than
 * registering a fresh identity, per this step's own scope -- the seed
 * routine creates the fixture room as the fixture user, so the fixture user
 * is that room's `master` and can always invoke AI there.
 */
test("selecting a specific OpenAI model and sending with AI reaches the matching stub fixture", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`

  creditTokenBalance(FIXTURE_USER.email, 1_000_000)

  await loginAsFixtureUser(page)

  await page.getByRole("heading", { name: FIXTURE_ROOM_NAME }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  const composer = page.getByPlaceholder("Ask me anything...")
  await expect(composer).toBeVisible()

  // Open the model selector. Its trigger is the button immediately
  // following the "Attach image" icon button in `MessageInput`'s button
  // row (see `ModelSelector.tsx`) -- located structurally rather than by
  // its current label, since the label is whatever model happens to be
  // selected (the first model the gateway returns, deterministically an
  // OpenAI model per `llm-gateway`'s provider registration order, but not
  // worth hard-coding here).
  const modelTrigger = page
    .getByRole("button", { name: "Attach image" })
    .locator("xpath=following-sibling::button[1]")
  await modelTrigger.click()

  // Step 34: models are grouped by provider. Both the OpenAI and Anthropic
  // groups are always present in the E2E profile (the gateway's model list
  // is static regardless of which providers have real keys configured --
  // only OpenAI is ever actually invoked here), so asserting both group
  // headings are visible proves real grouping, not just a flat list that
  // happens to include an OpenAI entry.
  await expect(page.getByText("openai", { exact: true }).first()).toBeVisible()
  await expect(page.getByText("anthropic", { exact: true }).first()).toBeVisible()

  // Explicitly select a specific, non-default OpenAI model.
  await page.getByText("GPT-5 Mini", { exact: true }).click()

  // The popover closes and the trigger now reflects the explicit selection.
  await expect(
    page.getByRole("button", { name: "GPT-5 Mini", exact: true }),
  ).toBeVisible()

  const messageContent = `[[fixture:model-select]] Which model am I talking to? ${runId}`

  // The shared fixture room accumulates one identical canned reply per run
  // of this spec against a long-lived stack, so counting instances *before*
  // the send and asserting the count grew by exactly one below (rather than
  // `.first()` visibility, which a leftover reply from a prior run would
  // already satisfy even if this send never reached the stub) proves this
  // specific send produced a new reply.
  const cannedReplyText =
    "This canned reply confirms the explicitly selected OpenAI model reached the E2E stub."
  const cannedReplies = page.getByText(cannedReplyText)
  const cannedReplyCountBefore = await cannedReplies.count()

  await composer.fill(messageContent)
  await page.getByRole("button", { name: "Send with AI" }).click()

  await expect(page.getByText(messageContent).first()).toBeVisible()

  // The AI response matches the new `model-select.json`/`model-select.sse`
  // fixture's canned text -- proving the request actually reached the
  // step-10 stub via the selected model, not some other provider's model
  // being silently substituted (which would instead 404 against a
  // fixture-less provider or return the unrelated `default` text). Longer
  // timeout: the reply now arrives via the streaming pipeline (Step 54)
  // whose paced chunk delivery can exceed the default 5s under full-suite
  // parallelism.
  await expect(cannedReplies).toHaveCount(cannedReplyCountBefore + 1, {
    timeout: 15_000,
  })

  // No silent substitution: the selector still shows the explicitly chosen
  // model after the send completes (nothing in `MessageInput`/`ModelSelector`
  // resets `explicitModel` on send).
  await expect(
    page.getByRole("button", { name: "GPT-5 Mini", exact: true }),
  ).toBeVisible()
})
