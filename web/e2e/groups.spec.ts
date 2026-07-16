import { expect, test, type Page } from "@playwright/test"

/**
 * End-to-end coverage for Step 46's group management UI: the full group
 * CRUD + membership + batch-invite-by-group lifecycle, including the
 * skip-reason path on a repeated batch invite. Every identity here is a
 * brand-new, ad hoc user registered through the UI in its own browser
 * context — no dependency on the seeded fixture user/room
 * (`e2e/support/fixtures.ts`), matching `smoke.spec.ts`'s and
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
  await expect(page).toHaveURL(/\/rooms$/)

  return user
}

/** Opens `/groups` via the avatar menu's "Groups" entry point (Step 46's `(main)/layout.tsx` addition). */
async function openGroupsFromAvatarMenu(page: Page) {
  await page.getByRole("button", { name: "Account menu" }).click()
  await page.getByRole("menuitem", { name: "Groups" }).click()
  await expect(page).toHaveURL(/\/groups$/)
}

/** Opens a room's invite dialog via the top bar's member drawer, from any `(main)` route with that room in the rail. */
async function openInviteDialog(page: Page, roomName: string) {
  await page.getByRole("link", { name: roomName }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)
  await page.getByRole("button", { name: "Room members" }).click()
  await expect(page.getByRole("heading", { name: "Members", exact: true })).toBeVisible()
  await page.getByRole("button", { name: "Invite" }).click()
}

/** Closes the currently-open invite dialog and its parent member drawer. */
async function closeInviteDialog(page: Page) {
  await page.getByRole("button", { name: "Close" }).click()
  await page.keyboard.press("Escape")
}

test.describe("Group management", () => {
  test("group CRUD, membership, and batch-invite-by-group lifecycle, including the repeated-batch-invite skip-reason path", async ({
    page: alicePage,
    browser,
  }) => {
    const roomName = `Groups Spec Room ${Date.now()}`
    const groupName = `Team A ${Date.now()}`
    const renamedGroupName = `Team Alpha ${Date.now()}`

    // (a) Alice registers and creates a room (Alice becomes its master).
    await registerUser(alicePage, "alice")
    await alicePage.getByRole("button", { name: "New Room" }).first().click()
    await alicePage.getByPlaceholder("e.g., Product Strategy").fill(roomName)
    await alicePage.getByRole("button", { name: "Create room" }).click()
    await alicePage.getByRole("heading", { name: roomName }).click()
    await expect(alicePage).toHaveURL(/\/rooms\/[^/]+$/)

    // (b) Bob and Carol register in separate browser contexts (own cookies).
    const bobContext = await browser.newContext()
    const bobPage = await bobContext.newPage()
    const bob = await registerUser(bobPage, "bob")

    const carolContext = await browser.newContext()
    const carolPage = await carolContext.newPage()
    const carol = await registerUser(carolPage, "carol")

    // (c) Alice creates a group and adds Bob and Carol by exact username.
    await openGroupsFromAvatarMenu(alicePage)
    await alicePage.getByRole("button", { name: "New group" }).click()
    await alicePage.getByPlaceholder("e.g., Design Team").fill(groupName)
    await alicePage
      .getByPlaceholder("What is this group for?")
      .fill("A group for the batch-invite e2e spec")
    await alicePage.getByRole("button", { name: "Create group" }).click()

    await alicePage.getByRole("link", { name: new RegExp(groupName) }).click()
    await expect(alicePage.getByRole("heading", { name: groupName })).toBeVisible()

    await alicePage.getByPlaceholder("exact username").fill(bob.username)
    await alicePage.getByRole("button", { name: "Add" }).click()
    await expect(alicePage.getByText(bob.username)).toBeVisible()

    await alicePage.getByPlaceholder("exact username").fill(carol.username)
    await alicePage.getByRole("button", { name: "Add" }).click()
    await expect(alicePage.getByText(carol.username)).toBeVisible()

    // (d) Alice renames the group; the detail page reflects the update.
    await alicePage.getByRole("button", { name: "Edit" }).click()
    await alicePage.getByLabel("Name").fill(renamedGroupName)
    await alicePage.getByRole("button", { name: "Save changes" }).click()
    await expect(
      alicePage.getByRole("heading", { name: renamedGroupName }),
    ).toBeVisible()

    // (e) Alice invites the group into her room at role `member` (the
    // `GroupPicker` trigger's accessible name is "Select a group" until a
    // group is chosen, after which it shows the selected group's name).
    await openInviteDialog(alicePage, roomName)
    await alicePage.getByRole("button", { name: /select a group/i }).click()
    await alicePage.getByRole("menuitem", { name: renamedGroupName }).click()
    await alicePage.getByRole("button", { name: "Invite group" }).click()
    await expect(alicePage.getByText("Invited 2 member(s).")).toBeVisible()
    await closeInviteDialog(alicePage)

    // (f) Bob accepts the pending invitation and is navigated into Alice's room.
    await bobPage.getByRole("button", { name: "Invitations" }).click()
    await bobPage.getByRole("button", { name: "Accept" }).click()
    await expect(bobPage).toHaveURL(/\/rooms\/[^/]+$/)

    // (g) Alice repeats the same batch invite; both members are now skipped
    // — Bob because he's already a room member, Carol because her
    // invitation from step (e) is still pending. `InviteDialog` resets its
    // local state (including the picked group) on close, so the picker
    // trigger reads "Select a group" again rather than remembering the
    // previous pick.
    await openInviteDialog(alicePage, roomName)
    await alicePage.getByRole("button", { name: /select a group/i }).click()
    await alicePage.getByRole("menuitem", { name: renamedGroupName }).click()
    await alicePage.getByRole("button", { name: "Invite group" }).click()
    await expect(alicePage.getByText("Invited 0 member(s).")).toBeVisible()
    await expect(
      alicePage.getByText(`${bob.username}: user is already a member of the room`),
    ).toBeVisible()
    await expect(
      alicePage.getByText(`${carol.username}: invitation already exists`),
    ).toBeVisible()
    await closeInviteDialog(alicePage)

    // (h) Alice removes Carol from the group, then deletes the group entirely.
    await openGroupsFromAvatarMenu(alicePage)
    await alicePage.getByRole("link", { name: new RegExp(renamedGroupName) }).click()
    await expect(
      alicePage.getByRole("heading", { name: renamedGroupName }),
    ).toBeVisible()

    const carolRow = alicePage
      .locator('[data-testid^="group-member-row-"]')
      .filter({ hasText: carol.username })
    await carolRow.getByRole("button", { name: "Remove" }).click()
    await expect(alicePage.getByText(carol.username)).toHaveCount(0)

    await alicePage.getByRole("button", { name: "Delete group" }).click()
    await alicePage
      .getByRole("dialog", { name: "Delete this group?" })
      .getByRole("button", { name: "Delete group" })
      .click()
    await expect(alicePage).toHaveURL(/\/groups$/)
    await expect(alicePage.getByText(renamedGroupName)).toHaveCount(0)

    await bobContext.close()
    await carolContext.close()
  })
})
