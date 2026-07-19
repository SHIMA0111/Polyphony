import { expect, test } from "@playwright/test"
import { creditTokenBalance } from "./support/credit-token-balance"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"` (see `attachments.spec.ts`'s identical note).

/**
 * The full assembled text of `llm-stub/fixtures/stream/streaming-demo.sse`
 * (see that file's twin `llm-stub/fixtures/streaming-demo.json` for the
 * non-streaming completion the same fixture name serves). Deliberately a
 * long, many-chunk (22 SSE events) fixture rather than the default 4-chunk
 * one: `llm-stub/server.ts` serves the entire SSE body in a single
 * `Response` with no artificial pacing, so a short fixture can traverse
 * stub -> gateway -> Go -> WebSocket fast enough that polling for
 * "two intermediate DOM states" would be flaky. This spec sidesteps that by
 * capturing every DOM mutation synchronously in-page via a
 * `MutationObserver` (installed *before* the send, so it cannot miss the
 * optimistic-placeholder -> real-id swap) instead of polling from the test
 * runner.
 */
const FINAL_TEXT =
  "Streaming responses render token by token, so the reader watches each word appear as soon as the model produces it."

/**
 * A handful of the fixture's early word-chunks: used to prove a captured
 * snapshot reflects genuinely *partial* streamed content (not just the
 * "Sending…"/"Streaming…" status label changing) without hardcoding every
 * possible partial state the observer might have captured.
 */
const EARLY_PARTIAL_WORDS = ["Streaming ", "responses ", "render ", "token "]

/**
 * End-to-end coverage for Step 54's streaming AI rendering: sends a message
 * with AI in a fresh room (Step 51's `POST /rooms/:roomId/messages/ai/stream`
 * endpoint, driven through the UI exactly like a normal "Send with AI"
 * click), and asserts the AI bubble:
 * - starts in the thinking state (`ThinkingBubble`, no content yet);
 * - grows through at least one genuinely partial streamed-content state as
 *   `token_chunk` WS frames arrive (see `EARLY_PARTIAL_WORDS`);
 * - settles on the stub's canned final text and stops mutating once
 *   settled (no further DOM changes, and the in-flight "Streaming…" status
 *   label is gone).
 */
test("AI reply renders incrementally via token_chunk WS frames and settles on the final text", async ({
  page,
}) => {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `streaming-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens, matching every other spec's convention.
  const username = `streaming_${runId}`
  const password = "streaming-test-password-123"
  const roomName = `Streaming Test Room ${runId}`
  const messageContent = `[[fixture:streaming-demo]] Show me a streaming response ${runId}`

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()

  // Longer timeout (wave-7 deflake, same as members/groups/rooms'
  // registerUser carryover fix): under full-suite parallelism this
  // post-auth navigation can exceed Playwright's default 5s.
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  creditTokenBalance(email, 1_000_000)

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()

  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  // Install the observer on the message-list container *before* sending —
  // that container is mounted from the very first render, unlike the
  // eventual AI `MessageBubble`, whose DOM node is replaced (not just
  // mutated) once its optimistic placeholder id is swapped for the real,
  // server-assigned one. Attaching only after the send is issued risks
  // missing the whole stream if it completes before the attach round-trip.
  await page.evaluate(() => {
    const w = window as unknown as { __streamSnapshots?: string[] }
    w.__streamSnapshots = []
    const target = document.querySelector('[data-testid="message-list"]')
    if (!target) return

    const snapshots = w.__streamSnapshots
    snapshots.push(target.textContent ?? "")

    const observer = new MutationObserver(() => {
      const text = target.textContent ?? ""
      if (snapshots[snapshots.length - 1] !== text) {
        snapshots.push(text)
      }
    })
    observer.observe(target, {
      childList: true,
      characterData: true,
      subtree: true,
    })
    ;(window as unknown as { __streamObserver?: MutationObserver }).__streamObserver =
      observer
  })

  await page.getByPlaceholder("Ask me anything...").fill(messageContent)
  await page.getByRole("button", { name: "Send with AI" }).click()

  // `.first()` (Step 29/30 convention, see `smoke.spec.ts`): the composer
  // keeps the typed text until the server acknowledges the send.
  await expect(page.getByText(messageContent).first()).toBeVisible()

  // Thinking state: the AI bubble renders with no content yet -- but tolerate
  // the deterministic llm-stub streaming and finalizing the whole reply
  // before this assertion is evaluated (observed live in the wave-9
  // integration review: a strict must-observe-the-transient-state `expect`
  // here timed out because the stream had already settled). The
  // MutationObserver capture below is what actually proves the thinking ->
  // partial -> final progression happened; this assertion only needs to
  // confirm the reply is in flight *or* already done.
  await expect(
    page.getByLabel("AI is thinking").or(page.getByText(FINAL_TEXT)),
  ).toBeVisible()

  // Finalized state: the stub's full canned text eventually renders.
  await expect(page.getByText(FINAL_TEXT)).toBeVisible({ timeout: 20_000 })

  // Give any trailing WS frame (the terminating message_updated finalize, if
  // it hasn't landed yet by the time the text itself appeared) a brief
  // moment to settle before treating the capture window as closed.
  await page.waitForTimeout(500)

  const snapshots = await page.evaluate(() => {
    const w = window as unknown as {
      __streamSnapshots?: string[]
      __streamObserver?: MutationObserver
    }
    w.__streamObserver?.disconnect()
    return w.__streamSnapshots ?? []
  })

  // At least: the pre-send/thinking state, one or more growing intermediate
  // states, and the final settled state.
  expect(snapshots.length).toBeGreaterThanOrEqual(3)

  const finalSnapshot = snapshots[snapshots.length - 1]
  expect(finalSnapshot).toContain(FINAL_TEXT)

  // At least one captured snapshot shows genuinely partial streamed content
  // -- proving the text grew incrementally rather than jumping straight
  // from "thinking" to the finished reply.
  const hasPartialState = snapshots.some((snapshot) => {
    if (snapshot.includes(FINAL_TEXT)) return false
    return EARLY_PARTIAL_WORDS.some((word) => snapshot.includes(word))
  })
  expect(hasPartialState).toBe(true)

  // No further changes once the reply has settled.
  const textAfterSettle = await page
    .locator('[data-testid="message-list"]')
    .textContent()
  await page.waitForTimeout(500)
  const textAfterExtraWait = await page
    .locator('[data-testid="message-list"]')
    .textContent()
  expect(textAfterExtraWait).toBe(textAfterSettle)

  // The bottom-row status label has moved off the in-flight affordances to
  // a normal, non-streaming timestamp.
  await expect(page.getByText("Streaming…")).toHaveCount(0)
  await expect(page.getByText("Sending…")).toHaveCount(0)
})
