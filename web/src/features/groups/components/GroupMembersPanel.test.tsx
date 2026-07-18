import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor, within } from "@/test/render"
import { server } from "@/test/msw/server"
import { GroupMembersPanel } from "./GroupMembersPanel"
import type { GroupMember, GroupMemberListResponse } from "../types"

/**
 * Component tests for `GroupMembersPanel`: renders members with resolved
 * usernames from a mocked `GET /api/proxy/groups/:groupId/members`
 * (Step 40's `GroupMemberWithUsername` join means a real username is always
 * present, unlike a room's `Member`), and its add/remove entry points call
 * `POST`/`DELETE /api/proxy/groups/:groupId/members[/:userId]` respectively.
 */
const fixtureGroupMembers: GroupMember[] = [
  {
    id: "group-member-1",
    group_id: "group-1",
    user_id: "user-3",
    username: "bob",
    added_at: "2026-01-02T00:00:00Z",
  },
  {
    id: "group-member-2",
    group_id: "group-1",
    user_id: "user-4",
    username: "carol",
    added_at: "2026-01-03T00:00:00Z",
  },
]

describe("GroupMembersPanel", () => {
  it("renders members with resolved usernames", async () => {
    server.use(
      http.get("/api/proxy/groups/:groupId/members", () => {
        return HttpResponse.json<GroupMemberListResponse>({
          members: fixtureGroupMembers,
        })
      }),
    )

    render(<GroupMembersPanel groupId="group-1" />)

    expect(await screen.findByText("bob")).toBeInTheDocument()
    expect(screen.getByText("carol")).toBeInTheDocument()
  })

  it("shows a retryable error instead of the empty state when the members query fails", async () => {
    server.use(
      http.get("/api/proxy/groups/:groupId/members", () => {
        return new HttpResponse(null, { status: 500 })
      }),
    )

    render(<GroupMembersPanel groupId="group-1" />)

    expect(await screen.findByRole("alert")).toBeInTheDocument()
    expect(screen.queryByText("No members yet.")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument()
  })

  it("shows an empty state when the group has no members", async () => {
    server.use(
      http.get("/api/proxy/groups/:groupId/members", () => {
        return HttpResponse.json<GroupMemberListResponse>({ members: [] })
      }),
    )

    render(<GroupMembersPanel groupId="group-1" />)

    expect(await screen.findByText("No members yet.")).toBeInTheDocument()
  })

  it("calls add-member with the entered username", async () => {
    server.use(
      http.get("/api/proxy/groups/:groupId/members", () => {
        return HttpResponse.json<GroupMemberListResponse>({ members: [] })
      }),
    )
    let capturedBody: unknown
    server.use(
      http.post("/api/proxy/groups/:groupId/members", async ({ request }) => {
        capturedBody = await request.json()
        return HttpResponse.json<GroupMember>(
          {
            id: "group-member-new",
            group_id: "group-1",
            user_id: "user-5",
            username: "dave",
            added_at: "2026-01-07T00:00:00Z",
          },
          { status: 201 },
        )
      }),
    )

    const user = userEvent.setup()
    render(<GroupMembersPanel groupId="group-1" />)

    await user.type(screen.getByPlaceholderText("exact username"), "dave")
    await user.click(screen.getByRole("button", { name: "Add" }))

    await waitFor(() => expect(capturedBody).toEqual({ username: "dave" }))
  })

  it("calls remove-member for the clicked row", async () => {
    server.use(
      http.get("/api/proxy/groups/:groupId/members", () => {
        return HttpResponse.json<GroupMemberListResponse>({
          members: fixtureGroupMembers,
        })
      }),
    )
    let removedUserId: string | undefined
    server.use(
      http.delete("/api/proxy/groups/:groupId/members/:userId", ({ params }) => {
        removedUserId = String(params.userId)
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const user = userEvent.setup()
    render(<GroupMembersPanel groupId="group-1" />)

    await screen.findByText("bob")
    const bobRow = screen.getByTestId("group-member-row-user-3")
    await user.click(within(bobRow).getByRole("button", { name: "Remove" }))

    await waitFor(() => expect(removedUserId).toBe("user-3"))
  })

  it("shows an inline error near Remove when removal fails", async () => {
    server.use(
      http.get("/api/proxy/groups/:groupId/members", () => {
        return HttpResponse.json<GroupMemberListResponse>({
          members: fixtureGroupMembers,
        })
      }),
      http.delete("/api/proxy/groups/:groupId/members/:userId", () => {
        return HttpResponse.json({ message: "Cannot remove the group owner" }, { status: 409 })
      }),
    )

    const user = userEvent.setup()
    render(<GroupMembersPanel groupId="group-1" />)

    await screen.findByText("bob")
    const bobRow = screen.getByTestId("group-member-row-user-3")
    await user.click(within(bobRow).getByRole("button", { name: "Remove" }))

    expect(await within(bobRow).findByRole("alert")).toHaveTextContent(
      "Cannot remove the group owner",
    )
  })
})
