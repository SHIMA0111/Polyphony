"use client"

import { useQuery } from "@tanstack/react-query"
import { getGroupQueryOptions } from "../api/get-group"

/** A single group, sourced from `GET /api/proxy/groups/:groupId`. */
export function useGroup(groupId: string) {
  return useQuery(getGroupQueryOptions(groupId))
}
