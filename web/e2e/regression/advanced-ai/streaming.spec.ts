import { execFileSync } from "node:child_process"
import path from "node:path"
import { expect, test } from "@playwright/test"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly (see
// `attachments.spec.ts`, which this spec's balance-crediting helper and the
// MutationObserver technique below are both copied/extended from
// `streaming.spec.ts`, Step 54's own per-feature spec).

/** `server/`, so `go run ./cmd/seed-tokens` below resolves relative to the
 * Go module root regardless of the shell's own working directory. */
const SERVER_DIR = path.resolve(__dirname, "../../../../server")

/**
 * Credits `email`'s token balance directly against the e2e stack's Postgres
 * database -- see `web/e2e/attachments.spec.ts`'s identical helper for the
 * full rationale. Credited generously enough to cover both this spec's
 * initial streamed send and its follow-up regenerate.
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
 * The full assembled text of `llm-stub/fixtures/stream/streaming-demo.sse`,
 * whose non-streaming twin `llm-stub/fixtures/streaming-demo.json` serves
 * the identical final text for the follow-up regenerate call below (see
 * `llm-stub/server.ts`'s `extractFixtureName`: it re-derives the same
 * `streaming-demo` fixture name from the still-present `[[fixture:...]]`
 * marker on the original human message).
 */
const FINAL_TEXT =
  "Streaming responses render token by token, so the reader watches each word appear as soon as the model produces it."

/** A handful of the fixture's early word-chunks, to prove a captured snapshot reflects genuinely partial content. */
const EARLY_PARTIAL_WORDS = ["Streaming ", "responses ", "render ", "token "]

/**
 * Broader regression coverage for Step 54's streaming AI rendering,
 * layered on top of Step 54's own `web/e2e/streaming.spec.ts` (still run
 * unmodified alongside this spec): reuses that spec's exact fixture
 * (`[[fixture:streaming-demo]]`) and in-page `MutationObserver` technique
 * (necessary because `llm-stub/server.ts` serves the whole SSE body in one
 * unpaced `Response`, so test-runner-side DOM polling could miss every
 * intermediate frame), and additionally proves the wave-7-review carryover
 * fix pairs correctly with streaming: the AI message's Regenerate button is
 * disabled for as long as `status === "streaming"` (server:
 * `RegenerateAIMessage` now rejects a streaming target with
 * `domain.ErrConflict`/409; client: `MessageBubble`'s Regenerate button is
 * `disabled={isStreaming}`) and becomes enabled and genuinely functional
 * again once the message has settled into its final, stable state.
 */
test("AI reply streams incrementally, disables Regenerate while in flight, and settles into a functional final state", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `adv-streaming-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens, matching every other spec's convention. Prefix kept
  // short: registerSchema also caps usernames at 32 characters, and the
  // runId alone is up to 19 (`adv_streaming_` + runId reached 33 and failed
  // client-side validation — wave-8 review fix).
  const username = `advstr_${runId}`
  const password = "adv-streaming-test-password-123"
  const roomName = `Advanced Streaming Test Room ${runId}`
  const messageContent = `[[fixture:streaming-demo]] Show me a streaming response ${runId}`

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

  // Install the observer *before* sending, so it cannot miss the
  // optimistic-placeholder -> real-id swap or any early chunk. Captures
  // both distinct content snapshots (as Step 54's own spec does) and
  // whether a disabled Regenerate button was ever observed while the
  // "Streaming…" status label was showing -- the carryover fix's actual
  // client-visible effect.
  await page.evaluate(() => {
    const w = window as unknown as {
      __streamSnapshots?: string[]
      __sawDisabledRegenerateWhileStreaming?: boolean
      __streamObserver?: MutationObserver
    }
    w.__streamSnapshots = []
    w.__sawDisabledRegenerateWhileStreaming = false
    const target = document.querySelector('[data-testid="message-list"]')
    if (!target) return

    const check = () => {
      const text = target.textContent ?? ""
      const snapshots = w.__streamSnapshots as string[]
      if (snapshots[snapshots.length - 1] !== text) {
        snapshots.push(text)
      }
      if (text.includes("Streaming…")) {
        const regenerateButton = Array.from(target.querySelectorAll("button")).find(
          (b) => b.textContent?.includes("Regenerate"),
        )
        if (regenerateButton?.disabled) {
          w.__sawDisabledRegenerateWhileStreaming = true
        }
      }
    }
    check()

    const observer = new MutationObserver(check)
    observer.observe(target, {
      childList: true,
      characterData: true,
      attributes: true,
      subtree: true,
    })
    w.__streamObserver = observer
  })

  await page.getByPlaceholder("Ask me anything...").fill(messageContent)
  await page.getByRole("button", { name: "Send with AI" }).click()

  await expect(page.getByText(messageContent).first()).toBeVisible()
  await expect(page.getByLabel("AI is thinking")).toBeVisible()
  await expect(page.getByText(FINAL_TEXT)).toBeVisible({ timeout: 20_000 })

  // Give any trailing WS frame a brief moment to settle before reading back
  // the capture.
  await page.waitForTimeout(500)

  const capture = await page.evaluate(() => {
    const w = window as unknown as {
      __streamSnapshots?: string[]
      __sawDisabledRegenerateWhileStreaming?: boolean
      __streamObserver?: MutationObserver
    }
    w.__streamObserver?.disconnect()
    return {
      snapshots: w.__streamSnapshots ?? [],
      sawDisabledRegenerateWhileStreaming: w.__sawDisabledRegenerateWhileStreaming ?? false,
    }
  })

  // At least: pre-send/thinking, one or more growing intermediate states,
  // and the final settled state.
  expect(capture.snapshots.length).toBeGreaterThanOrEqual(3)

  const finalSnapshot = capture.snapshots[capture.snapshots.length - 1]
  expect(finalSnapshot).toContain(FINAL_TEXT)

  const hasPartialState = capture.snapshots.some((snapshot) => {
    if (snapshot.includes(FINAL_TEXT)) return false
    return EARLY_PARTIAL_WORDS.some((word) => snapshot.includes(word))
  })
  expect(hasPartialState).toBe(true)

  // The carryover fix's client-visible effect: Regenerate was disabled for
  // at least one observed moment while the message was actually streaming.
  expect(capture.sawDisabledRegenerateWhileStreaming).toBe(true)

  // No further DOM changes once the reply has settled.
  const textAfterSettle = await page
    .locator('[data-testid="message-list"]')
    .textContent()
  await page.waitForTimeout(500)
  const textAfterExtraWait = await page
    .locator('[data-testid="message-list"]')
    .textContent()
  expect(textAfterExtraWait).toBe(textAfterSettle)

  // The finalized bubble reads as a normal, non-streaming steady state: no
  // lingering pulsing-cursor/streaming affordance.
  await expect(page.getByText("Streaming…")).toHaveCount(0)
  await expect(page.getByText("Sending…")).toHaveCount(0)

  // Regenerate is present, enabled, and genuinely functional once settled
  // -- proving streaming did not leave the message in a degraded/partial
  // state (the carryover guard only ever rejected it while still in
  // flight).
  const regenerateButton = page.getByRole("button", { name: /regenerate/i })
  await expect(regenerateButton).toBeEnabled()

  // `FINAL_TEXT` is already visible from the original streamed reply, so
  // asserting it right after the click would trivially pass even if
  // Regenerate never actually re-ran. Set up the response listener *before*
  // the click and await it (same `page.waitForResponse` pattern as
  // `attachments.spec.ts`'s regenerate coverage) so this proves a genuine
  // second round trip against `regenerateAIMessage`'s non-streaming
  // `POST .../regenerate` endpoint completed.
  const regenerateResponsePromise = page.waitForResponse(
    (res) =>
      res.request().method() === "POST" &&
      new URL(res.url()).pathname.endsWith("/regenerate"),
  )
  await regenerateButton.click()
  await regenerateResponsePromise
  await expect(page.getByText(FINAL_TEXT)).toBeVisible({ timeout: 20_000 })
})
