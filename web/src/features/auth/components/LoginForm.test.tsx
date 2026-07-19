import { HttpResponse, http } from "msw"
import { beforeEach, describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { makeLoginFlowUiWithError } from "@/features/auth/api/handlers"
import { LoginForm } from "./LoginForm"

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
 * Component-level tests for `LoginForm`'s Kratos-flow rewrite: the flow
 * loads from the mocked `GET /api/kratos/self-service/login/browser`
 * handler and renders the identifier/password fields, client-side `zod`
 * validation still renders `role="alert"` text without hitting the
 * network, a successful submission (the default MSW handler) redirects,
 * and a `400` flow-validation failure (an MSW override) surfaces through
 * the toaster.
 */
describe("LoginForm", () => {
  it("loads the flow and renders the identifier and password fields", async () => {
    render(<LoginForm />)

    expect(await screen.findByPlaceholderText("you@example.com")).toBeInTheDocument()
    expect(screen.getByPlaceholderText("Enter your password")).toBeInTheDocument()
  })

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

  it("redirects to /rooms on a successful submission", async () => {
    const user = userEvent.setup()
    render(<LoginForm />)

    await user.type(screen.getByPlaceholderText("you@example.com"), "user@example.com")
    await user.type(screen.getByPlaceholderText("Enter your password"), "password123")
    await user.click(screen.getByRole("button", { name: "Sign in" }))

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/rooms"))
  })

  it("re-renders the flow's field/flow-level errors and shows a toast on a 400 response", async () => {
    server.use(
      http.post("/api/kratos/self-service/login", () => {
        return HttpResponse.json({ ui: makeLoginFlowUiWithError() }, { status: 400 })
      }),
    )

    const user = userEvent.setup()
    render(<LoginForm />)

    await user.type(screen.getByPlaceholderText("you@example.com"), "user@example.com")
    await user.type(screen.getByPlaceholderText("Enter your password"), "wrong-password")
    await user.click(screen.getByRole("button", { name: "Sign in" }))

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("Sign in failed"),
    )
    expect(pushMock).not.toHaveBeenCalled()
  })
})
