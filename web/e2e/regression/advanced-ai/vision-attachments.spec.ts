import path from "node:path"
import { expect, test } from "@playwright/test"
import { creditTokenBalance } from "../../support/credit-token-balance"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly (see
// `attachments.spec.ts`'s identical note, which this spec's send/attach
// flow is copied from).

/** The same fixture image `web/e2e/attachments.spec.ts` already committed --
 * reused as-is, no second copy. */
const FIXTURE_IMAGE_PATH = path.resolve(__dirname, "../../fixtures/test-image.png")

/**
 * Broader regression coverage for Step 45's image upload + attachment UI +
 * Vision send, layered on top of Step 45's own `web/e2e/attachments.spec.ts`
 * (still run unmodified alongside this spec): that spec only ever asserts
 * on the send-time, optimistic-cache-seeded thumbnail render
 * (`MessageAttachments.tsx`'s doc comment: "for a message sent this
 * session, `useChatRoom`'s send-with-attachments sequencing has already
 * seeded that exact query key"). This spec additionally reloads the page —
 * forcing `useMessageAttachments`'s real network fetch (`GET
 * /rooms/:roomId/messages/:messageId/attachments`) rather than the
 * send-time cache — and re-asserts the thumbnail still renders from that
 * cold-loaded `view_url`, then opens and closes Step 45's lightbox against
 * it.
 */
test("an attached image survives a page reload and its lightbox still opens/closes from cold-loaded history", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `adv-vision-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens, matching every other spec's convention.
  const username = `adv_vision_${runId}`
  const password = "adv-vision-test-password-123"
  const roomName = `Advanced Vision Test Room ${runId}`
  const messageContent = `Check out this image ${runId}`

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  creditTokenBalance(email, 1_000_000)

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  // (a) Attach and send, exactly like Step 45's own spec.
  await page.locator('input[type="file"]').setInputFiles(FIXTURE_IMAGE_PATH)

  const removeAttachmentButton = page.getByRole("button", { name: "Remove attachment" })
  const sendWithAIButton = page.getByRole("button", { name: "Send with AI" })

  await expect(removeAttachmentButton).toBeVisible()
  await page.getByPlaceholder("Ask me anything...").fill(messageContent)
  await expect(sendWithAIButton).toBeEnabled()
  await sendWithAIButton.click()

  await expect(removeAttachmentButton).toHaveCount(0)
  await expect(page.getByText(messageContent).first()).toBeVisible()

  const sendTimeThumbnail = page.getByRole("img", { name: "Message attachment" })
  await expect(sendTimeThumbnail).toBeVisible()

  await expect(
    page.getByText("This is a canned E2E stub response for testing purposes."),
  ).toBeVisible({ timeout: 15_000 })

  // (b) Reload -- forces `useMessageAttachments`'s real network fetch
  // instead of the send-time optimistic cache seed.
  await page.reload()
  await expect(page.getByText(messageContent).first()).toBeVisible()

  const reloadedThumbnail = page.getByRole("img", { name: "Message attachment" })
  await expect(reloadedThumbnail).toBeVisible()
  await expect(reloadedThumbnail).toHaveAttribute("src", /.+/)

  // (c) Clicking the cold-loaded thumbnail still opens the lightbox with the
  // full-size image, closable via Escape.
  await reloadedThumbnail.click()
  const fullSizeImage = page.getByRole("img", { name: "Full-size attachment" })
  await expect(fullSizeImage).toBeVisible()

  await page.keyboard.press("Escape")
  await expect(fullSizeImage).toHaveCount(0)
})
