import { describe, expect, it, vi } from "vitest"
import { render, screen, waitFor } from "@/test/render"
import { fixtureRooms } from "@/features/rooms/api/handlers"
import MainLayout from "./layout"

const { useParamsMock, useRouterMock } = vi.hoisted(() => ({
  useParamsMock: vi.fn(),
  useRouterMock: vi.fn(() => ({ push: vi.fn() })),
}))

vi.mock("next/navigation", () => ({
  useParams: useParamsMock,
  useRouter: useRouterMock,
}))

/**
 * Covers Step 16's Scope item requiring a test for "rail and content-pane
 * `display` values differ correctly at `base` vs when `roomId` is present":
 * asserts the two panes carry opposite `base` visibility, driven solely by
 * `useParams()`'s `roomId`, matching the single-component-tree responsive
 * collapse (no separate mobile/desktop branches).
 */
describe("(main)/layout", () => {
  it("shows the rail and hides the content pane at `base` when there is no active room", async () => {
    useParamsMock.mockReturnValue({})

    render(
      <MainLayout>
        <div data-testid="page-content">rooms grid</div>
      </MainLayout>,
    )

    await waitFor(() =>
      expect(screen.getByText(fixtureRooms[0].name)).toBeInTheDocument(),
    )

    const rail = screen.getByRole("navigation", { name: "Rooms" })
    const contentPane = screen.getByTestId("page-content").parentElement

    expect(rail).toHaveStyle({ display: "flex" })
    expect(contentPane).toHaveStyle({ display: "none" })
  })

  it("hides the rail and shows the content pane at `base` when a room is active", async () => {
    useParamsMock.mockReturnValue({ roomId: fixtureRooms[0].id })

    render(
      <MainLayout>
        <div data-testid="page-content">chat thread</div>
      </MainLayout>,
    )

    await waitFor(() =>
      expect(screen.getByText(fixtureRooms[0].name)).toBeInTheDocument(),
    )

    // `hidden: true` includes elements a real browser would drop from the
    // accessibility tree once `display: none` applies — needed here since
    // this assertion is about the *style*, not accessibility-tree presence.
    // Name matching is dropped for this query: the accessible-name
    // computation this matcher uses does not resolve `aria-label` once the
    // element is `display: none`, even though the attribute is still set.
    const rail = screen.getByRole("navigation", { hidden: true })
    const contentPane = screen.getByTestId("page-content").parentElement

    expect(rail).toHaveAttribute("aria-label", "Rooms")
    expect(rail).toHaveStyle({ display: "none" })
    expect(contentPane).toHaveStyle({ display: "flex" })
  })

  it("renders a single persistent top bar, regardless of the active room", () => {
    useParamsMock.mockReturnValue({})

    render(
      <MainLayout>
        <div>content</div>
      </MainLayout>,
    )

    // Exactly one "Polyphony" heading — the top bar the layout owns, not a
    // second copy re-rendered by the page underneath it.
    expect(screen.getAllByText("Polyphony")).toHaveLength(1)
  })
})
