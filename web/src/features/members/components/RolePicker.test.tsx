import { http, HttpResponse } from "msw"
import { describe, expect, it } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen } from "@/test/render"
import { server } from "@/test/msw/server"
import { RolePicker } from "./RolePicker"

/**
 * `RolePicker` covers Step 37's Scope requirement: the `master` option must
 * never be offered (granting ownership through this endpoint is rejected
 * server-side; use the dedicated transfer-ownership flow instead), and
 * selecting an assignable role calls `PATCH
 * /rooms/:roomId/members/:userId/role` with the expected body.
 */
describe("RolePicker", () => {
  it("never renders a master option", async () => {
    const user = userEvent.setup()
    render(<RolePicker roomId="room-1" userId="user-3" currentRole="member" />)

    await user.click(screen.getByRole("button", { name: /member/i }))

    expect(screen.getByRole("menuitem", { name: /admin/i })).toBeInTheDocument()
    expect(screen.queryByRole("menuitem", { name: /master|owner/i })).not.toBeInTheDocument()
  })

  it("calls the change-role mutation with the selected role", async () => {
    let capturedBody: unknown
    server.use(
      http.patch(
        "/api/proxy/rooms/:roomId/members/:userId/role",
        async ({ request }) => {
          capturedBody = await request.json()
          return HttpResponse.json({
            id: "member-3",
            room_id: "room-1",
            user_id: "user-3",
            username: "",
            role: "admin",
            joined_at: "2026-01-03T00:00:00Z",
          })
        },
      ),
    )

    const user = userEvent.setup()
    render(<RolePicker roomId="room-1" userId="user-3" currentRole="member" />)

    await user.click(screen.getByRole("button", { name: /member/i }))
    await user.click(screen.getByRole("menuitem", { name: /admin/i }))

    expect(capturedBody).toEqual({ role: "admin" })
  })
})
