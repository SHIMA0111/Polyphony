import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { GroupFormDialog } from "./GroupFormDialog"
import type { Group } from "../types"

/**
 * Component tests for `GroupFormDialog`'s two modes: create mode calls
 * `POST /api/proxy/groups` (via `useCreateGroup`) with the entered
 * name/description, and edit mode pre-fills the fields from `initialGroup`
 * and calls `PUT /api/proxy/groups/:groupId` (via `useUpdateGroup`) with the
 * updated values.
 */
const fixtureGroup: Group = {
  id: "group-1",
  owner_id: "user-1",
  name: "Team A",
  description: "A test group",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

describe("GroupFormDialog", () => {
  it("create mode calls POST /api/proxy/groups with the entered name/description", async () => {
    let capturedBody: unknown
    server.use(
      http.post("/api/proxy/groups", async ({ request }) => {
        capturedBody = await request.json()
        return HttpResponse.json<Group>(fixtureGroup, { status: 201 })
      }),
    )

    const user = userEvent.setup()
    render(<GroupFormDialog mode="create" />)

    await user.click(screen.getByRole("button", { name: "New group" }))
    await user.type(screen.getByPlaceholderText("e.g., Design Team"), "Team B")
    await user.type(
      screen.getByPlaceholderText("What is this group for?"),
      "Second group",
    )
    await user.click(screen.getByRole("button", { name: "Create group" }))

    await waitFor(() =>
      expect(capturedBody).toEqual({ name: "Team B", description: "Second group" }),
    )
  })

  it("edit mode pre-fills existing values and calls PUT /api/proxy/groups/:groupId", async () => {
    let capturedBody: unknown
    server.use(
      http.put("/api/proxy/groups/:groupId", async ({ request }) => {
        capturedBody = await request.json()
        return HttpResponse.json<Group>(fixtureGroup)
      }),
    )

    const user = userEvent.setup()
    render(<GroupFormDialog mode="edit" initialGroup={fixtureGroup} />)

    await user.click(screen.getByRole("button", { name: "Edit" }))

    expect(screen.getByDisplayValue("Team A")).toBeInTheDocument()
    expect(screen.getByDisplayValue("A test group")).toBeInTheDocument()

    const nameInput = screen.getByDisplayValue("Team A")
    await user.clear(nameInput)
    await user.type(nameInput, "Team A Renamed")

    await user.click(screen.getByRole("button", { name: "Save changes" }))

    await waitFor(() =>
      expect(capturedBody).toEqual({
        name: "Team A Renamed",
        description: "A test group",
      }),
    )
  })
})
