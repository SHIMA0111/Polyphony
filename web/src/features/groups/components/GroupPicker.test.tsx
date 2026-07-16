import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { GroupPicker } from "./GroupPicker"
import type { Group, GroupListResponse } from "../types"

const fixtureGroups: Group[] = [
  {
    id: "group-1",
    owner_id: "user-1",
    name: "Team A",
    description: "A test group",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
]

/**
 * Component tests for `GroupPicker`: renders the caller's groups from a
 * mocked `GET /api/proxy/groups`, renders a `/groups` link empty state when
 * there are none instead of an unusable empty menu, and calls `onSelect`
 * with the chosen group.
 */
describe("GroupPicker", () => {
  it("renders groups from GET /api/proxy/groups and selecting one calls onSelect", async () => {
    server.use(
      http.get("/api/proxy/groups", () => {
        return HttpResponse.json<GroupListResponse>({ groups: fixtureGroups })
      }),
    )

    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(<GroupPicker onSelect={onSelect} />)

    await user.click(await screen.findByRole("button", { name: /select a group/i }))
    // `waitFor` (rather than a bare `await user.click(await screen.findByRole(...))`)
    // retries the click itself: Chakra's `Menu.Content` briefly carries
    // `pointer-events: none` during its open transition, and userEvent's
    // click throws (rather than retrying) if it lands in that window.
    await waitFor(() =>
      user.click(screen.getByRole("menuitem", { name: "Team A" })),
    )

    expect(onSelect).toHaveBeenCalledWith(fixtureGroups[0])
  })

  it("renders an empty state linking to /groups when there are no groups", async () => {
    server.use(
      http.get("/api/proxy/groups", () => {
        return HttpResponse.json<GroupListResponse>({ groups: [] })
      }),
    )

    render(<GroupPicker onSelect={vi.fn()} />)

    await waitFor(() =>
      expect(screen.getByText("create one")).toBeInTheDocument(),
    )
    expect(screen.getByRole("link", { name: "create one" })).toHaveAttribute(
      "href",
      "/groups",
    )
  })
})
