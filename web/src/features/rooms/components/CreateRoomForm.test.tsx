import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { createTestQueryClient, render, screen, waitFor } from "@/test/render"
import { CreateRoomForm } from "./CreateRoomForm"

/**
 * Component-level test proving `useCreateRoom`'s mutation invalidates the
 * `["rooms"]` query on success. The actual network call is backed by MSW's
 * `POST /api/proxy/rooms` handler (`../api/handlers.ts`); `invalidateQueries`
 * is spied on directly to assert the cache-invalidation side effect without
 * coupling the test to `RoomList`'s rendering.
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
})
