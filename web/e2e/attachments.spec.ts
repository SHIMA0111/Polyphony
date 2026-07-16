import { execFileSync } from "node:child_process"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { expect, test } from "@playwright/test"

const __dirname = path.dirname(fileURLToPath(import.meta.url))

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

  // The chip appears immediately; "Send with AI" is enabled once the
  // (fast, local MinIO) upload finishes.
  await expect(removeAttachmentButton).toBeVisible()
  await expect(sendWithAIButton).toBeEnabled()

  await page.getByPlaceholder("Ask me anything...").fill(messageContent)
  await sendWithAIButton.click()

  // Staging is reset() on a successful send, so the chip disappears.
  await expect(removeAttachmentButton).toHaveCount(0)

  await expect(page.getByText(messageContent).first()).toBeVisible()

  // The thumbnail renders inside the sent message bubble.
  await expect(
    page.getByRole("img", { name: "Message attachment" }),
  ).toBeVisible()

  // The AI's reply appears -- the initial pass and the Vision-aware
  // regenerate both draw from the same canned stub fixture
  // (`llm-stub/fixtures/default.json`), so this asserts the final reply
  // text is present rather than distinguishing the two passes textually.
  await expect(
    page.getByText("This is a canned E2E stub response for testing purposes."),
  ).toBeVisible()
})
