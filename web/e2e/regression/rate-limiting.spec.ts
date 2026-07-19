import { execSync } from "node:child_process"
import path from "node:path"
import { expect, test, type Page, type Response } from "@playwright/test"
import { creditTokenBalance } from "../support/credit-token-balance"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because `web/package.json` has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly.

/** Repo root, so the `docker compose` recreate below resolves regardless of
 * the shell's own working directory (this file lives two levels deeper than
 * the repo root's `docker-compose.yml`). */
const REPO_ROOT = path.resolve(__dirname, "../../../")

/** Base URL of the test-profile Go API, reachable from the host -- matches
 * `e2e/seed/seed.ts`'s own default. */
const E2E_API_URL = process.env.E2E_API_URL ?? "http://localhost:8090"

const HEALTH_POLL_TIMEOUT_MS = 30_000
const HEALTH_POLL_INTERVAL_MS = 1_000

/**
 * Polls `GET {E2E_API_URL}/health` until it responds 200, or throws once
 * `HEALTH_POLL_TIMEOUT_MS` has elapsed. Mirrors `e2e/seed/seed.ts`'s own
 * `waitForHealth` (not imported directly since that module has no exported
 * standalone helper of its own -- see that file's docstring for why its
 * `globalSetup` default export is the only thing it exposes), needed here
 * because recreating `api-e2e` briefly takes it off the health-checked
 * `Up`/healthy state while it reboots.
 */
async function waitForApiHealth(): Promise<void> {
  const deadline = Date.now() + HEALTH_POLL_TIMEOUT_MS
  let lastError: unknown

  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${E2E_API_URL}/health`)
      if (res.ok) return
      lastError = new Error(`/health returned HTTP ${res.status}`)
    } catch (err) {
      lastError = err
    }
    await new Promise((resolve) => setTimeout(resolve, HEALTH_POLL_INTERVAL_MS))
  }

  throw new Error(
    `E2E API at ${E2E_API_URL} did not become healthy within ${HEALTH_POLL_TIMEOUT_MS}ms: ${String(lastError)}`,
  )
}

/**
 * Recreates just the `api-e2e` container (`--no-deps`, so `db-e2e`/
 * `redis-e2e`/etc. are left untouched) under the isolated `polyphony-e2e`
 * compose project, optionally overriding `RATE_LIMIT_AI_INVOKE_PER_MINUTE_E2E`
 * in the child process's own environment. Passing `rateLimitOverride` as
 * `undefined` omits the variable entirely from the child env, letting
 * `docker-compose.yml`'s own `${RATE_LIMIT_AI_INVOKE_PER_MINUTE_E2E:-1000}`
 * default apply -- this is how the limit is restored, not by setting it back
 * to `1000` explicitly (which would stop tracking the compose file's own
 * documented default if it ever changes).
 *
 * Uses `execSync` directly (no process-spawn utility) to match
 * `e2e/seed/seed.ts`'s own plain-`fetch`/no-extra-dependency style.
 */
function recreateApiE2E(rateLimitOverride: string | undefined): void {
  const env = { ...process.env }
  if (rateLimitOverride === undefined) {
    delete env.RATE_LIMIT_AI_INVOKE_PER_MINUTE_E2E
  } else {
    env.RATE_LIMIT_AI_INVOKE_PER_MINUTE_E2E = rateLimitOverride
  }

  execSync(
    "docker compose -p polyphony-e2e --profile test up -d --no-deps api-e2e",
    { cwd: REPO_ROOT, env, stdio: "pipe" },
  )
}

/** Matches the BFF proxy's AI-invoke route regardless of whether the caller
 * used the non-streaming `POST /rooms/:roomId/messages/ai` endpoint or the
 * streaming `.../messages/ai/stream` variant -- both carry the same
 * `ai_invoke` rate limit (see `server/internal/app/routes_message.go`), and
 * this spec only cares about the HTTP-level 429/`Retry-After` contract, not
 * which of the two the UI happens to call. */
const AI_INVOKE_RESPONSE_PATTERN = /\/rooms\/[^/]+\/messages\/ai(\/stream)?(\?|$)/

/** Registers a brand-new user (matching `smoke.spec.ts`'s convention),
 * creates a room, and returns to the room's chat view -- used by both cases
 * below so neither depends on the shared fixture user/room's message
 * history or balance. */
async function registerUserAndRoom(
  page: Page,
  runId: string,
): Promise<{ email: string }> {
  const email = `rate-limit-${runId}@polyphony.test`
  // Short "rl_" prefix: registerSchema caps usernames at 32 characters, and
  // `rate_limit_${runId}` (with runId's 13-digit-ms timestamp + random
  // suffix + case tag) is 33 -- registration could never succeed (caught
  // live by the wave-7 integration run).
  const username = `rl_${runId}`
  const password = "rate-limit-test-password-123"
  const roomName = `Rate Limit Test Room ${runId}`

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

  return { email }
}

/**
 * Regression coverage for Step 33's Redis-backed AI-invoke rate limit,
 * against the real E2E compose profile: (1) that its relaxed defaults
 * (`RATE_LIMIT_AI_INVOKE_PER_MINUTE_E2E`, default `1000`/min) never
 * spuriously trip this suite's own ordinary traffic, and (2) that a
 * deliberately tightened limit produces a correct HTTP 429 +
 * numeric `Retry-After`, restored afterward.
 *
 * Marked `serial` (not `fullyParallel`'s default) so Case 2's brief
 * `api-e2e` container recreate can never overlap with Case 1's own
 * traffic within this file, and so the whole file runs as one uninterrupted
 * unit in a single worker regardless of how many workers the overall run
 * uses.
 */
test.describe("rate limiting", () => {
  test.describe.configure({ mode: "serial" })

  test("Case 1: the E2E profile's relaxed limits do not trip ordinary suite-level traffic", async ({
    page,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}_c1`
    const statuses: number[] = []
    page.on("response", (res) => {
      if (AI_INVOKE_RESPONSE_PATTERN.test(res.url())) {
        statuses.push(res.status())
      }
    })

    await registerUserAndRoom(page, runId)

    const composer = page.getByPlaceholder("Ask me anything...")
    const sendButton = page.getByRole("button", { name: "Send", exact: true })
    const sendWithAIButton = page.getByRole("button", { name: "Send with AI" })

    // A handful of plain sends, well under any per-minute limit.
    for (let i = 0; i < 5; i++) {
      await composer.fill(`Plain burst message ${i} ${runId}`)
      await sendButton.click()
      await expect(page.getByText(`Plain burst message ${i} ${runId}`).first()).toBeVisible()
    }

    // A handful of AI sends (falls back to the `default` fixture -- no
    // marker needed), still well under the relaxed default.
    for (let i = 0; i < 3; i++) {
      // Fill *before* asserting enabled: the button is disabled while the
      // composer is empty, and every send clears the composer (caught live
      // by the wave-7 integration run).
      await composer.fill(`AI burst message ${i} ${runId}`)
      await expect(sendWithAIButton).toBeEnabled()
      await sendWithAIButton.click()
      await expect(page.getByText(`AI burst message ${i} ${runId}`).first()).toBeVisible()
    }

    expect(statuses.length).toBeGreaterThan(0)
    expect(statuses.every((status) => status !== 429)).toBe(true)
  })

  test.describe("Case 2: a deliberately tightened limit returns 429 with Retry-After", () => {
    test.beforeAll(async () => {
      // Recreate just `api-e2e` with a tight per-minute AI-invoke limit for
      // this isolated run. `--no-deps` leaves `db-e2e`/`redis-e2e`/etc.
      // untouched, and this bracket (beforeAll/afterAll, not per-test) keeps
      // the tightened window as short as possible.
      recreateApiE2E("3")
      await waitForApiHealth()
    })

    test.afterAll(async () => {
      // Restore the relaxed default so no other spec file sharing this
      // running stack (including this suite's own Case 1, if re-run) is
      // left rate-limited.
      recreateApiE2E(undefined)
      await waitForApiHealth()
    })

    test("AI invocations past the tightened limit receive HTTP 429 with a numeric Retry-After header", async ({
      page,
    }) => {
      const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}_c2`
      await registerUserAndRoom(page, runId)

      const composer = page.getByPlaceholder("Ask me anything...")
      const sendWithAIButton = page.getByRole("button", { name: "Send with AI" })

      let limitedResponse: Response | undefined
      const MAX_ATTEMPTS = 8

      for (let i = 0; i < MAX_ATTEMPTS && !limitedResponse; i++) {
        // Fill *before* asserting enabled: the button is disabled while the
        // composer is empty, and every send clears the composer (caught
        // live by the wave-7 integration run, same as Case 1).
        await composer.fill(`Tightened limit probe ${i} ${runId}`)
        await expect(sendWithAIButton).toBeEnabled()

        const [response] = await Promise.all([
          page.waitForResponse(
            (res) =>
              AI_INVOKE_RESPONSE_PATTERN.test(res.url()) &&
              res.request().method() === "POST",
          ),
          sendWithAIButton.click(),
        ])

        if (response.status() === 429) {
          limitedResponse = response
        } else {
          // A successful send re-enables the button once its optimistic
          // state settles; give the UI a moment to catch up before the next
          // attempt's `toBeEnabled()` check.
          await expect(page.getByText(`Tightened limit probe ${i} ${runId}`).first()).toBeVisible()
        }
      }

      expect(limitedResponse).toBeDefined()
      expect(limitedResponse?.status()).toBe(429)

      const retryAfterHeader = limitedResponse?.headers()["retry-after"]
      expect(retryAfterHeader).toBeDefined()
      const retryAfterSeconds = Number(retryAfterHeader)
      expect(Number.isInteger(retryAfterSeconds)).toBe(true)
      // The server clamps Retry-After to >= 1 (never 0), so >= 0 is looser
      // than the actual contract and would silently pass a 0 regression.
      expect(retryAfterSeconds).toBeGreaterThanOrEqual(1)
    })
  })
})
