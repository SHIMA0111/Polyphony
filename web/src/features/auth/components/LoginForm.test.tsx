import { HttpResponse, http } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { LoginForm } from "./LoginForm"

const pushMock = vi.fn()
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}))

/**
 * Component-level tests for `LoginForm`'s `react-hook-form` + `zod`
 * rewrite: validation errors render as `role="alert"` text without hitting
 * the network, and a mutation failure (simulated via an MSW override)
 * surfaces through the toaster rather than an inline banner.
 */
describe("LoginForm", () => {
  it("shows a role=alert validation error and does not submit when the email is empty", async () => {
    const user = userEvent.setup()
    render(<LoginForm />)

    // Fill in a valid password so only the email field fails validation,
    // keeping the assertion below unambiguous (both fields are empty by
    // default, which would otherwise render two role="alert" nodes).
    await user.type(screen.getByPlaceholderText("Enter your password"), "password123")
    await user.click(screen.getByRole("button", { name: "Sign in" }))

    const alerts = await screen.findAllByRole("alert")
    expect(alerts.map((el) => el.textContent)).toContain("Email is required")
  })

  it("shows a toast when the login mutation fails", async () => {
    // A non-401 failure status is used deliberately: `apiFetch`'s shared
    // 401 handling (`@/lib/http-client.ts`) clears the session cookie and
    // redirects to `/login` via `window.location.assign`, which jsdom
    // doesn't implement and would make this test about the redirect, not
    // the toaster.
    server.use(
      http.post("/api/auth/login", () => {
        return HttpResponse.json(
          { message: "Invalid email or password" },
          { status: 400 },
        )
      }),
    )

    const user = userEvent.setup()
    render(<LoginForm />)

    await user.type(screen.getByPlaceholderText("you@example.com"), "user@example.com")
    await user.type(screen.getByPlaceholderText("Enter your password"), "password123")
    await user.click(screen.getByRole("button", { name: "Sign in" }))

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("Sign in failed"),
    )
  })
})
