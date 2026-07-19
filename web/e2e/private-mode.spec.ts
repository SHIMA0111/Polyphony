import { execFileSync } from "node:child_process"
import path from "node:path"
import { expect, test, type Page } from "@playwright/test"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly (see
// `attachments.spec.ts`'s identical note).

/** `server/`, so `go run ./cmd/seed-tokens` below resolves relative to the
 * Go module root regardless of the shell's own working directory. */
const SERVER_DIR = path.resolve(__dirname, "../../server")

/**
 * Credits `email`'s token balance directly against the e2e stack's Postgres
 * database, by shelling out to `server/cmd/seed-tokens` -- the same
 * `BillingRepository.CreditAndRecord` production code path `task
 * billing:topup` drives (see `attachments.spec.ts`'s identical helper for
 * the full rationale). A brand-new user's balance is lazily created at zero
 * (`BillingUsecase.GetOrCreateBalance`), so "Send with AI" 402s unless
 * topped up first -- this spec's private-mode sender needs a real,
 * successful AI exchange to assert the badge/isolation on, not a rejected
 * one.
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

interface RegisteredUser {
  email: string
  username: string
  password: string
}

/** Registers a brand-new user through the rendered Kratos-flow `RegisterForm` and lands on `/rooms` (see `members.spec.ts`'s identical helper). */
async function registerUser(page: Page, prefix: string): Promise<RegisteredUser> {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const user: RegisteredUser = {
    email: `${prefix}-${runId}@polyphony.test`,
    // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
    // rejects hyphens (see `smoke.spec.ts`/`members.spec.ts`'s convention).
    username: `${prefix}_${runId}`,
    password: `${prefix}-test-password-123`,
  }

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(user.email)
  await page.getByPlaceholder("johndoe").fill(user.username)
  await page.getByPlaceholder("Create a password").fill(user.password)
  await page.getByPlaceholder("Confirm your password").fill(user.password)
  await page.getByRole("button", { name: "Create account" }).click()
  // Longer timeout (wave-7 deflake, same as members/groups/rooms'
  // registerUser carryover fix): under full-suite parallelism this
  // post-auth navigation can exceed Playwright's default 5s.
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  return user
}

/**
 * As the currently-open room's viewer, invites `invitee` by exact username
 * with the default `member` role, via the member avatar stack -> member
 * panel -> invite dialog (see `members.spec.ts`'s identical helper).
 */
async function inviteByUsername(page: Page, invitee: string) {
  await page.getByRole("button", { name: "Room members" }).click()
  await expect(
    page.getByRole("heading", { name: "Members", exact: true }),
  ).toBeVisible()

  await page.getByRole("button", { name: "Invite" }).click()
  await page.getByPlaceholder("exact username").fill(invitee)
  await page.getByRole("button", { name: "Send invitation" }).click()
  await expect(page.getByText("Invitation sent.")).toBeVisible()

  await page.getByRole("button", { name: "Close" }).click()
  await page.keyboard.press("Escape")
}

/** As the invitee, opens the invitations bell, accepts the pending invitation, and waits for the room to load. */
async function acceptFromInbox(page: Page) {
  await page.getByRole("button", { name: "Invitations" }).click()
  await page.getByRole("button", { name: "Accept" }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)
}

/**
 * End-to-end coverage for Step 47's private AI mode UI: proves a private
 * exchange (both the human prompt and the AI reply) is visible only to its
 * own sender in a shared room, while ordinary messages still deliver
 * normally to every other member -- the web counterpart to Step 41's
 * server-side per-user `MessageHub` targeting.
 *
 * Two independent, brand-new users (matching `members.spec.ts`'s
 * two-browser-context pattern) are placed in the same room: Alice creates
 * it and invites Bob, who accepts from his own browser context (separate
 * cookies/session, simulating a genuinely different client). Alice is
 * credited a token balance first (see `creditTokenBalance`) so her "Send
 * with AI" actually succeeds instead of 402ing.
 */
test.describe("Private AI mode", () => {
  test("a private AI exchange is visible only to its sender, while ordinary messages still deliver", async ({
    page: alicePage,
    browser,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
    const roomName = `Private Mode Test Room ${runId}`
    const privateContent = `What's my confidential salary question ${runId}`
    const publicContent = `An ordinary, non-private message ${runId}`

    // (a) Alice registers, is credited a balance, and creates a room.
    const alice = await registerUser(alicePage, "alice")
    creditTokenBalance(alice.email, 1_000_000)

    await alicePage.getByRole("button", { name: "New Room" }).first().click()
    await alicePage.getByPlaceholder("e.g., Product Strategy").fill(roomName)
    await alicePage.getByRole("button", { name: "Create room" }).click()
    await alicePage.getByRole("heading", { name: roomName }).click()
    await expect(alicePage).toHaveURL(/\/rooms\/[^/]+$/)

    // (b) Bob registers in a separate browser context (own cookies/session).
    const bobContext = await browser.newContext()
    const bobPage = await bobContext.newPage()
    const bob = await registerUser(bobPage, "bob")

    // (c) Alice invites Bob and he accepts, landing both users in the same room.
    await inviteByUsername(alicePage, bob.username)
    await acceptFromInbox(bobPage)
    await expect(bobPage.getByPlaceholder("Ask me anything...")).toBeVisible()

    // (d) Alice toggles private mode on and sends an AI message.
    const privateToggle = alicePage.getByRole("button", {
      name: "Private mode off",
    })
    await expect(privateToggle).toBeVisible()
    await privateToggle.click()
    await expect(
      alicePage.getByRole("button", { name: "Private mode on" }),
    ).toBeVisible()

    await alicePage.getByPlaceholder("Ask me anything...").fill(privateContent)
    await alicePage.getByRole("button", { name: "Send with AI" }).click()

    // (e) Alice's own view shows the private badge on both the prompt and
    // the AI's reply (the Step 10 LLM stub's canned fixture response).
    await expect(alicePage.getByText(privateContent).first()).toBeVisible()
    await expect(
      alicePage.getByText(
        "This is a canned E2E stub response for testing purposes.",
      ),
    ).toBeVisible()
    await expect(alicePage.getByText("Private", { exact: true })).toHaveCount(2)

    // The toggle resets to off after the send.
    await expect(
      alicePage.getByRole("button", { name: "Private mode off" }),
    ).toBeVisible()

    // (f) Bob's view -- polled for a few seconds to allow for any WS
    // delivery -- never shows either half of the private exchange, neither
    // live (WS) nor after a manual refresh (REST list filter, Step 41).
    await bobPage.waitForTimeout(3000)
    await expect(bobPage.getByText(privateContent)).toHaveCount(0)
    await expect(
      bobPage.getByText("This is a canned E2E stub response for testing purposes."),
    ).toHaveCount(0)

    await bobPage.reload()
    await expect(bobPage.getByPlaceholder("Ask me anything...")).toBeVisible()
    await expect(bobPage.getByText(privateContent)).toHaveCount(0)
    await expect(
      bobPage.getByText("This is a canned E2E stub response for testing purposes."),
    ).toHaveCount(0)

    // (g) A subsequent ordinary (non-private) message from Alice still
    // delivers to Bob promptly -- the WS connection and normal delivery are
    // otherwise unaffected by private mode. Bob's socket must be connected
    // *before* Alice sends: the reload above tore down Bob's WebSocket, a
    // live-delivered message has no refetch fallback, and a send landing
    // during Bob's reconnect window is silently missed (caught flaking live
    // by the wave-7 integration run; same ordering rule as
    // `regression/two-client-realtime.spec.ts`).
    await expect(
      bobPage.locator('[aria-label="Connection status: Connected"]'),
    ).toBeVisible()
    await alicePage.getByPlaceholder("Ask me anything...").fill(publicContent)
    await alicePage
      .getByRole("button", { name: "Send", exact: true })
      .click()
    await expect(alicePage.getByText(publicContent).first()).toBeVisible()
    // Longer timeout (wave-7 deflake): Bob's copy arrives over a live WS
    // broadcast, which can exceed the default 5s under full-suite
    // parallelism (two browser contexts + Kratos registrations competing
    // for CPU) even though delivery itself is otherwise prompt.
    await expect(bobPage.getByText(publicContent).first()).toBeVisible({
      timeout: 15_000,
    })

    await bobContext.close()
  })
})
