"use client"

import { useQuery } from "@tanstack/react-query"
import { getMembersQueryOptions } from "../api/get-members"

/**
 * Members of a room, sourced from `GET /api/proxy/rooms/:roomId/members`.
 *
 * Overrides the app-wide `staleTime: 60_000` (see `@/lib/query-client`) with
 * `staleTime: 0` + `refetchOnMount: "always"`: the member list changes
 * through *other* users' actions (an invitee accepting, another admin
 * changing a role) with no WebSocket event to push those changes into this
 * cache, so `MemberPanel` must refetch every time it opens — otherwise a
 * just-joined member stays invisible for up to a minute.
 */
export function useMembers(roomId: string) {
  return useQuery({
    ...getMembersQueryOptions(roomId),
    staleTime: 0,
    refetchOnMount: "always",
  })
}
