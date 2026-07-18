import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { fixtureGroups } from "../api/handlers"
import { GroupList } from "./GroupList"

/**
 * Component-level tests for `GroupList`, covering:
 * - groups rendering from a mocked `GET /api/proxy/groups` (the fixture
 *   from `../api/handlers.ts`);
 * - the empty state when there are no groups;
 * - the error state when the query fails, checked before the empty-state
 *   branch — matching `RoomList`'s own isPending -> isError -> empty ->
 *   list ordering — including that "Try again" refetches and can recover
 *   into the successful list.
 */
describe("GroupList", () => {
  it("renders group cards from GET /groups", async () => {
    render(<GroupList />)

    await waitFor(() =>
      expect(screen.getByText(fixtureGroups[0].name)).toBeInTheDocument(),
    )
  })

  it("renders the empty state when there are no groups", async () => {
    server.use(
      http.get("/api/proxy/groups", () => {
        return HttpResponse.json({ groups: [] })
      }),
    )

    render(<GroupList />)

    await waitFor(() => expect(screen.getByText("No groups yet")).toBeInTheDocument())
  })

  it("renders an error state with a Try again button when the groups query fails, and recovers on refetch", async () => {
    let requestCount = 0
    server.use(
      http.get("/api/proxy/groups", () => {
        requestCount += 1
        if (requestCount === 1) {
          return HttpResponse.json({ message: "Internal Server Error" }, { status: 500 })
        }
        return HttpResponse.json({ groups: fixtureGroups })
      }),
    )

    const user = userEvent.setup()
    render(<GroupList />)

    await waitFor(() =>
      expect(screen.getByText("Failed to load groups")).toBeInTheDocument(),
    )
    expect(screen.queryByText("No groups yet")).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Try again" }))

    await waitFor(() =>
      expect(screen.getByText(fixtureGroups[0].name)).toBeInTheDocument(),
    )
    expect(screen.queryByText("Failed to load groups")).not.toBeInTheDocument()
  })
})
