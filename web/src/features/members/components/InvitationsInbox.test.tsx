import { http, HttpResponse } from "msw"
import { describe, expect, it, vi } from "vitest"
import userEvent from "@testing-library/user-event"
import { render, screen, waitFor } from "@/test/render"
import { server } from "@/test/msw/server"
import type { Invitation, InvitationListResponse } from "../types"
import { InvitationsInbox } from "./InvitationsInbox"

const { useRouterMock } = vi.hoisted(() => ({
  useRouterMock: vi.fn(() => ({ push: vi.fn() })),
}))

vi.mock("next/navigation", () => ({
  useRouter: useRouterMock,
  // `Provider` (via `src/test/render.tsx`) wraps every test in
  // `EmotionRegistry`, which calls this Next.js hook to flush Emotion's SSR
  // styles; jsdom never streams, so a no-op is all component tests need
  // (see `(main)/layout.test.tsx`'s identical mock).
  useServerInsertedHTML: vi.fn(),
}))

const pendingInvitation: Invitation = {
  id: "invitation-mine",
  room_id: "room-9",
  inviter_id: "user-1",
  invitee_id: "user-2",
  invite_code: "mine-code",
  role: "member",
  status: "pending",
  expires_at: "2026-03-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
}

/**
 * Covers Step 37's Scope requirement: `InvitationsInbox` renders the
 * caller's own pending invitations (mocked `GET /api/proxy/invitations`)
 * and calls accept/reject on click.
 */
describe("InvitationsInbox", () => {
  it("renders a pending invitation with its offered role", async () => {
    server.use(
      http.get("/api/proxy/invitations", () => {
        return HttpResponse.json<InvitationListResponse>({
          invitations: [pendingInvitation],
        })
      }),
    )

    render(<InvitationsInbox />)

    expect(await screen.findByText(/room-9/)).toBeInTheDocument()
    expect(screen.getByText("Member")).toBeInTheDocument()
  })

  it("shows an empty state when there are no pending invitations", async () => {
    server.use(
      http.get("/api/proxy/invitations", () => {
        return HttpResponse.json<InvitationListResponse>({ invitations: [] })
      }),
    )

    render(<InvitationsInbox />)

    expect(await screen.findByText("No pending invitations")).toBeInTheDocument()
  })

  it("calls accept on click and navigates into the room", async () => {
    server.use(
      http.get("/api/proxy/invitations", () => {
        return HttpResponse.json<InvitationListResponse>({
          invitations: [pendingInvitation],
        })
      }),
    )
    let acceptCalled = false
    server.use(
      http.post("/api/proxy/invitations/:invitationId/accept", () => {
        acceptCalled = true
        return HttpResponse.json({
          id: "member-x",
          room_id: "room-9",
          user_id: "user-2",
          role: "member",
          joined_at: "2026-01-05T00:00:00Z",
        })
      }),
    )

    const push = vi.fn()
    useRouterMock.mockReturnValue({ push })

    const user = userEvent.setup()
    render(<InvitationsInbox />)

    await user.click(await screen.findByRole("button", { name: "Accept" }))

    await waitFor(() => expect(acceptCalled).toBe(true))
    await waitFor(() => expect(push).toHaveBeenCalledWith("/rooms/room-9"))
  })

  it("calls reject on click for a username-targeted invitation", async () => {
    server.use(
      http.get("/api/proxy/invitations", () => {
        return HttpResponse.json<InvitationListResponse>({
          invitations: [pendingInvitation],
        })
      }),
    )
    let rejectCalled = false
    server.use(
      http.post("/api/proxy/invitations/:invitationId/reject", () => {
        rejectCalled = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    const user = userEvent.setup()
    render(<InvitationsInbox />)

    await user.click(await screen.findByRole("button", { name: "Reject" }))

    await waitFor(() => expect(rejectCalled).toBe(true))
  })
})
