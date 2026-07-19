"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { addGroupMember } from "../api/add-group-member"

/**
 * Adds an existing user (by exact username) to a group and invalidates the
 * group's own member-list query on success.
 */
export function useAddGroupMember(groupId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (username: string) => addGroupMember(groupId, username),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["groups", groupId, "members"] })
    },
  })
}
