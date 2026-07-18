"use client"

import { useQuery } from "@tanstack/react-query"
import { getMembersQueryOptions } from "../api/get-members"

/** Members of a room, sourced from `GET /api/proxy/rooms/:roomId/members`. */
export function useMembers(roomId: string) {
  return useQuery(getMembersQueryOptions(roomId))
}
