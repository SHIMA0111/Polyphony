import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { HttpResponse, http } from "msw"
import { server } from "@/test/msw/server"
import { render, screen, waitFor } from "@/test/render"
import { fixtureRooms } from "@/features/rooms/api/handlers"
import { RoomRail } from "./RoomRail"

const { useParamsMock } = vi.hoisted(() => ({ useParamsMock: vi.fn() }))

vi.mock("next/navigation", () => ({
  useParams: useParamsMock,
  // `Provider` (via `src/test/render.tsx`) now wraps every test in
  // `EmotionRegistry`, which calls this Next.js hook to flush Emotion's
  // SSR styles; jsdom never streams, so a no-op is all component tests need.
  useServerInsertedHTML: vi.fn(),
}))

/**
 * Component test for `RoomRail`, the always-mounted room list rendered by
 * `web/src/app/(main)/layout.tsx`. `useRooms()` is exercised against the
 * real MSW `GET /api/proxy/rooms` handler (`../api/handlers.ts`) rather than
 * mocked, matching this repo's existing data-fetching-hook test convention
 * (see `use-rooms.test.tsx`); only `next/navigation`'s `useParams` is
 * mocked, since `RoomRail` derives the active room from the `roomId` route
 * param.
 */
describe("RoomRail", () => {
  it("highlights the room matching the roomId route param and leaves others unstyled", async () => {
    useParamsMock.mockReturnValue({ roomId: fixtureRooms[1].id })

    render(<RoomRail />)

    await waitFor(() =>
      expect(screen.getByText(fixtureRooms[0].name)).toBeInTheDocument(),
    )

    // The active room's row is marked `aria-current="page"` (and rendered
    // with a distinct highlighted background/left accent bar); the inactive
    // row carries neither.
    expect(
      screen.getByRole("link", { name: fixtureRooms[1].name, current: "page" }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole("link", { name: fixtureRooms[0].name }),
    ).not.toHaveAttribute("aria-current")
  })

  it("renders no active highlight when there is no roomId route param", async () => {
    useParamsMock.mockReturnValue({})

    render(<RoomRail />)

    await waitFor(() =>
      expect(screen.getByText(fixtureRooms[0].name)).toBeInTheDocument(),
    )

    for (const room of fixtureRooms) {
      expect(
        screen.getByRole("link", { name: room.name }),
      ).not.toHaveAttribute("aria-current")
    }
  })

  it("renders a compact error state with a retry action when the rooms query fails", async () => {
    useParamsMock.mockReturnValue({})
    let requestCount = 0
    server.use(
      http.get("/api/proxy/rooms", () => {
        requestCount += 1
        return HttpResponse.json(
          { message: "Internal Server Error" },
          { status: 500 },
        )
      }),
    )

    const user = userEvent.setup()
    render(<RoomRail />)

    await waitFor(() =>
      expect(screen.getByText(/couldn't load rooms/i)).toBeInTheDocument(),
    )
    const retryButton = screen.getByRole("button", { name: /retry/i })
    expect(retryButton).toBeInTheDocument()
    const requestsBeforeRetry = requestCount

    await user.click(retryButton)

    await waitFor(() => expect(requestCount).toBeGreaterThan(requestsBeforeRetry))
  })
})
