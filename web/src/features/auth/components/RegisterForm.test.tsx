import { HttpResponse, http } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { RegisterForm } from "./RegisterForm"

const pushMock = vi.fn()
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}))

/**
 * Component-level tests for `RegisterForm`'s `react-hook-form` + `zod`
 * rewrite: live confirm-password validation (`mode: "onChange"`), the
 * password strength meter, and toaster-surfaced mutation failures.
 */
describe("RegisterForm", () => {
  it("shows a live role=alert mismatch error under Confirm Password and clears it once the fields match", async () => {
    const user = userEvent.setup()
    render(<RegisterForm />)

    await user.type(screen.getByPlaceholderText("Create a password"), "correcthorse1")
    await user.type(screen.getByPlaceholderText("Confirm your password"), "mismatch1")

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Passwords do not match",
    )

    await user.clear(screen.getByPlaceholderText("Confirm your password"))
    await user.type(screen.getByPlaceholderText("Confirm your password"), "correcthorse1")

    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull())
  })

  it("renders the password strength meter once the password field is non-empty", async () => {
    const user = userEvent.setup()
    render(<RegisterForm />)

    expect(screen.queryByText(/Low|Medium|High/)).toBeNull()

    await user.type(screen.getByPlaceholderText("Create a password"), "abc")

    expect(await screen.findByText(/Low|Medium|High/)).toBeInTheDocument()
  })

  it("shows a toast when the register mutation fails", async () => {
    server.use(
      http.post("/api/auth/register", () => {
        return HttpResponse.json(
          { message: "Email already in use" },
          { status: 409 },
        )
      }),
    )

    const user = userEvent.setup()
    render(<RegisterForm />)

    await user.type(screen.getByPlaceholderText("you@example.com"), "user@example.com")
    await user.type(screen.getByPlaceholderText("johndoe"), "testuser")
    await user.type(screen.getByPlaceholderText("Create a password"), "correcthorse1")
    await user.type(screen.getByPlaceholderText("Confirm your password"), "correcthorse1")
    await user.click(screen.getByRole("button", { name: "Create account" }))

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("Registration failed"),
    )
  })
})
