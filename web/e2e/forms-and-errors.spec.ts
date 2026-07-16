import { expect, test } from "@playwright/test"
import { FIXTURE_USER } from "./support/fixtures"

/**
 * End-to-end coverage for Step 17's form/toaster/App-Router-error-surface
 * work, driven through the real UI against the seeded `test` profile stack
 * (see `docs/tasks/step10.md`/`playwright.config.ts`).
 *
 * Complements the component-level Vitest suites (colocated `*.test.tsx`
 * files, e.g. `LoginForm.test.tsx`), which cover the same assertions in
 * isolation with a mocked network layer — this spec proves the same
 * behaviors hold end to end through the real Next.js server and Go API.
 */
test.describe("form validation, toasts, and the room not-found page", () => {
  test("submitting the login form with an empty email shows a role=alert error and makes no request", async ({
    page,
  }) => {
    await page.goto("/login")

    const requests: string[] = []
    page.on("request", (req) => {
      if (req.url().includes("/api/auth/login")) requests.push(req.url())
    })

    await page.getByRole("button", { name: "Sign in" }).click()

    // Scope to the specific field alert: Next.js's own
    // `__next-route-announcer__` also renders `role="alert"`, and the login
    // form's submit-time validation surfaces both the email and password
    // field errors simultaneously, so a bare `getByRole("alert")` hits a
    // Playwright strict-mode violation (multiple matches).
    await expect(
      page.getByRole("alert").filter({ hasText: "Email is required" }),
    ).toBeVisible()
    expect(requests).toHaveLength(0)
  })

  test("register form shows a live mismatch error and a password strength meter", async ({
    page,
  }) => {
    await page.goto("/register")

    await page.getByPlaceholder("Create a password").fill("correcthorse1")
    await expect(page.getByText(/Low|Medium|High/)).toBeVisible()

    await page.getByPlaceholder("Confirm your password").fill("mismatch1")

    const mismatchAlert = page
      .getByRole("alert")
      .filter({ hasText: "Passwords do not match" })
    await expect(mismatchAlert).toBeVisible()

    await page.getByPlaceholder("Confirm your password").fill("correcthorse1")
    await expect(mismatchAlert).toHaveCount(0)
  })

  test("registering with an email that is already in use shows an error toast", async ({
    page,
  }) => {
    const runId = `${Date.now()}_${Math.floor(Math.random() * 100_000)}`

    await page.goto("/register")
    // The seeded fixture user's email is guaranteed to already exist.
    await page.getByPlaceholder("you@example.com").fill(FIXTURE_USER.email)
    // Underscores only: registerSchema's username regex
    // (`/^[a-zA-Z0-9_]+$/`) rejects hyphens, which would otherwise fail
    // client-side validation before any request (or toast) ever fires.
    await page.getByPlaceholder("johndoe").fill(`dup_${runId}`)
    await page.getByPlaceholder("Create a password").fill("correcthorse1")
    await page.getByPlaceholder("Confirm your password").fill("correcthorse1")
    await page.getByRole("button", { name: "Create account" }).click()

    // Assert via the toaster region (role="region", not the bare "status"
    // role the individual toast carries) so this doesn't collide with any
    // other status-role element on the page.
    await expect(
      page.getByRole("region", { name: /notifications/i }).getByText("Registration failed"),
    ).toBeVisible()
  })

  test("creating a room with an empty name shows a role=alert error", async ({
    page,
  }) => {
    await page.goto("/login")
    await page.getByPlaceholder("you@example.com").fill(FIXTURE_USER.email)
    await page.getByPlaceholder("Enter your password").fill(FIXTURE_USER.password)
    await page.getByRole("button", { name: "Sign in" }).click()
    await expect(page).toHaveURL(/\/rooms$/)

    await page.getByRole("button", { name: "New Room" }).first().click()
    await page.getByRole("button", { name: "Create room" }).click()

    await expect(
      page.getByRole("alert").filter({ hasText: "Room name is required" }),
    ).toBeVisible()
  })

  test("navigating to a non-existent room ID renders the not-found page", async ({
    page,
  }) => {
    await page.goto("/login")
    await page.getByPlaceholder("you@example.com").fill(FIXTURE_USER.email)
    await page.getByPlaceholder("Enter your password").fill(FIXTURE_USER.password)
    await page.getByRole("button", { name: "Sign in" }).click()
    await expect(page).toHaveURL(/\/rooms$/)

    await page.goto("/rooms/00000000-0000-0000-0000-000000000000")

    await expect(
      page.getByRole("heading", { name: "Room not found" }),
    ).toBeVisible()
    await expect(
      page.getByRole("link", { name: "Back to rooms" }),
    ).toHaveAttribute("href", "/rooms")
  })
})
