import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { HttpResponse, http } from "msw"
import { server } from "@/test/msw/server"
import { fixtureRooms } from "@/features/rooms/api/handlers"
import { render, screen, waitFor } from "@/test/render"
import { RoomList } from "./RoomList"

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  // `Provider` (via `src/test/render.tsx`) now wraps every test in
  // `EmotionRegistry`, which calls this Next.js hook to flush Emotion's
  // SSR styles; jsdom never streams, so a no-op is all component tests need.
  useServerInsertedHTML: vi.fn(),
}))

/**
 * Component-level tests for `RoomList`'s dedicated error state: when
 * `useRooms()` fails, the room grid must be replaced by an error message with
 * a retry control, rendered before the empty-state branch is ever reached.
 */
describe("RoomList", () => {
  it("renders the room grid on success", async () => {
    render(<RoomList />)

    await waitFor(() =>
      expect(screen.getByText(fixtureRooms[0].name)).toBeInTheDocument(),
    )
  })

  it("renders a dedicated error state with a retry button when the rooms query fails", async () => {
    server.use(
      http.get("/api/proxy/rooms", () => {
        return HttpResponse.json(
          { message: "Internal Server Error" },
          { status: 500 },
        )
      }),
    )

    render(<RoomList />)

    await waitFor(() =>
      expect(screen.getByText(/couldn't load your rooms/i)).toBeInTheDocument(),
    )
    expect(
      screen.getByRole("button", { name: /try again/i }),
    ).toBeInTheDocument()
    // The empty-state copy must not also render alongside the error state.
    expect(screen.queryByText(/no rooms yet/i)).not.toBeInTheDocument()
  })

  it("retries the rooms query when the retry button is clicked", async () => {
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
    render(<RoomList />)

    await waitFor(() =>
      expect(screen.getByText(/couldn't load your rooms/i)).toBeInTheDocument(),
    )
    const requestsBeforeRetry = requestCount

    await user.click(screen.getByRole("button", { name: /try again/i }))

    await waitFor(() => expect(requestCount).toBeGreaterThan(requestsBeforeRetry))
  })
})
