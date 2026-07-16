import { execFileSync } from "node:child_process"
import path from "node:path"
import { expect, test } from "@playwright/test"
import { seedLongHistoryRoom } from "../../support/seed-long-history"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly (see
// `attachments.spec.ts`'s identical note).

/** `server/`, so `go run ./cmd/seed-tokens` below resolves relative to the
 * Go module root regardless of the shell's own working directory. */
const SERVER_DIR = path.resolve(__dirname, "../../../../server")

/** Repo root, so the `docker compose exec` below resolves the compose file
 * regardless of the shell's own working directory (see `room-fork.spec.ts`'s
 * identical constant). */
const REPO_ROOT = path.resolve(__dirname, "../../../../")

/**
 * Credits `email`'s token balance directly against the e2e stack's Postgres
 * database, by shelling out to `server/cmd/seed-tokens` -- see
 * `attachments.spec.ts`'s identical helper for the full rationale. A
 * brand-new user's balance is lazily created at zero, so "Send with AI"
 * 402s unless topped up first. The stub's canned `usage` block
 * (`llm-stub/fixtures/default.json`) is a fixed, tiny 22-token debit
 * regardless of how large the actual prompt was, so this amount is far more
 * than either AI send in this spec will ever need -- generous headroom, not
 * a precise budget.
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
 * Counts `message_context_summaries` rows for `roomId` directly against the
 * e2e stack's Postgres (same `docker compose exec ... psql` convention as
 * `room-fork.spec.ts`'s `setRoomArchived`), so this spec's summarization
 * assertion is backed by genuine server-side cache-write evidence, not just
 * the client-rendered badge.
 */
function countContextSummaryRows(roomId: string): number {
  const output = execFileSync(
    "docker",
    [
      "compose",
      "-p",
      "polyphony-e2e",
      "--profile",
      "test",
      "exec",
      "-T",
      "db-e2e",
      "psql",
      "-U",
      "polyphony",
      "-d",
      "polyphony",
      "-t",
      "-c",
      `SELECT COUNT(*) FROM message_context_summaries WHERE room_id = '${roomId}'`,
    ],
    { cwd: REPO_ROOT, stdio: ["ignore", "pipe", "pipe"], encoding: "utf-8" },
  )
  return parseInt(output.trim(), 10)
}

/**
 * End-to-end regression coverage tying Step 50's server-side context
 * summarization to Step 50/54's client-visible "Summarized history" badge
 * (`MessageBubble.tsx`, rendered when `message.used_context_summary` is
 * `true` on an AI message -- see that file's existing render for the exact
 * field/text this spec asserts against, already shipped and unit-tested
 * before this step; no new UI wiring was needed).
 *
 * A brand-new user registers through the rendered UI (establishing a real
 * browser/Kratos session used for every subsequent request in this test),
 * then `seedLongHistoryRoom` (via the standalone `request` fixture, direct
 * REST against the Go API, reusing the same login) builds a room whose
 * accumulated history comfortably exceeds Step 50's actual, discovered
 * summarization threshold. The final AI-triggering send goes through the
 * non-streaming `POST /api/proxy/rooms/:roomId/messages/ai` (page.request,
 * sharing the browser's own Kratos session cookie via the Next.js BFF proxy)
 * rather than the UI's streaming "Send with AI" button -- the API-driven
 * path the step doc calls out as preferred for setup speed, and one that
 * sidesteps any streaming-timing flakiness entirely for this assertion.
 *
 * The room page is opened (with its WebSocket connected) *before* that
 * final send: `used_context_summary` is, by Step 50's shipped design, a
 * one-time, request-scoped signal describing how a message was *generated*
 * -- `handler.MessageResponse.UsedContextSummary`'s doc comment pins that
 * list/historical reads always report `false`, so the badge is only ever
 * client-visible on the live delivery (the send response or the WS
 * `message_created`/`message_updated` frame), never on a cold refetch
 * (wave-8 review fix: this spec originally asserted the badge after a fresh
 * `page.goto`, which the shipped contract explicitly never renders).
 */
test.describe("Summarization regression", () => {
  test("a long-history room triggers server-side summarization and renders the badge; a fresh short room does not", async ({
    page,
    request,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
    const email = `summarization-${runId}@polyphony.test`
    // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
    // rejects hyphens, matching every other spec's convention. Prefix kept
    // short: registerSchema also caps usernames at 32 characters, and the
    // runId alone is up to 19 (`summarization_` + runId reached 33 and
    // failed client-side validation — wave-8 review fix).
    const username = `summ_${runId}`
    const password = "summarization-test-password-123"

    await page.goto("/register")
    await page.getByPlaceholder("you@example.com").fill(email)
    await page.getByPlaceholder("johndoe").fill(username)
    await page.getByPlaceholder("Create a password").fill(password)
    await page.getByPlaceholder("Confirm your password").fill(password)
    await page.getByRole("button", { name: "Create account" }).click()
    // Longer timeout (wave-7 deflake, matching every other spec's
    // registerUser convention): under full-suite parallelism this post-auth
    // navigation can exceed Playwright's default 5s.
    await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

    creditTokenBalance(email, 1_000_000)

    // (a) Seed a long-history room via direct REST, reusing this same
    // user/password (the helper logs in rather than registering again).
    const roomName = `Summarization Test Room ${runId}`
    const seeded = await seedLongHistoryRoom(request, {
      userEmail: email,
      userPassword: password,
      roomName,
    })
    expect(seeded.messageCount).toBeGreaterThan(0)
    expect(seeded.estimatedTokens).toBeGreaterThan(0)

    // (b) Open the room page and wait for its WebSocket to connect *before*
    // the AI-triggering send: `used_context_summary` is a one-time,
    // request-scoped signal (see this file's header comment), so the badge
    // only ever renders for a live-delivered message -- never on a cold
    // refetch of history.
    await page.goto(`/rooms/${seeded.roomId}`)
    await expect(
      page.locator('[aria-label="Connection status: Connected"]'),
    ).toBeVisible({ timeout: 15_000 })

    // (c) The final AI-triggering send: non-streaming, API-driven, sharing
    // the browser's own Kratos session cookie via the BFF proxy.
    const aiRes = await page.request.post(
      `/api/proxy/rooms/${seeded.roomId}/messages/ai`,
      { data: { content: `Please summarize our discussion so far ${runId}` } },
    )
    expect(aiRes.ok()).toBe(true)
    const aiBody = (await aiRes.json()) as {
      ai_message: { used_context_summary: boolean; status: string }
    }
    expect(aiBody.ai_message.status).toBe("completed")
    expect(aiBody.ai_message.used_context_summary).toBe(true)

    // (d) Stronger-than-badge evidence: a summary was actually cached
    // server-side for this room (Step 50's `message_context_summaries`).
    expect(countContextSummaryRows(seeded.roomId)).toBeGreaterThanOrEqual(1)

    // (e) The live WS delivery (`message_created` carrying
    // `used_context_summary: true`) renders the "Summarized history" badge
    // on the AI exchange in the already-open room page.
    await expect(page.getByText("Summarized history")).toBeVisible({
      timeout: 15_000,
    })

    // (f) A fresh, short room (a couple of messages, no seeding) does NOT
    // show the badge -- proving it is conditional, not always-on for every
    // AI reply.
    const shortRoomRes = await page.request.post("/api/proxy/rooms", {
      data: {
        name: `Summarization Control Room ${runId}`,
        description: "A short, unsummarized room for the badge-absence control.",
      },
    })
    expect(shortRoomRes.ok()).toBe(true)
    const shortRoom = (await shortRoomRes.json()) as { id: string }

    const shortAiRes = await page.request.post(
      `/api/proxy/rooms/${shortRoom.id}/messages/ai`,
      { data: { content: `A short, ordinary question ${runId}` } },
    )
    expect(shortAiRes.ok()).toBe(true)
    const shortAiBody = (await shortAiRes.json()) as {
      ai_message: { used_context_summary: boolean; status: string }
    }
    expect(shortAiBody.ai_message.used_context_summary).toBe(false)

    await page.goto(`/rooms/${shortRoom.id}`)
    await expect(
      page.getByText("This is a canned E2E stub response for testing purposes."),
    ).toBeVisible()
    await expect(page.getByText("Summarized history")).toHaveCount(0)
  })

  /**
   * The exact H1 post-review regression, exercised on the DEFAULT streaming
   * path (the UI's "Send with AI" button, `POST .../messages/ai/stream`):
   * during streaming, each `token_chunk` WS frame carries
   * `summary_used: true` and the badge renders early -- but before the H1
   * fix, `consumeAIStream`'s terminating `message_updated` publish omitted
   * `UsedContextSummary`, so the finalize frame's wholesale replace silently
   * WIPED the already-rendered badge the moment the stream settled. This
   * test asserts the badge is (still) visible strictly *after* the stream
   * has finalized (final text rendered, no "Streaming…" affordance left).
   */
  test("the 'Summarized history' badge survives stream finalize on the default streaming send path", async ({
    page,
    request,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
    const email = `summ-stream-${runId}@polyphony.test`
    // Underscores only + short prefix: see the first test's username comment.
    const username = `summstr_${runId}`
    const password = "summarization-stream-password-123"

    await page.goto("/register")
    await page.getByPlaceholder("you@example.com").fill(email)
    await page.getByPlaceholder("johndoe").fill(username)
    await page.getByPlaceholder("Create a password").fill(password)
    await page.getByPlaceholder("Confirm your password").fill(password)
    await page.getByRole("button", { name: "Create account" }).click()
    await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

    creditTokenBalance(email, 1_000_000)

    const roomName = `Summarization Streaming Room ${runId}`
    const seeded = await seedLongHistoryRoom(request, {
      userEmail: email,
      userPassword: password,
      roomName,
    })
    expect(seeded.messageCount).toBeGreaterThan(0)

    await page.goto(`/rooms/${seeded.roomId}`)
    await expect(
      page.locator('[aria-label="Connection status: Connected"]'),
    ).toBeVisible({ timeout: 15_000 })

    // Explicitly select GPT-5 Mini before sending: `seedLongHistoryRoom`
    // calibrates the room's history against gpt-5-mini's 272k context
    // window (see that helper's SUMMARIZATION_TOKEN_BUDGET), but the UI's
    // ModelSelector defaults to the gateway's *first* listed model
    // (gpt-5.2, 400k window -- see `MessageInput.tsx`'s `models[0]`
    // fallback), whose larger budget the seeded history does not reliably
    // overflow. The first test never hits this because its API-driven send
    // omits `model` entirely, falling through to the server-side
    // DEFAULT_AI_MODEL (gpt-5-mini). Selector-driving convention copied
    // from `regression/ai-send-model-select.spec.ts`.
    const modelTrigger = page
      .getByRole("button", { name: "Attach image" })
      .locator("xpath=following-sibling::button[1]")
    await modelTrigger.click()
    await page.getByText("GPT-5 Mini", { exact: true }).click()
    await expect(
      page.getByRole("button", { name: "GPT-5 Mini", exact: true }),
    ).toBeVisible()

    // The default streaming path: through the rendered UI, not page.request.
    await page
      .getByPlaceholder("Ask me anything...")
      .fill(`Please summarize our discussion so far ${runId}`)
    await page.getByRole("button", { name: "Send with AI" }).click()

    // Wait for the stream to fully finalize: the stub's default streamed
    // reply text is rendered and no streaming affordance remains -- i.e. the
    // terminating `message_updated` frame has already been merged.
    await expect(page.getByText("This is a canned SSE stub response.")).toBeVisible({
      timeout: 20_000,
    })
    await expect(page.getByText("Streaming…")).toHaveCount(0)
    // Give any trailing WS frame a brief moment to settle (matching
    // `regression/advanced-ai/streaming.spec.ts`'s convention), so the badge
    // assertion below genuinely runs post-finalize rather than racing it.
    await page.waitForTimeout(500)

    // The H1 assertion: the badge is visible AFTER finalize, not merely at
    // some point during streaming.
    await expect(page.getByText("Summarized history")).toBeVisible()
  })
})
