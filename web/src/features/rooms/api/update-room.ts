import { apiRequest } from "@/lib/http-client"
import type { Room } from "../types"

export interface UpdateRoomInput {
  name: string
  description: string
}

/** Calls `PUT /api/proxy/rooms/:roomId`. */
export function updateRoom(roomId: string, input: UpdateRoomInput): Promise<Room> {
  return apiRequest<Room>(`/rooms/${roomId}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}
