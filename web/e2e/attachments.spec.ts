import path from "node:path"
import { expect, test } from "@playwright/test"
import { creditTokenBalance } from "./support/credit-token-balance"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly.

/** A small, real 1x1 PNG committed alongside this spec. */
const FIXTURE_IMAGE_PATH = path.resolve(__dirname, "./fixtures/test-image.png")

/**
 * End-to-end coverage for Step 45's image upload + attachment UI + Vision
 * send: registers a brand-new user (matching `smoke.spec.ts`'s convention),
 * attaches a fixture image via the file-picker input, sends it via "Send
 * with AI", and asserts:
 * - the attachment chip's "Remove attachment" button disappears once the
 *   message has sent (staging is `reset()` on success);
 * - a thumbnail `<img>` renders inside the sent message bubble;
 * - an AI reply (from the Step 10 LLM stub's `default.json` fixture, served
 *   for every request regardless of content) eventually appears, proving
 *   the documented `sendAIMessage` -> `attachToMessage` ->
 *   `regenerateAIMessage` sequence round-trips end to end against the real
 *   (isolated) compose test stack;
 * - the initial (text-only) pass and the Vision-aware regenerate pass carry
 *   *distinct* `llm-stub` responses -- both draw from the same
 *   `default.json` fixture (there is no `[[fixture:NAME]]` marker in either
 *   request), so the stub tags every response with a per-request,
 *   monotonically increasing `[[seq:N]]` marker (see `llm-stub/server.ts`'s
 *   `nextRequestSequence`) precisely so a broken regenerate that silently
 *   no-ops (reusing the first pass's reply instead of actually re-invoking
 *   the LLM Gateway with the now-attached image) cannot pass this
 *   assertion merely by producing byte-identical final text.
 */
test("attach an image and send it with AI", async ({ page }) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `attachments-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens, matching every other spec's convention.
  const username = `attachments_${runId}`
  const password = "attachments-test-password-123"
  const roomName = `Attachments Test Room ${runId}`
  const messageContent = `Check out this image ${runId}`

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()

  await expect(page).toHaveURL(/\/rooms$/)

  creditTokenBalance(email, 1_000_000)

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()

  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  // Attach the fixture image via the hidden file-picker input rather than
  // simulating real OS drag-and-drop, which Playwright cannot drive
  // directly — `setInputFiles` is the standard idiom for this and works
  // regardless of the input's `hidden` attribute.
  await page.locator('input[type="file"]').setInputFiles(FIXTURE_IMAGE_PATH)

  const removeAttachmentButton = page.getByRole("button", {
    name: "Remove attachment",
  })
  const sendWithAIButton = page.getByRole("button", { name: "Send with AI" })

  // The chip appears immediately. The message text must be filled before
  // asserting enablement: both send buttons stay disabled while the input
  // is empty (`MessageInput`'s `isDisabled` includes `!input.trim()`), so
  // "Send with AI" only becomes enabled once there is text AND the (fast,
  // local MinIO) upload has finished.
  await expect(removeAttachmentButton).toBeVisible()
  await page.getByPlaceholder("Ask me anything...").fill(messageContent)
  await expect(sendWithAIButton).toBeEnabled()

  // Set up both response listeners *before* the click that triggers them:
  // `handleSendWithAI` fires the initial `POST .../messages/ai` call, then
  // -- once the attachment is linked -- the Vision-aware
  // `POST .../messages/:messageId/regenerate` call, all within the single
  // click handler below. Each response's own JSON body (not the final DOM
  // state, which only ever shows the *last* write) is what proves the two
  // passes are genuinely distinct round-trips against the LLM stub.
  const sendAIResponsePromise = page.waitForResponse(
    (res) =>
      res.request().method() === "POST" &&
      new URL(res.url()).pathname.endsWith("/messages/ai"),
  )
  const regenerateResponsePromise = page.waitForResponse(
    (res) =>
      res.request().method() === "POST" &&
      new URL(res.url()).pathname.endsWith("/regenerate"),
  )

  await sendWithAIButton.click()

  const [sendAIResponse, regenerateResponse] = await Promise.all([
    sendAIResponsePromise,
    regenerateResponsePromise,
  ])

  // Staging is reset() on a successful send, so the chip disappears.
  await expect(removeAttachmentButton).toHaveCount(0)

  await expect(page.getByText(messageContent).first()).toBeVisible()

  // The thumbnail renders inside the sent message bubble.
  await expect(
    page.getByRole("img", { name: "Message attachment" }),
  ).toBeVisible()

  // The AI's reply appears -- the initial pass and the Vision-aware
  // regenerate both draw from the same canned stub fixture
  // (`llm-stub/fixtures/default.json`), so this only proves *a* reply
  // landed; the sequence-marker assertion below is what proves the
  // regenerate pass actually ran. Longer timeout (wave-7): the non-private
  // AI send now goes through the streaming endpoint (Step 54) with the
  // stub's paced SSE delivery, and this send is additionally followed by a
  // Vision regenerate -- under full-suite parallelism the combined round
  // trip can exceed the default 5s expect timeout.
  await expect(
    page.getByText("This is a canned E2E stub response for testing purposes."),
  ).toBeVisible({ timeout: 15_000 })

  // Distinguishability: extract each pass's own `[[seq:N]]` marker (see
  // this spec's own doc comment) from the two captured responses' bodies,
  // and assert they differ -- a broken regenerate that no-ops would instead
  // leave the AI message's content (and therefore its marker) unchanged
  // from the initial pass.
  const SEQ_MARKER = /\[\[seq:(\d+)]]/
  function extractSeqMarker(content: string): number {
    const match = content.match(SEQ_MARKER)
    expect(match, `expected a [[seq:N]] marker in llm-stub content: ${content}`).not.toBeNull()
    return Number(match![1])
  }

  const sendAIBody = (await sendAIResponse.json()) as {
    ai_message: { content: string }
  }
  const regenerateBody = (await regenerateResponse.json()) as { content: string }

  const firstPassSeq = extractSeqMarker(sendAIBody.ai_message.content)
  const secondPassSeq = extractSeqMarker(regenerateBody.content)

  expect(
    secondPassSeq,
    "the Vision-aware regenerate pass must produce a genuinely new llm-stub response rather than silently reusing the initial pass's reply",
  ).not.toBe(firstPassSeq)
})
