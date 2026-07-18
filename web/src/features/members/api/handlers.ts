import { http, HttpResponse } from "msw"
import type {
  Invitation,
  Member,
  MemberListResponse,
  InvitationListResponse,
  RoomMembershipResponse,
} from "../types"

/**
 * MSW request handlers for the members feature (members, room invitations,
 * my-invitations, invitation-by-code, accept/reject, role-change, leave,
 * transfer-ownership), shared by the Node `setupServer` (Vitest, see
 * `src/test/msw/server.ts`) and the browser `setupWorker`
 * (`src/test/msw/browser.ts`).
 *
 * Every path is pinned to the `/api/proxy/*` contract that Step 4's BFF
 * catch-all proxy (`app/api/proxy/[...path]/route.ts`) forwards 1:1 to the
 * Go API.
 */

export const fixtureMembers: Member[] = [
  {
    id: "member-1",
    room_id: "room-1",
    user_id: "user-1",
    username: "testuser",
    role: "master",
    joined_at: "2026-01-01T00:00:00Z",
  },
  {
    id: "member-2",
    room_id: "room-1",
    user_id: "user-2",
    username: "alice",
    role: "admin",
    joined_at: "2026-01-02T00:00:00Z",
  },
  {
    id: "member-3",
    room_id: "room-1",
    user_id: "user-3",
    username: "bob",
    role: "member",
    joined_at: "2026-01-03T00:00:00Z",
  },
]

export const fixtureInvitation: Invitation = {
  id: "invitation-1",
  room_id: "room-1",
  inviter_id: "user-1",
  invitee_id: "user-4",
  invite_code: "test-invite-code",
  role: "member",
  status: "pending",
  expires_at: "2026-02-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
}

export const membersHandlers = [
  http.get("/api/proxy/rooms/:roomId/members", () => {
    return HttpResponse.json<MemberListResponse>({ members: fixtureMembers })
  }),

  http.delete("/api/proxy/rooms/:roomId/members/:userId", () => {
    return new HttpResponse(null, { status: 204 })
  }),

  http.patch("/api/proxy/rooms/:roomId/members/:userId/role", async ({ request, params }) => {
    const body = (await request.json()) as { role: string }
    return HttpResponse.json<Member>({
      id: "member-updated",
      room_id: String(params.roomId),
      user_id: String(params.userId),
      username: "",
      role: body.role as Member["role"],
      joined_at: "2026-01-03T00:00:00Z",
    })
  }),

  http.patch("/api/proxy/rooms/:roomId/owner", ({ params }) => {
    return HttpResponse.json({
      id: String(params.roomId),
      name: "General",
      description: "General discussion room",
      owner_id: "user-2",
      role: "admin",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    })
  }),

  http.post("/api/proxy/rooms/:roomId/invitations", async ({ request, params }) => {
    const body = (await request.json()) as {
      invitee_username?: string
      role: string
      expires_in_hours?: number
    }
    return HttpResponse.json<Invitation>(
      {
        id: "invitation-new",
        room_id: String(params.roomId),
        inviter_id: "user-1",
        invitee_id: body.invitee_username ? "user-4" : null,
        invite_code: "new-invite-code",
        role: body.role as Invitation["role"],
        status: "pending",
        expires_at: "2026-02-01T00:00:00Z",
        created_at: "2026-01-01T00:00:00Z",
      },
      { status: 201 },
    )
  }),

  http.get("/api/proxy/rooms/:roomId/invitations", () => {
    return HttpResponse.json<InvitationListResponse>({
      invitations: [fixtureInvitation],
    })
  }),

  http.get("/api/proxy/invitations", () => {
    return HttpResponse.json<InvitationListResponse>({ invitations: [] })
  }),

  http.get("/api/proxy/invitations/by-code/:code", ({ params }) => {
    return HttpResponse.json<Invitation>({
      ...fixtureInvitation,
      invite_code: String(params.code),
    })
  }),

  http.post("/api/proxy/invitations/:invitationId/accept", () => {
    return HttpResponse.json<RoomMembershipResponse>({
      id: "member-new",
      room_id: "room-1",
      user_id: "user-4",
      role: "member",
      joined_at: "2026-01-05T00:00:00Z",
    })
  }),

  http.post("/api/proxy/invitations/:invitationId/reject", () => {
    return new HttpResponse(null, { status: 204 })
  }),
]
