import { expect, test, type Page } from "@playwright/test"

/**
 * End-to-end coverage for Step 52's web room fork UI: triggering a fork
 * from `RoomSettingsDrawer`, observing the inline progress view resolve to
 * an "Open forked room" link, the copied room being fully usable once the
 * job completes, and the destination room's archived read-only treatment
 * while the copy job is still in flight.
 *
 * Registers a brand-new user and its own room per test (like
 * `ai-context-controls.spec.ts`), rather than reusing the shared fixture
 * user/room, so this spec's message-order assertions and its fork jobs
 * never have to account for content other specs have left behind in the
 * shared fixture room.
 */

interface CreatedRoom {
  roomName: string
}

/** Registers a brand-new user and creates a room named `${prefix} <runId>`, landing on that room's page. */
async function registerAndCreateRoom(
  page: Page,
  prefix: string,
): Promise<CreatedRoom> {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const email = `${prefix}-${runId}@polyphony.test`
  // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
  // rejects hyphens (see `smoke.spec.ts`/`auth.spec.ts`'s same convention).
  const username = `${prefix}_${runId}`
  const password = `${prefix}-test-password-123`
  const roomName = `Fork Test Room ${runId}`

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(email)
  await page.getByPlaceholder("johndoe").fill(username)
  await page.getByPlaceholder("Create a password").fill(password)
  await page.getByPlaceholder("Confirm your password").fill(password)
  await page.getByRole("button", { name: "Create account" }).click()
  await expect(page).toHaveURL(/\/rooms$/)

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  return { roomName }
}

/** Sends a plain message via the composer and waits for it to appear in the message list. */
async function sendPlainMessage(page: Page, content: string): Promise<void> {
  await page.getByPlaceholder("Ask me anything...").fill(content)
  await page.getByRole("button", { name: "Send", exact: true }).click()
  await expect(page.getByText(content).first()).toBeVisible()
}

/** Opens `RoomSettingsDrawer` via the header's "Room settings" button. */
async function openRoomSettings(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Room settings" }).click()
  await expect(page.getByText("Room settings")).toBeVisible()
}

test.describe("room fork", () => {
  test("fork trigger -> progress -> completed -> new room usable", async ({
    page,
  }) => {
    await registerAndCreateRoom(page, "fork")

    const firstMessage = `Fork message one ${Date.now()}`
    const secondMessage = `Fork message two ${Date.now()}`
    await sendPlainMessage(page, firstMessage)
    await sendPlainMessage(page, secondMessage)

    await openRoomSettings(page)
    await page.getByRole("button", { name: "Fork this room" }).click()

    // The progress view appears: either the indeterminate "Preparing…"
    // label (job still "pending") or the numeric "Copying messages… x / y"
    // line (job now "running") -- the small E2E fixture stack may move
    // through "pending" too quickly to reliably observe both, so either is
    // acceptable here.
    await expect(
      page.getByText(/Preparing…|Copying messages…/).first(),
    ).toBeVisible()

    // Poll/wait until the job completes (bounded wait, consistent with the
    // small message counts this spec produces).
    const openForkedRoomLink = page.getByRole("link", {
      name: "Open forked room",
    })
    await expect(openForkedRoomLink).toBeVisible({ timeout: 30_000 })

    await openForkedRoomLink.click()
    await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

    // The copied messages are visible in the same order as the source room.
    const firstMessageLocator = page.getByText(firstMessage).first()
    const secondMessageLocator = page.getByText(secondMessage).first()
    await expect(firstMessageLocator).toBeVisible()
    await expect(secondMessageLocator).toBeVisible()
    const firstBox = await firstMessageLocator.boundingBox()
    const secondBox = await secondMessageLocator.boundingBox()
    expect(firstBox).not.toBeNull()
    expect(secondBox).not.toBeNull()
    expect(firstBox!.y).toBeLessThan(secondBox!.y)

    // The new room is fully usable: `is_archived` has flipped to `false`,
    // so `MessageInput` renders (no archived banner) and a plain send
    // succeeds.
    await expect(page.getByText(/This room is archived/)).toHaveCount(0)
    const newMessage = `New room message ${Date.now()}`
    await sendPlainMessage(page, newMessage)
  })

  test("navigating directly to the fork destination before completion shows the archived, read-only state", async ({
    page,
  }) => {
    await registerAndCreateRoom(page, "fork-archived")
    await sendPlainMessage(page, "Seed message before forking")

    await openRoomSettings(page)

    const [forkResponse] = await Promise.all([
      page.waitForResponse(
        (res) =>
          /\/fork$/.test(res.url()) && res.request().method() === "POST",
      ),
      page.getByRole("button", { name: "Fork this room" }).click(),
    ])
    const forkBody = (await forkResponse.json()) as {
      new_room: { id: string }
    }
    const newRoomId = forkBody.new_room.id

    // Navigate directly to the destination room before its fork job has
    // had a chance to complete (a fresh page load, not the drawer's own
    // "Open forked room" link).
    await page.goto(`/rooms/${newRoomId}`)

    await expect(page.getByText(/This room is archived/)).toBeVisible()
    await expect(page.getByPlaceholder("Ask me anything...")).toHaveCount(0)
  })
})
