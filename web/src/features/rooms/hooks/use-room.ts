"use client"

import { useQuery } from "@tanstack/react-query"
import { getRoomQueryOptions } from "../api/get-room"

/** A single room, sourced from `GET /api/proxy/rooms/:roomId`. */
export function useRoom(roomId: string) {
  return useQuery(getRoomQueryOptions(roomId))
}
