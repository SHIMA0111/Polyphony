"use client"

import { useQuery } from "@tanstack/react-query"
import { getGroupMembersQueryOptions } from "../api/get-group-members"

/**
 * Members of a group (with resolved usernames), sourced from
 * `GET /api/proxy/groups/:groupId/members`.
 */
export function useGroupMembers(groupId: string) {
  return useQuery(getGroupMembersQueryOptions(groupId))
}
