import { expect, test, type Page } from "@playwright/test"

/**
 * End-to-end coverage for plain room CRUD (Step 36's `RoomSettingsDrawer`,
 * triggered by `ChatRoomHeader`'s "more" button): create -> rename ->
 * delete for the owning `master`, plus a role-gated check that only the
 * room's `master` sees the "Delete room" control (an `admin` invitee sees
 * the rename fields but not the delete action). No earlier step shipped a
 * dedicated spec for this surface (`ChatRoom.tsx`'s settings-drawer wiring
 * landed in Step 36, after Step 37's `members.spec.ts` was written).
 *
 * Every identity here is a brand-new, ad hoc user registered through the UI
 * in its own browser context -- no dependency on the seeded fixture
 * user/room (`e2e/support/fixtures.ts`), matching `smoke.spec.ts`'s and
 * `members.spec.ts`'s convention so this spec has no ordering dependency on
 * other specs or the seed routine.
 */

interface RegisteredUser {
  email: string
  username: string
  password: string
}

/** Registers a brand-new user through the rendered Kratos-flow `RegisterForm` and lands on `/rooms`. */
async function registerUser(page: Page, prefix: string): Promise<RegisteredUser> {
  const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`
  const user: RegisteredUser = {
    email: `${prefix}-${runId}@polyphony.test`,
    // Underscores only: registerSchema's username regex (`/^[a-zA-Z0-9_]+$/`)
    // rejects hyphens (see `smoke.spec.ts`/`members.spec.ts`'s same convention).
    username: `${prefix}_${runId}`,
    password: `${prefix}-test-password-123`,
  }

  await page.goto("/register")
  await page.getByPlaceholder("you@example.com").fill(user.email)
  await page.getByPlaceholder("johndoe").fill(user.username)
  await page.getByPlaceholder("Create a password").fill(user.password)
  await page.getByPlaceholder("Confirm your password").fill(user.password)
  await page.getByRole("button", { name: "Create account" }).click()
  // Longer timeout, matching members.spec.ts/groups.spec.ts's carryover fix:
  // under parallel worker load this navigation can exceed the default 5s.
  await expect(page).toHaveURL(/\/rooms$/, { timeout: 15_000 })

  return user
}

/** Creates a room via `CreateRoomForm` and opens it, landing on `/rooms/[roomId]`. */
async function createAndOpenRoom(page: Page, roomName: string) {
  await page.getByRole("button", { name: "New Room" }).first().click()
  await page.getByPlaceholder("e.g., Product Strategy").fill(roomName)
  await page.getByRole("button", { name: "Create room" }).click()
  await page.getByRole("heading", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)
}

/**
 * As the currently-open room's viewer, invites `invitee` by exact username
 * with `role`, via the member avatar stack -> member panel -> invite dialog
 * (same flow `members.spec.ts` already exercises).
 */
async function inviteByUsername(page: Page, invitee: string, role: string) {
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

/** As the invitee, opens the invitations bell, accepts the pending invitation, and waits for the room to load. */
async function acceptFromInbox(page: Page) {
  await page.getByRole("button", { name: "Invitations" }).click()
  await page.getByRole("button", { name: "Accept" }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)
}

test.describe("Room settings: create, rename, delete, and RBAC gating", () => {
  test("owning master can create, rename, and delete a room", async ({ page }) => {
    const runId = Date.now()
    const roomName = `Rooms Spec Room ${runId}`
    const renamedName = `Rooms Spec Room ${runId} Renamed`

    await registerUser(page, "owner")
    await createAndOpenRoom(page, roomName)

    // Open the settings drawer and confirm this is the caller's `master`
    // room: rename controls (`admin`+) and the master-only "Delete room"
    // control are both present.
    await page.getByRole("button", { name: "Room settings" }).click()
    const nameInput = page.getByRole("textbox", { name: "Room name" })
    await expect(nameInput).toBeVisible()
    await expect(nameInput).toHaveValue(roomName)
    await expect(page.getByRole("button", { name: "Delete room" })).toBeVisible()

    // Rename: edit the name and description, then save. Waits for the
    // underlying `PUT /rooms/:roomId` proxy request to actually resolve
    // before proceeding, rather than just firing the click, so the
    // subsequent navigate-away-and-back assertion isn't racing the mutation.
    await nameInput.fill(renamedName)
    await page
      .getByRole("textbox", { name: "Description" })
      .fill("Renamed via rooms.spec.ts")
    const updateResponse = page.waitForResponse(
      (res) => res.url().includes("/api/proxy/rooms/") && res.request().method() === "PUT",
    )
    await page.getByRole("button", { name: "Save" }).first().click()
    const updateRes = await updateResponse
    expect(updateRes.ok()).toBe(true)

    // `exact: true`: the drawer also has an icon close-trigger whose
    // accessible name is "Close room settings", which non-exact (substring)
    // role matching would also hit, tripping strict mode.
    await page.getByRole("button", { name: "Close", exact: true }).click()

    // Navigate away and back (rather than just relying on the in-place
    // update) so the assertion also covers the renamed room surviving a
    // fresh fetch of the room list/detail, not just the mutation's optimistic
    // cache write.
    await page.goto("/rooms")
    await expect(
      page.getByRole("navigation", { name: "Rooms" }).getByText(renamedName),
    ).toBeVisible()
    // Click + URL assertion run inside a `toPass` retry: right after
    // `page.goto("/rooms")`, Next.js may not have finished hydrating yet, so
    // a click that lands before the `Link`'s client-side handlers attach is
    // silently lost (a plain anchor navigation never fires, since the
    // element is present in the DOM but not yet interactive). Retrying the
    // click-then-assert pair recovers once hydration completes, matching the
    // `toPass` deflake pattern already used in `room-fork.spec.ts`.
    await expect(async () => {
      await page
        .getByRole("navigation", { name: "Rooms" })
        .getByText(renamedName)
        .click()
      await expect(page).toHaveURL(/\/rooms\/[^/]+$/, { timeout: 2_000 })
    }).toPass({ timeout: 15_000 })
    await expect(page.getByRole("heading", { name: renamedName })).toBeVisible()

    // Delete: reopen the drawer, invoke "Delete room" behind its
    // confirmation dialog, and confirm.
    await page.getByRole("button", { name: "Room settings" }).click()
    await page.getByRole("button", { name: "Delete room" }).click()
    const confirmDialog = page.getByRole("dialog", { name: "Delete this room?" })
    await expect(confirmDialog).toBeVisible()
    await confirmDialog.getByRole("button", { name: "Yes, delete room" }).click()

    await expect(page).toHaveURL(/\/rooms$/)
    await expect(
      page.getByRole("navigation", { name: "Rooms" }).getByText(renamedName),
    ).toHaveCount(0)
  })

  test("an invited admin sees rename controls but not the master-only delete action", async ({
    page: ownerPage,
    browser,
  }) => {
    const roomName = `Rooms Spec RBAC Room ${Date.now()}`

    await registerUser(ownerPage, "rbacowner")
    await createAndOpenRoom(ownerPage, roomName)

    const adminContext = await browser.newContext()
    const adminPage = await adminContext.newPage()
    const admin = await registerUser(adminPage, "rbacadmin")

    await inviteByUsername(ownerPage, admin.username, "admin")
    await acceptFromInbox(adminPage)

    await adminPage.getByRole("button", { name: "Room settings" }).click()
    await expect(
      adminPage.getByRole("textbox", { name: "Room name" }),
    ).toBeVisible()
    await expect(
      adminPage.getByRole("button", { name: "Delete room" }),
    ).toHaveCount(0)

    await adminContext.close()
  })
})
