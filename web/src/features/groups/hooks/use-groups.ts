"use client"

import { useQuery } from "@tanstack/react-query"
import { getGroupsQueryOptions } from "../api/get-groups"

/** The caller's own personal groups, sourced from `GET /api/proxy/groups`. */
export function useGroups() {
  return useQuery(getGroupsQueryOptions())
}
