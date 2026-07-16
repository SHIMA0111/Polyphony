import { execFileSync } from "node:child_process"
import path from "node:path"
import { expect, test, type Page } from "@playwright/test"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because web/package.json has no
// `"type": "module"`, so the ambient CJS `__dirname` is used directly (see
// `attachments.spec.ts`'s identical note).

/** `server/`, so `go run ./cmd/seed-tokens` below resolves relative to the
 * Go module root regardless of the shell's own working directory. */
const SERVER_DIR = path.resolve(__dirname, "../../../../server")

/**
 * Credits `email`'s token balance directly against the e2e stack's Postgres
 * database -- see `web/e2e/attachments.spec.ts`'s identical helper for the
 * full rationale.
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

/** Registers a brand-new user through the rendered Kratos-flow `RegisterForm` and lands on `/rooms` (see `web/e2e/private-mode.spec.ts`'s identical helper). `prefix` must be 12 characters or fewer: registerSchema caps usernames at 32, and the runId alone is up to 19 (wave-8 review fix — `advpriv_alice_` + runId reached 33 and failed client-side validation). */
async function registerUser(page: Page, prefix: string): Promise<RegisteredUser> {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const user: RegisteredUser = {
    email: `${prefix}-${runId}@polyphony.test`,
    // Underscores only: registerSchema's username regex
    // (`/^[a-zA-Z0-9_]+$/`) rejects hyphens.
    username: `${prefix}_${runId}`,
    password: `${prefix}-test-password-123`,
  }

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(user.email)
  await page.getByPlaceholder("johndoe").fill(user.username)
  await page.getByPlaceholder("Create a password").fill(user.password)
  await page.getByPlaceholder("Confirm your password").fill(user.password)
  await page.getByRole("button", { name: "Create account" }).click()
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  return user
}

/**
 * As the currently-open room's viewer, invites `invitee` by exact username
 * with the default `member` role, via the member avatar stack -> member
 * panel -> invite dialog (see `web/e2e/members.spec.ts`/`private-mode.spec.ts`'s
 * identical helper).
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

interface MessageListEnvelope {
  messages: Array<{ content: string; visibility: string }>
  next_cursor: string | null
}

/**
 * Fetches the room's message list as `viewerPage`'s own authenticated user
 * (`page.request` shares the browser context's own session cookie, exactly
 * like `billing-checkout.spec.ts`'s proxy calls), bypassing any client-side
 * rendering entirely -- a stronger regression check than asserting on the
 * DOM alone, since a client-side rendering bug could theoretically hide a
 * row the server actually sent.
 */
async function fetchMessages(viewerPage: Page, roomId: string): Promise<MessageListEnvelope> {
  const res = await viewerPage.request.get(
    `/api/proxy/rooms/${roomId}/messages?limit=50`,
  )
  expect(res.ok()).toBe(true)
  return (await res.json()) as MessageListEnvelope
}

/**
 * Broader regression coverage for Step 47's private AI mode UI, layered on
 * top of Step 47's own `web/e2e/private-mode.spec.ts` (still run
 * unmodified alongside this spec): adds a raw, authenticated `GET` of the
 * room's message list as the non-sender (not merely a rendered-UI
 * assertion, re-verifying Step 41's server-side per-user visibility
 * filtering holds at the wire level) and re-checks every assertion after
 * both clients do a full page reload, proving the private state survives a
 * fresh fetch rather than only the original WebSocket delivery.
 */
test.describe("Private AI mode regression", () => {
  test("a private exchange stays invisible to a second user in rendered UI, raw REST, and after reload; ordinary delivery is unaffected", async ({
    page: alicePage,
    browser,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
    const roomName = `Private Mode Regression Room ${runId}`
    const privateContent = `A confidential private question ${runId}`
    const publicContent = `An ordinary, non-private message ${runId}`

    // (a) Alice registers, is credited a balance, and creates a room.
    const alice = await registerUser(alicePage, "apriv_alice")
    creditTokenBalance(alice.email, 1_000_000)

    await alicePage.getByRole("button", { name: "New Room" }).first().click()
    await alicePage.getByPlaceholder("e.g., Product Strategy").fill(roomName)
    await alicePage.getByRole("button", { name: "Create room" }).click()
    await alicePage.getByRole("heading", { name: roomName }).click()
    await expect(alicePage).toHaveURL(/\/rooms\/[^/]+$/)

    const roomIdMatch = /\/rooms\/([^/]+)$/.exec(alicePage.url())
    if (!roomIdMatch) throw new Error(`could not extract room id from URL: ${alicePage.url()}`)
    const roomId = roomIdMatch[1]

    // (b) Bob registers in a separate browser context (own cookies/session).
    const bobContext = await browser.newContext()
    const bobPage = await bobContext.newPage()
    const bob = await registerUser(bobPage, "apriv_bob")

    // (c) Alice invites Bob and he accepts, landing both users in the same room.
    await inviteByUsername(alicePage, bob.username)
    await acceptFromInbox(bobPage)
    await expect(bobPage.getByPlaceholder("Ask me anything...")).toBeVisible()

    // (d) Alice toggles private mode on and sends an AI message.
    const privateToggle = alicePage.getByRole("button", { name: "Private mode off" })
    await expect(privateToggle).toBeVisible()
    await privateToggle.click()
    await expect(alicePage.getByRole("button", { name: "Private mode on" })).toBeVisible()

    await alicePage.getByPlaceholder("Ask me anything...").fill(privateContent)
    await alicePage.getByRole("button", { name: "Send with AI" }).click()

    await expect(alicePage.getByText(privateContent).first()).toBeVisible()
    await expect(
      alicePage.getByText("This is a canned E2E stub response for testing purposes."),
    ).toBeVisible()
    await expect(alicePage.getByText("Private", { exact: true })).toHaveCount(2)

    // (e) Bob's rendered UI -- polled for a few seconds for any WS delivery
    // -- never shows either half of the private exchange.
    await bobPage.waitForTimeout(3000)
    await expect(bobPage.getByText(privateContent)).toHaveCount(0)
    await expect(
      bobPage.getByText("This is a canned E2E stub response for testing purposes."),
    ).toHaveCount(0)

    // (f) Stronger check: a raw, authenticated GET of the room's message
    // list as Bob's own user contains no row for either half of the private
    // exchange -- not merely hidden by client-side rendering.
    const bobRawMessages = await fetchMessages(bobPage, roomId)
    expect(
      bobRawMessages.messages.some((m) => m.content === privateContent),
    ).toBe(false)
    expect(
      bobRawMessages.messages.some(
        (m) => m.content === "This is a canned E2E stub response for testing purposes.",
      ),
    ).toBe(false)

    // (g) Both users reload; the same invisibility/visibility holds against
    // a fresh fetch, not just the original WebSocket delivery.
    await alicePage.reload()
    await expect(alicePage.getByPlaceholder("Ask me anything...")).toBeVisible()
    await expect(alicePage.getByText(privateContent).first()).toBeVisible()
    await expect(
      alicePage.getByText("This is a canned E2E stub response for testing purposes."),
    ).toBeVisible()
    await expect(alicePage.getByText("Private", { exact: true })).toHaveCount(2)

    await bobPage.reload()
    await expect(bobPage.getByPlaceholder("Ask me anything...")).toBeVisible()
    await expect(bobPage.getByText(privateContent)).toHaveCount(0)
    await expect(
      bobPage.getByText("This is a canned E2E stub response for testing purposes."),
    ).toHaveCount(0)

    const bobRawMessagesAfterReload = await fetchMessages(bobPage, roomId)
    expect(
      bobRawMessagesAfterReload.messages.some((m) => m.content === privateContent),
    ).toBe(false)
    expect(
      bobRawMessagesAfterReload.messages.some(
        (m) => m.content === "This is a canned E2E stub response for testing purposes.",
      ),
    ).toBe(false)

    // (h) A subsequent ordinary (non-private) message from Alice still
    // delivers to Bob promptly. Bob's socket must be connected *before*
    // Alice sends (both reloads above tore down each page's WebSocket).
    await expect(
      bobPage.locator('[aria-label="Connection status: Connected"]'),
    ).toBeVisible()
    await alicePage.getByPlaceholder("Ask me anything...").fill(publicContent)
    await alicePage.getByRole("button", { name: "Send", exact: true }).click()
    await expect(alicePage.getByText(publicContent).first()).toBeVisible()
    await expect(bobPage.getByText(publicContent).first()).toBeVisible({
      timeout: 15_000,
    })

    await bobContext.close()
  })
})
