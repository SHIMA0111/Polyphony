import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { fixtureRooms } from "../api/handlers"
import { RoomList } from "./RoomList"

/**
 * Component-level tests for `RoomList`, covering:
 * - rooms rendering from a mocked `GET /api/proxy/rooms` (the fixture from
 *   `../api/handlers.ts`);
 * - the empty state when there are no rooms;
 * - the error state when the query fails, including that "Try again"
 *   refetches and can recover into the successful list.
 */
describe("RoomList", () => {
  it("renders room cards from GET /rooms", async () => {
    render(<RoomList />)

    await waitFor(() =>
      expect(screen.getByText(fixtureRooms[0].name)).toBeInTheDocument(),
    )
    expect(screen.getByText(fixtureRooms[1].name)).toBeInTheDocument()
  })

  it("renders the empty state when there are no rooms", async () => {
    server.use(
      http.get("/api/proxy/rooms", () => {
        return HttpResponse.json([])
      }),
    )

    render(<RoomList />)

    await waitFor(() => expect(screen.getByText("No rooms yet")).toBeInTheDocument())
  })

  it("renders an error state with a Try again button when the rooms query fails, and recovers on refetch", async () => {
    let requestCount = 0
    server.use(
      http.get("/api/proxy/rooms", () => {
        requestCount += 1
        if (requestCount === 1) {
          return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
        }
        return HttpResponse.json(fixtureRooms)
      }),
    )

    const user = userEvent.setup()
    render(<RoomList />)

    await waitFor(() =>
      expect(screen.getByText("Failed to load rooms")).toBeInTheDocument(),
    )

    await user.click(screen.getByRole("button", { name: "Try again" }))

    await waitFor(() =>
      expect(screen.getByText(fixtureRooms[0].name)).toBeInTheDocument(),
    )
    expect(screen.queryByText("Failed to load rooms")).not.toBeInTheDocument()
  })
})
