import { execFileSync } from "node:child_process"
import path from "node:path"
import { expect, test } from "@playwright/test"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly.

/** `server/`, so `go run ./cmd/seed-tokens` below resolves relative to the
 * Go module root regardless of the shell's own working directory. */
const SERVER_DIR = path.resolve(__dirname, "../../server")

/** A small, real 1x1 PNG committed alongside this spec. */
const FIXTURE_IMAGE_PATH = path.resolve(__dirname, "./fixtures/test-image.png")

/**
 * Credits `email`'s token balance directly against the e2e stack's Postgres
 * database (`db-e2e`, mapped to the host at `localhost:5433` — see
 * `docker-compose.yml`), by shelling out to `server/cmd/seed-tokens`: the
 * same `BillingRepository.CreditAndRecord` production code path the `task
 * billing:topup` dev convenience task already drives (see that CLI's own
 * doc comment), just pointed at the e2e database instead of the dev one.
 *
 * A brand-new user's `token_balances` row is lazily created at zero balance
 * (`BillingUsecase.GetOrCreateBalance` — see `billing.spec.ts`'s 402
 * coverage), so "Send with AI" against a freshly-registered user always
 * 402s unless topped up first. Crediting balance is the only way to
 * exercise this step's send -> attach -> regenerate sequence end to end
 * without adding a dedicated (and much heavier, out-of-scope-for-a-web-step)
 * test-only balance-grant endpoint to the Go API.
 */
function creditTokenBalance(email: string, amount: number): void {
  const databaseUrl =
    process.env.E2E_SEED_DATABASE_URL ??
    "postgres://polyphony:polyphony@localhost:5433/polyphony?sslmode=disable"

  execFileSync(
    "go",
    ["run", "./cmd/seed-tokens", "-email", email, "-amount", String(amount)],
    {
      cwd: SERVER_DIR,
      env: { ...process.env, DATABASE_URL: databaseUrl },
      stdio: "pipe",
      // Without this, a hung seed command (e.g. the DB never becoming
      // reachable) blocks Node's event loop indefinitely -- Playwright's own
      // test timeout can't interrupt a synchronous `execFileSync` call, so
      // the run would hang forever instead of failing with a clear timeout
      // error.
      timeout: 30_000,
    },
  )
}

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
 *   (isolated) compose test stack.
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

  // Both the initial "Send with AI" pass and the follow-up Vision-aware
  // regenerate (see `use-chat-room.ts`'s `handleSendWithAI`) draw from the
  // same canned stub fixture and so return textually identical replies --
  // asserting on the rendered text alone (as below) would pass even if the
  // regenerate never actually ran, or errored, since the first pass's reply
  // is already on screen. These two `waitForResponse` calls instead capture
  // the actual `POST /messages/ai` and `POST /messages/:id/regenerate`
  // response bodies the app itself receives, so the assertions after the
  // click can prove the regenerate genuinely re-ran against the same AI
  // message (matching `id`, but a *different* `updated_at`) rather than
  // merely reflecting the untouched first-pass row. Registered before the
  // click since both responses can arrive before `waitForResponse` would
  // otherwise start listening.
  const initialAiResponsePromise = page.waitForResponse(
    (res) => res.request().method() === "POST" && res.url().endsWith("/messages/ai"),
  )
  const regenerateResponsePromise = page.waitForResponse(
    (res) =>
      res.request().method() === "POST" &&
      /\/messages\/[^/]+\/regenerate$/.test(res.url()),
  )

  await sendWithAIButton.click()

  const [initialAiResponse, regenerateResponse] = await Promise.all([
    initialAiResponsePromise,
    regenerateResponsePromise,
  ])
  const { ai_message: initialAiMessage } = (await initialAiResponse.json()) as {
    ai_message: { id: string; updated_at: string }
  }
  const regeneratedMessage = (await regenerateResponse.json()) as {
    id: string
    updated_at: string
  }
  expect(regeneratedMessage.id).toBe(initialAiMessage.id)
  expect(regeneratedMessage.updated_at).not.toBe(initialAiMessage.updated_at)

  // Staging is reset() on a successful send, so the chip disappears.
  await expect(removeAttachmentButton).toHaveCount(0)

  await expect(page.getByText(messageContent).first()).toBeVisible()

  // The thumbnail renders inside the sent message bubble. The clickable
  // element is a `Button` (keyboard-accessible; the `Image` inside it is
  // decorative with an empty `alt`), so its accessible name comes from
  // `aria-label`, not the image.
  await expect(
    page.getByRole("button", { name: "Open message attachment" }),
  ).toBeVisible()

  // The AI's reply appears in the DOM too. The regenerate call having
  // genuinely re-run (not merely reflected the first pass) is already
  // proven above via the two responses' `id`/`updated_at`; the initial pass
  // and the Vision-aware regenerate both draw from the same canned stub
  // fixture (`llm-stub/fixtures/default.json`), so this text assertion on
  // its own would not distinguish the two passes. Longer timeout (wave-7):
  // this send is followed by a Vision regenerate -- under full-suite
  // parallelism the combined round trip can exceed the default 5s expect
  // timeout.
  await expect(
    page.getByText("This is a canned E2E stub response for testing purposes."),
  ).toBeVisible({ timeout: 15_000 })
})
