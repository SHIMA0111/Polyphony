import { apiRequest } from "@/lib/http-client"
import type { Room } from "../types"

export interface CreateRoomInput {
  name: string
  description: string
}

/** Calls `POST /api/proxy/rooms`. */
export function createRoom(input: CreateRoomInput): Promise<Room> {
  return apiRequest<Room>("/rooms", {
    method: "POST",
    body: JSON.stringify(input),
  })
}
