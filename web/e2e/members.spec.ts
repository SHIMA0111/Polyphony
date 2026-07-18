import { expect, test, type Page } from "@playwright/test"

/**
 * End-to-end coverage for Step 37's member/invitation/role UI: the full
 * invite -> accept -> role-change -> transfer-ownership lifecycle, plus the
 * reader/guest gating checks (`docs/tasks/step37.md`'s Verification item
 * 5). Every identity here is a brand-new, ad hoc user registered through
 * the UI in its own browser context — no dependency on the seeded fixture
 * user/room (`e2e/support/fixtures.ts`), matching `smoke.spec.ts`'s and
 * `auth.spec.ts`'s convention so this spec has no ordering dependency on
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
    // rejects hyphens (see `smoke.spec.ts`/`auth.spec.ts`'s same convention).
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

/**
 * As the currently-open room's viewer, invites `invitee` by exact username
 * with `role`, via the member avatar stack -> member panel -> invite dialog,
 * and asserts the dialog reports success.
 */
async function inviteByUsername(page: Page, invitee: string, role: string) {
  await page.getByRole("button", { name: "Room members" }).click()
  // `exact: true`: the room under test is named "Members Spec Room <ts>",
  // whose own page heading would otherwise also fuzzy-match "Members" and
  // trip strict mode whenever the drawer's aria-inert treatment of the
  // background hasn't kicked in yet.
  await expect(page.getByRole("heading", { name: "Members", exact: true })).toBeVisible()

  await page.getByRole("button", { name: "Invite" }).click()
  await page.getByPlaceholder("exact username").fill(invitee)
  if (role !== "member") {
    await page.locator("select").first().selectOption(role)
  }
  await page.getByRole("button", { name: "Send invitation" }).click()
  await expect(page.getByText("Invitation sent.")).toBeVisible()

  await page.getByRole("button", { name: "Close" }).click()
  // Close the member drawer too, so it doesn't linger open across steps.
  await page.keyboard.press("Escape")
}

/** As the invitee, opens the invitations bell, accepts the pending invitation from `inviterRoomHeading`, and waits for the room to load. */
async function acceptFromInbox(page: Page) {
  await page.getByRole("button", { name: "Invitations" }).click()
  await page.getByRole("button", { name: "Accept" }).click()
  await expect(page).toHaveURL(/\/rooms\/[^/]+$/)
}

test.describe("Members, invitations, and roles", () => {
  test("invite -> accept -> role-change -> transfer-ownership lifecycle, plus reader/guest gating", async ({
    page: alicePage,
    browser,
  }) => {
    const roomName = `Members Spec Room ${Date.now()}`

    // (a) Alice registers and creates a room.
    await registerUser(alicePage, "alice")
    await alicePage.getByRole("button", { name: "New Room" }).first().click()
    await alicePage.getByPlaceholder("e.g., Product Strategy").fill(roomName)
    await alicePage.getByRole("button", { name: "Create room" }).click()
    await alicePage.getByRole("heading", { name: roomName }).click()
    await expect(alicePage).toHaveURL(/\/rooms\/[^/]+$/)

    // (b) Bob registers in a separate browser context (own cookies).
    const bobContext = await browser.newContext()
    const bobPage = await bobContext.newPage()
    const bob = await registerUser(bobPage, "bob")

    // (c) Alice invites Bob by exact username with role `member`.
    await inviteByUsername(alicePage, bob.username, "member")

    // (d) Bob accepts from the invitations inbox and can see the message input.
    await acceptFromInbox(bobPage)
    await expect(bobPage.getByPlaceholder("Ask me anything...")).toBeVisible()

    // (e) Alice changes Bob's role to `admin` via the role picker.
    await alicePage.getByRole("button", { name: "Room members" }).click()
    // `exact: true` — see inviteByUsername's comment on the same locator.
    await expect(
      alicePage.getByRole("heading", { name: "Members", exact: true }),
    ).toBeVisible()
    const bobRow = alicePage
      .locator('[data-testid^="member-row-"]')
      .filter({ hasText: bob.username })
    await bobRow.getByRole("button", { name: /member/i }).click()
    await alicePage.getByRole("menuitem", { name: "Admin" }).click()
    await expect(bobRow.getByText("Admin")).toBeVisible()

    // (f) Alice transfers ownership to Bob; Alice becomes admin.
    await alicePage.getByRole("button", { name: "Transfer ownership" }).click()
    const transferDialog = alicePage.getByRole("dialog", { name: "Transfer ownership" })
    await transferDialog.getByText(bob.username).click()
    await transferDialog.getByRole("button", { name: "Confirm transfer" }).click()
    await expect(
      alicePage.getByRole("button", { name: "Transfer ownership" }),
    ).toHaveCount(0)

    // Reset the member drawer left open by steps (e)/(f) — inviteByUsername
    // below starts from a closed drawer and re-opens it itself. A reload is
    // used deliberately: after the nested TransferOwnershipDialog confirms,
    // both Escape and a click on the drawer's own close trigger fail to
    // dispatch (the click hangs in hit-testing — an overlay/pointer-events
    // remnant of the nested dialog; flagged in the wave-5 review findings),
    // so closing the drawer via UI is not reliably possible here.
    await alicePage.reload()
    await expect(alicePage.getByRole("dialog", { name: "Members" })).toHaveCount(0)

    // (g) Alice (now admin) invites Carol with role `reader`; Carol sees no message input.
    const carolContext = await browser.newContext()
    const carolPage = await carolContext.newPage()
    const carol = await registerUser(carolPage, "carol")
    await inviteByUsername(alicePage, carol.username, "reader")
    await acceptFromInbox(carolPage)
    await expect(carolPage.getByPlaceholder("Ask me anything...")).toHaveCount(0)

    // (h) Alice invites Dave with role `guest`; Dave sees "Send" but not "Send with AI".
    const daveContext = await browser.newContext()
    const davePage = await daveContext.newPage()
    const dave = await registerUser(davePage, "dave")
    await inviteByUsername(alicePage, dave.username, "guest")
    await acceptFromInbox(davePage)
    await expect(davePage.getByRole("button", { name: "Send", exact: true })).toBeVisible()
    await expect(davePage.getByRole("button", { name: "Send with AI" })).toHaveCount(0)

    await bobContext.close()
    await carolContext.close()
    await daveContext.close()
  })
})
