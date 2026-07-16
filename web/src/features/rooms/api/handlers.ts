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
]
