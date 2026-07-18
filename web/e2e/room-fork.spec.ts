import { execFileSync } from "node:child_process"
import path from "node:path"
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

// Both tests wait (bounded, up to 30s) for a server-side fork job to
// complete inside their own bodies, which does not fit in Playwright's
// default 30s per-test budget under full-suite parallelism.
test.describe.configure({ timeout: 60_000 })

/** Repo root, so the `docker compose exec` below resolves the compose file
 * regardless of the shell's own working directory. */
const REPO_ROOT = path.resolve(__dirname, "../../")

/**
 * Sets `rooms.is_archived` for `roomId` directly in the e2e stack's
 * Postgres (same out-of-band-state convention as `attachments.spec.ts`'s
 * `seed-tokens` shell-out). Used to re-create the transient
 * archived-destination window deterministically: the fork copy job runs in
 * an immediate server-side goroutine and finishes a tiny fixture room's
 * copy in milliseconds, so a live navigation essentially never observes the
 * destination room while it is still archived (caught live by the wave-7
 * integration run) -- and the room page's data is RSC-prefetched
 * server-side (60s `staleTime`), so client-side response interception
 * cannot fake it either. Flipping the real row exercises the full real
 * pipeline (server prefetch -> hydration -> `ChatRoom`'s archived
 * conditional) with genuine server state.
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
  // Longer timeout (wave-7 deflake, same as members/groups/rooms'
  // registerUser carryover fix): under full-suite parallelism this
  // post-auth navigation can exceed Playwright's default 5s.
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)

  return { roomName }
}

/**
 * Sends a plain message via the composer and waits for it to appear in the
 * message list. Fill-and-click runs inside a `toPass` retry: right after
 * navigating into a freshly-created (or freshly-forked) room, a late
 * query-settling re-render can remount `MessageInput` and wipe a just-typed
 * draft, leaving the Send button disabled forever from a single fill's
 * perspective (caught live by the wave-7 integration run) -- re-filling on
 * retry makes the send robust to that one-time wipe.
 */
async function sendPlainMessage(page: Page, content: string): Promise<void> {
  await expect(async () => {
    await page.getByPlaceholder("Ask me anything...").fill(content)
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
    // Order is asserted on the rendered list's text content rather than via
    // `boundingBox()` y-coordinates: the list re-renders as the fresh room's
    // queries settle, and a `boundingBox()` call racing one of those
    // re-renders returns `null` for a momentarily detached node (caught
    // flaking live by the wave-7 integration run).
    await expect(page.getByText(firstMessage).first()).toBeVisible()
    await expect(page.getByText(secondMessage).first()).toBeVisible()
    const listText = await page
      .locator('[data-testid="message-list"]')
      .textContent()
    expect(listText).not.toBeNull()
    expect(listText!.indexOf(firstMessage)).toBeGreaterThanOrEqual(0)
    expect(listText!.indexOf(firstMessage)).toBeLessThan(
      listText!.indexOf(secondMessage),
    )

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
    // "archived", not "fork-archived": the helper derives the username as
    // `${prefix}_${runId}`, and a "fork-archived" prefix both exceeds
    // registerSchema's 32-character username cap (33 chars with a
    // 13-digit-ms runId) and contains a hyphen its `/^[a-zA-Z0-9_]+$/`
    // regex rejects -- registration could never succeed (caught live by the
    // wave-7 integration run).
    await registerAndCreateRoom(page, "archived")
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
      new_room: { id: string; is_archived: boolean }
    }
    const newRoomId = forkBody.new_room.id

    // The server's own 202 asserts the destination room *starts* archived
    // -- that is Step 32's wire contract, and the part of "archived until
    // the job completes" that can be observed race-free.
    expect(forkBody.new_room.is_archived).toBe(true)

    // Wait for the job to actually complete (the drawer's own progress view
    // resolving) before re-creating the archived window below -- otherwise
    // the worker's final `SetArchived(false)` could race the UPDATE.
    await expect(
      page.getByRole("link", { name: "Open forked room" }),
    ).toBeVisible({ timeout: 30_000 })

    // Re-create the transient archived-destination state deterministically
    // (see setRoomArchived's doc comment for why the live window cannot be
    // observed by navigation) and assert the read-only treatment: banner
    // shown, no composer, history still visible.
    setRoomArchived(newRoomId, true)
    await page.goto(`/rooms/${newRoomId}`)

    await expect(page.getByText(/This room is archived/)).toBeVisible()
    await expect(page.getByPlaceholder("Ask me anything...")).toHaveCount(0)
    await expect(
      page.getByText("Seed message before forking").first(),
    ).toBeVisible()

    // Once `is_archived` flips back off (as the completed job's worker did
    // for the real row), a fresh load of the same room is writable again
    // with no manual workaround -- Step 52's un-archive completion
    // criterion.
    setRoomArchived(newRoomId, false)
    await page.reload()
    await expect(page.getByPlaceholder("Ask me anything...")).toBeVisible()
    await expect(page.getByText(/This room is archived/)).toHaveCount(0)
  })
})
