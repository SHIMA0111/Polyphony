import { http, HttpResponse } from "msw"
import type { Invitation } from "@/features/members/types"
import type {
  BatchInviteByGroupResult,
  Group,
  GroupListResponse,
  GroupMember,
  GroupMemberListResponse,
} from "../types"

/**
 * MSW request handlers for the groups feature (group CRUD, group
 * membership, batch-invite-by-group), shared by the Node `setupServer`
 * (Vitest, see `src/test/msw/server.ts`) and the browser `setupWorker`
 * (`src/test/msw/browser.ts`).
 *
 * Every path is pinned to the `/api/proxy/*` contract that Step 4's BFF
 * catch-all proxy (`app/api/proxy/[...path]/route.ts`) forwards 1:1 to the
 * Go API's `/groups*` routes (`server/internal/app/routes_group.go`).
 */

export const fixtureGroups: Group[] = [
  {
    id: "group-1",
    owner_id: "user-1",
    name: "Team A",
    description: "A test group",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
]

export const fixtureGroupMembers: GroupMember[] = [
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

export const fixtureBatchInviteInvitation: Invitation = {
  id: "invitation-batch-1",
  room_id: "room-1",
  inviter_id: "user-1",
  invitee_id: "user-3",
  invite_code: "batch-invite-code",
  role: "member",
  status: "pending",
  expires_at: "2026-02-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
}

export const groupsHandlers = [
  http.get("/api/proxy/groups", () => {
    return HttpResponse.json<GroupListResponse>({ groups: fixtureGroups })
  }),

  http.post("/api/proxy/groups", async ({ request }) => {
    const body = (await request.json()) as { name: string; description: string }
    return HttpResponse.json<Group>(
      {
        id: "group-new",
        owner_id: "user-1",
        name: body.name,
        description: body.description,
        created_at: "2026-01-05T00:00:00Z",
        updated_at: "2026-01-05T00:00:00Z",
      },
      { status: 201 },
    )
  }),

  http.get("/api/proxy/groups/:groupId", ({ params }) => {
    const group = fixtureGroups.find((g) => g.id === params.groupId) ?? fixtureGroups[0]
    return HttpResponse.json<Group>(group)
  }),

  http.put("/api/proxy/groups/:groupId", async ({ request, params }) => {
    const body = (await request.json()) as { name: string; description: string }
    return HttpResponse.json<Group>({
      id: String(params.groupId),
      owner_id: "user-1",
      name: body.name,
      description: body.description,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-06T00:00:00Z",
    })
  }),

  http.delete("/api/proxy/groups/:groupId", () => {
    return new HttpResponse(null, { status: 204 })
  }),

  http.get("/api/proxy/groups/:groupId/members", () => {
    return HttpResponse.json<GroupMemberListResponse>({
      members: fixtureGroupMembers,
    })
  }),

  http.post("/api/proxy/groups/:groupId/members", async ({ request, params }) => {
    const body = (await request.json()) as { username: string }
    return HttpResponse.json<GroupMember>(
      {
        id: "group-member-new",
        group_id: String(params.groupId),
        user_id: "user-5",
        username: body.username,
        added_at: "2026-01-07T00:00:00Z",
      },
      { status: 201 },
    )
  }),

  http.delete("/api/proxy/groups/:groupId/members/:userId", () => {
    return new HttpResponse(null, { status: 204 })
  }),

  http.post("/api/proxy/rooms/:roomId/invitations/batch-by-group", () => {
    return HttpResponse.json<BatchInviteByGroupResult>({
      invited: [fixtureBatchInviteInvitation],
      skipped: [],
    })
  }),
]
