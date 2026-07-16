import { http, HttpResponse } from "msw"
import type { Room } from "../types"

/**
 * MSW request handlers for the rooms feature, shared by the Node
 * `setupServer` (Vitest, see `src/test/msw/server.ts`) and the browser
 * `setupWorker` (`src/test/msw/browser.ts`).
 *
 * Every path is pinned to the `/api/proxy/*` contract that Step 4's BFF
 * catch-all proxy (`app/api/proxy/[...path]/route.ts`) forwards 1:1 to the
 * Go API.
 */

export const fixtureRoom: Room = {
  id: "room-1",
  name: "General",
  description: "General discussion room",
  owner_id: "user-1",
  role: "master",
  ai_context_cutoff_at: null,
  ai_provider: null,
  ai_model: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

export const fixtureRooms: Room[] = [
  fixtureRoom,
  {
    id: "room-2",
    name: "Random",
    description: "Off-topic chatter",
    owner_id: "user-1",
    role: "master",
    ai_context_cutoff_at: null,
    ai_provider: null,
    ai_model: null,
    created_at: "2026-01-02T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
  },
]

export const roomsHandlers = [
  http.get("/api/proxy/rooms", () => {
    return HttpResponse.json<Room[]>(fixtureRooms)
  }),

  http.post("/api/proxy/rooms", () => {
    return HttpResponse.json<Room>(fixtureRoom, { status: 201 })
  }),

  http.get("/api/proxy/rooms/:roomId", ({ params }) => {
    return HttpResponse.json<Room>({
      ...fixtureRoom,
      id: String(params.roomId),
    })
  }),

  http.delete("/api/proxy/rooms/:roomId", () => {
    return new HttpResponse(null, { status: 204 })
  }),

  http.put("/api/proxy/rooms/:roomId", async ({ request, params }) => {
    const body = (await request.json()) as { name: string; description: string }
    return HttpResponse.json<Room>({
      ...fixtureRoom,
      id: String(params.roomId),
      name: body.name,
      description: body.description,
    })
  }),

  // Mirrors `RoomUsecase.UpdateSettings`'s empty-string-clears convention
  // (`server/internal/usecase/room/settings.go`) -- this client never omits
  // either field, so only the "value" and "" branches are exercised here.
  http.patch("/api/proxy/rooms/:roomId/settings", async ({ request, params }) => {
    const body = (await request.json()) as { ai_provider: string; ai_model: string }
    return HttpResponse.json<Room>({
      ...fixtureRoom,
      id: String(params.roomId),
      ai_provider: body.ai_provider === "" ? null : body.ai_provider,
      ai_model: body.ai_model === "" ? null : body.ai_model,
    })
  }),

  http.patch(
    "/api/proxy/rooms/:roomId/ai-context-cutoff",
    async ({ request, params }) => {
      const body = (await request.json()) as { cutoff_at: string | null }
      return HttpResponse.json<Room>({
        ...fixtureRoom,
        id: String(params.roomId),
        ai_context_cutoff_at: body.cutoff_at ?? null,
      })
    },
  ),
]
