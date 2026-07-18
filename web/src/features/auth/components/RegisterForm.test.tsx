import { HttpResponse, http } from "msw"
import { beforeEach, describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { makeRegistrationFlowUiWithError } from "@/features/auth/api/handlers"
import { RegisterForm } from "./RegisterForm"

const pushMock = vi.fn()
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
  // `Provider` (via `src/test/render.tsx`) now wraps every test in
  // `EmotionRegistry`, which calls this Next.js hook to flush Emotion's
  // SSR styles; jsdom never streams, so a no-op is all component tests need.
  useServerInsertedHTML: vi.fn(),
}))

beforeEach(() => {
  pushMock.mockClear()
})

/**
 * Component-level tests for `RegisterForm`'s Kratos-flow rewrite: the flow
 * loads from the mocked `GET /api/kratos/self-service/registration/browser`
 * handler and renders the email/username/password fields, live
 * confirm-password validation (`mode: "onChange"`) and the password
 * strength meter stay pure client-side `zod`/RHF concerns, a successful
 * submission (the default MSW handler) redirects, and a `400`
 * flow-validation failure (an MSW override, e.g. "email already in use")
 * surfaces through the toaster.
 */
describe("RegisterForm", () => {
  it("loads the flow and renders the email, username, and password fields", async () => {
    render(<RegisterForm />)

    expect(await screen.findByPlaceholderText("you@example.com")).toBeInTheDocument()
    expect(screen.getByPlaceholderText("johndoe")).toBeInTheDocument()
    expect(screen.getByPlaceholderText("Create a password")).toBeInTheDocument()
  })

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

  it("redirects to /rooms on a successful submission", async () => {
    const user = userEvent.setup()
    render(<RegisterForm />)

    await user.type(screen.getByPlaceholderText("you@example.com"), "user@example.com")
    await user.type(screen.getByPlaceholderText("johndoe"), "testuser")
    await user.type(screen.getByPlaceholderText("Create a password"), "correcthorse1")
    await user.type(screen.getByPlaceholderText("Confirm your password"), "correcthorse1")
    await user.click(screen.getByRole("button", { name: "Create account" }))

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/rooms"))
  })

  it("shows a toast when the registration flow returns a 400 validation error", async () => {
    server.use(
      http.post("/api/kratos/self-service/registration", () => {
        return HttpResponse.json(
          { ui: makeRegistrationFlowUiWithError() },
          { status: 400 },
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
    expect(pushMock).not.toHaveBeenCalled()
  })
})
