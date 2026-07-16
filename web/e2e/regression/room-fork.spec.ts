import { execFileSync } from "node:child_process"
import path from "node:path"
import { expect, test, type Page } from "@playwright/test"

// NOTE: this spec must stay CommonJS-compatible (no `import.meta`):
// Playwright transpiles e2e specs to CJS because `web/package.json` has no
// `"type": "module"` -- see `attachments.spec.ts`'s identical note.

/**
 * Step 59's dedicated room-fork regression suite: the full trigger ->
 * live-progress -> archived-destination -> completed -> usable journey,
 * plus an RBAC regression check on the fork trigger's visibility, exercised
 * end to end on top of every wave-5/6/7 UI change layered onto Step 32/36's
 * server usecase and Step 52's own, narrower feature spec
 * (`web/e2e/room-fork.spec.ts`).
 *
 * Every identity here is a brand-new, ad hoc user registered through the UI
 * in its own test (or browser context) -- no dependency on the shared
 * fixture user/room, matching `smoke.spec.ts`'s convention, so this spec's
 * message-order assertions and its fork jobs never have to account for
 * content other specs have left behind in the shared fixture room.
 */

// Both tests below wait (bounded, up to 30s) for a server-side fork job to
// complete inside their own bodies, which does not fit in Playwright's
// default 30s per-test budget under full-suite parallelism -- same
// carryover as Step 52's own `room-fork.spec.ts`.
test.describe.configure({ timeout: 60_000 })

/** Repo root, so the `docker compose exec` below resolves the compose file
 * regardless of the shell's own working directory (this file lives two
 * levels deeper than the repo root's `docker-compose.yml`). */
const REPO_ROOT = path.resolve(__dirname, "../../../")

const AI_PLACEHOLDER = "Ask me anything..."

/**
 * Sets `rooms.is_archived` for `roomId` directly in the e2e stack's
 * Postgres (same out-of-band-state convention as `attachments.spec.ts`'s
 * `seed-tokens` shell-out, and identical to Step 52's own
 * `room-fork.spec.ts`'s `setRoomArchived` helper). Used to re-create the
 * transient archived-destination window deterministically: the fork copy
 * job runs in an immediate server-side goroutine and finishes a tiny
 * fixture room's copy in milliseconds, so a live navigation essentially
 * never observes the destination room while it is still archived -- and the
 * room page's data is RSC-prefetched server-side (60s `staleTime`), so
 * client-side response interception cannot fake it either. Flipping the
 * real row exercises the full real pipeline (server prefetch -> hydration
 * -> `ChatRoom`'s archived conditional) with genuine server state.
 */
function setRoomArchived(roomId: string, archived: boolean): void {
  execFileSync(
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
      "-c",
      `UPDATE rooms SET is_archived = ${archived} WHERE id = '${roomId}'`,
    ],
    { cwd: REPO_ROOT, stdio: "pipe" },
  )
}

interface RegisteredUser {
  username: string
}

/** Registers a brand-new user and creates a room named `${prefix} <runId>`, landing on that room's page. */
async function registerAndCreateRoom(
  page: Page,
  prefix: string,
): Promise<{ user: RegisteredUser; roomName: string }> {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `${prefix}-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens (see `smoke.spec.ts`/`auth.spec.ts`'s same convention).
  const username = `${prefix}_${runId}`
  const password = `${prefix}-test-password-123`
  const roomName = `Fork Regression Room ${runId}`

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  return { user: { username }, roomName }
}

/**
 * Sends a plain message via the composer and waits for it to appear in the
 * message list. Fill-and-click runs inside a `toPass` retry (see Step 52's
 * `room-fork.spec.ts`'s identical helper for the full rationale: a late
 * query-settling re-render can remount `MessageInput` right after
 * navigating into a fresh/forked room and wipe a just-typed draft).
 */
async function sendPlainMessage(page: Page, content: string): Promise<void> {
  await expect(async () => {
    await page.getByPlaceholder(AI_PLACEHOLDER).fill(content)
    await page
      .getByRole("button", { name: "Send", exact: true })
      .click({ timeout: 2_000 })
  }).toPass({ timeout: 20_000 })
  await expect(page.getByText(content).first()).toBeVisible()
}

/** Opens `RoomSettingsDrawer` via the header's "Room settings" button. */
async function openRoomSettings(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Room settings" }).click()
  await expect(page.getByText("Room settings")).toBeVisible()
}

/**
 * As the currently-open room's viewer, invites `invitee` by exact username
 * with `role`, via the member avatar stack -> member panel -> invite
 * dialog, and asserts the dialog reports success. Copied from
 * `members.spec.ts`'s `inviteByUsername` (see that file's doc comment) --
 * this step's own conflict notes call out `web/e2e/support/` as off-limits,
 * so this helper is duplicated here rather than imported.
 */
async function inviteByUsername(page: Page, invitee: string, role: string): Promise<void> {
  await page.getByRole("button", { name: "Room members" }).click()
  await expect(page.getByRole("heading", { name: "Members", exact: true })).toBeVisible()

  await page.getByRole("button", { name: "Invite" }).click()
  await page.getByPlaceholder("exact username").fill(invitee)
  if (role !== "member") {
    await page.locator("select").first().selectOption(role)
  }
  await page.getByRole("button", { name: "Send invitation" }).click()
  await expect(page.getByText("Invitation sent.")).toBeVisible()

  await page.getByRole("button", { name: "Close" }).click()
  await page.keyboard.press("Escape")
}

/** As the invitee, opens the invitations bell and accepts the pending invitation, landing on the room. */
async function acceptFromInbox(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Invitations" }).click()
  await page.getByRole("button", { name: "Accept" }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)
}

test.describe("room fork regression", () => {
  test("fork trigger -> progress -> archived destination -> completed -> new room fully usable", async ({
    page,
  }) => {
    await registerAndCreateRoom(page, "forkreg")

    const firstMessage = `Fork regression message one ${Date.now()}`
    const secondMessage = `Fork regression message two ${Date.now()}`
    await sendPlainMessage(page, firstMessage)
    await sendPlainMessage(page, secondMessage)

    await openRoomSettings(page)

    const [forkResponse] = await Promise.all([
      page.waitForResponse(
        (res) => /\/fork$/.test(res.url()) && res.request().method() === "POST",
      ),
      page.getByRole("button", { name: "Fork this room" }).click(),
    ])
    const forkBody = (await forkResponse.json()) as {
      new_room: { id: string; is_archived: boolean }
    }
    const newRoomId = forkBody.new_room.id

    // The server's own 202 asserts the destination room *starts* archived --
    // Step 32's wire contract, and the part of "archived until the job
    // completes" that can be observed race-free from the response itself.
    expect(forkBody.new_room.is_archived).toBe(true)

    // The progress view appears: either the indeterminate "Preparing…"
    // label (job still "pending") or the numeric "Copying messages… x / y"
    // line (job now "running") -- the small E2E fixture stack may move
    // through "pending" too quickly to reliably observe both.
    await expect(
      page.getByText(/Preparing…|Copying messages…/).first(),
    ).toBeVisible()

    // Wait for the job to actually complete (bounded wait), then follow the
    // drawer's own "Open forked room" link -- the completion-criterion path
    // a user actually takes.
    const openForkedRoomLink = page.getByRole("link", { name: "Open forked room" })
    await expect(openForkedRoomLink).toBeVisible({ timeout: 30_000 })
    await openForkedRoomLink.click()
    await expect(page).toHaveURL(new RegExp(`/rooms/${newRoomId}$`))

    // The copied messages are visible in the same order as the source room.
    // Order is asserted on the rendered list's text content rather than via
    // `boundingBox()` y-coordinates (see Step 52's `room-fork.spec.ts` for
    // the flake this avoids).
    await expect(page.getByText(firstMessage).first()).toBeVisible()
    await expect(page.getByText(secondMessage).first()).toBeVisible()
    const listText = await page.locator('[data-testid="message-list"]').textContent()
    expect(listText).not.toBeNull()
    expect(listText!.indexOf(firstMessage)).toBeGreaterThanOrEqual(0)
    expect(listText!.indexOf(firstMessage)).toBeLessThan(listText!.indexOf(secondMessage))

    // The new room is fully usable: no archived banner, `MessageInput`
    // renders, and a plain send succeeds.
    await expect(page.getByText(/This room is archived/)).toHaveCount(0)
    const newMessage = `New room message ${Date.now()}`
    await sendPlainMessage(page, newMessage)

    // Re-create the transient archived-destination window deterministically
    // (see `setRoomArchived`'s doc comment for why the live window cannot be
    // observed by navigation, since the job above has already completed)
    // and assert the read-only treatment: message history visible, no
    // `MessageInput`, an explanatory banner shown.
    setRoomArchived(newRoomId, true)
    await page.reload()

    await expect(page.getByText(/This room is archived/)).toBeVisible()
    await expect(page.getByPlaceholder(AI_PLACEHOLDER)).toHaveCount(0)
    await expect(page.getByText(firstMessage).first()).toBeVisible()
    await expect(page.getByText(secondMessage).first()).toBeVisible()

    // Flip the real row back off (as the completed job's worker already
    // did) and confirm the room is writable again with no manual
    // workaround.
    setRoomArchived(newRoomId, false)
    await page.reload()
    await expect(page.getByPlaceholder(AI_PLACEHOLDER)).toBeVisible()
    await expect(page.getByText(/This room is archived/)).toHaveCount(0)
  })

  test("a member-role room member never sees the fork trigger", async ({ page, browser }) => {
    await registerAndCreateRoom(page, "forkrbac")

    // Master sees "Fork room"/"Fork this room" -- sanity check for contrast
    // against the member-role assertion below.
    await openRoomSettings(page)
    await expect(page.getByText("Fork room")).toBeVisible()
    await expect(page.getByRole("button", { name: "Fork this room" })).toBeVisible()
    await page.keyboard.press("Escape")

    // Invite a second user with the `member` role (own browser context, own
    // cookies -- matching `members.spec.ts`'s multi-context convention).
    const memberContext = await browser.newContext()
    const memberPage = await memberContext.newPage()
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
    const memberUsername = `forkmem_${runId}`
    await memberPage.goto("/register")
    await memberPage.getByPlaceholder("you@example.com").fill(`forkmem-${runId}@polyphony.test`)
    await memberPage.getByPlaceholder("johndoe").fill(memberUsername)
    await memberPage.getByPlaceholder("Create a password").fill("forkmem-test-password-123")
    await memberPage.getByPlaceholder("Confirm your password").fill("forkmem-test-password-123")
    await memberPage.getByRole("button", { name: "Create account" }).click()
    await expect(memberPage).toHaveURL(/\/rooms$/, { timeout: 15_000 })

    await inviteByUsername(page, memberUsername, "member")
    await acceptFromInbox(memberPage)

    // As a `member` (not admin/master), the member sees the message input
    // (RBAC allows sending) but `RoomSettingsDrawer`'s `canManage`-gated
    // "Fork room" section -- heading and trigger both -- is absent entirely.
    await expect(memberPage.getByPlaceholder(AI_PLACEHOLDER)).toBeVisible()
    await openRoomSettings(memberPage)
    await expect(memberPage.getByText("Fork room")).toHaveCount(0)
    await expect(memberPage.getByRole("button", { name: "Fork this room" })).toHaveCount(0)

    await memberContext.close()
  })
})
