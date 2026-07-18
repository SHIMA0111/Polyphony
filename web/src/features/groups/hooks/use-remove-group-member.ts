"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { removeGroupMember } from "../api/remove-group-member"

/**
 * Removes a member from a group and invalidates the group's own
 * member-list query on success.
 */
export function useRemoveGroupMember(groupId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (userId: string) => removeGroupMember(groupId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["groups", groupId, "members"] })
    },
  })
}
