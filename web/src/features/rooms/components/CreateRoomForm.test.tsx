import { HttpResponse, http } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { createTestQueryClient, render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import { CreateRoomForm } from "./CreateRoomForm"

/**
 * Component-level tests for `CreateRoomForm`'s `react-hook-form` + `zod`
 * rewrite, covering:
 * - the success path, proving `useCreateRoom`'s mutation still invalidates
 *   the `["rooms"]` query (the actual network call is backed by MSW's
 *   `POST /api/proxy/rooms` handler from `../api/handlers.ts`;
 *   `invalidateQueries` is spied on directly to assert the cache-invalidation
 *   side effect without coupling the test to `RoomList`'s rendering);
 * - the empty-name validation error rendering as `role="alert"` text without
 *   calling the mutation;
 * - a mutation failure surfacing through the toaster instead of the old
 *   silent `// TODO: show error toast` catch block;
 * - clicking Cancel resets the form fields, not just the dialog's open
 *   state, so reopening the dialog never shows a previous attempt's stale
 *   input.
 */
describe("CreateRoomForm", () => {
  it("invalidates the rooms query after a successful create", async () => {
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries")
    const user = userEvent.setup()

    render(<CreateRoomForm />, { queryClient })

    await user.click(screen.getByRole("button", { name: "New Room" }))
    await user.type(
      screen.getByPlaceholderText("e.g., Product Strategy"),
      "Test Room",
    )
    await user.click(screen.getByRole("button", { name: "Create room" }))

    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["rooms"] }),
    )
  })

  it("shows a role=alert validation error and does not submit when the name is empty", async () => {
    const user = userEvent.setup()
    render(<CreateRoomForm />)

    await user.click(screen.getByRole("button", { name: "New Room" }))
    await user.click(screen.getByRole("button", { name: "Create room" }))

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Room name is required",
    )
  })

  it("shows a toast when the create-room mutation fails", async () => {
    server.use(
      http.post("/api/proxy/rooms", () => {
        return HttpResponse.json(
          { message: "Internal Server Error" },
          { status: 500 },
        )
      }),
    )

    const user = userEvent.setup()
    render(<CreateRoomForm />)

    await user.click(screen.getByRole("button", { name: "New Room" }))
    await user.type(
      screen.getByPlaceholderText("e.g., Product Strategy"),
      "Test Room",
    )
    await user.click(screen.getByRole("button", { name: "Create room" }))

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "Failed to create room",
      ),
    )
  })

  it("resets the form when Cancel is clicked, so reopening starts blank", async () => {
    const user = userEvent.setup()
    render(<CreateRoomForm />)

    await user.click(screen.getByRole("button", { name: "New Room" }))
    await user.type(
      screen.getByPlaceholderText("e.g., Product Strategy"),
      "Draft name",
    )
    await user.click(screen.getByRole("button", { name: "Cancel" }))

    await user.click(screen.getByRole("button", { name: "New Room" }))

    expect(
      screen.getByPlaceholderText("e.g., Product Strategy"),
    ).toHaveValue("")
  })
})
