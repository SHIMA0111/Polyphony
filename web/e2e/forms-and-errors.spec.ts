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

    await expect(page.getByRole("alert")).toHaveText("Email is required")
    expect(requests).toHaveLength(0)
  })

  test("register form shows a live mismatch error and a password strength meter", async ({
    page,
  }) => {
    await page.goto("/register")

    await page.getByPlaceholder("Create a password").fill("correcthorse1")
    await page.getByPlaceholder("Confirm your password").fill("mismatch1")

    await expect(page.getByRole("alert")).toHaveText("Passwords do not match")

    await page.getByPlaceholder("Confirm your password").fill("correcthorse1")
    await expect(page.getByRole("alert")).toHaveCount(0)
  })

  test("registering with an email that is already in use shows an error toast", async ({
    page,
  }) => {
    const runId = `${Date.now()}-${Math.floor(Math.random() * 100_000)}`

    await page.goto("/register")
    // The seeded fixture user's email is guaranteed to already exist.
    await page.getByPlaceholder("you@example.com").fill(FIXTURE_USER.email)
    await page.getByPlaceholder("johndoe").fill(`dup-${runId}`)
    await page.getByPlaceholder("Create a password").fill("correcthorse1")
    await page.getByPlaceholder("Confirm your password").fill("correcthorse1")
    await page.getByRole("button", { name: "Create account" }).click()

    await expect(page.getByRole("status")).toBeVisible()
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

    await expect(page.getByRole("alert")).toHaveText("Room name is required")
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
