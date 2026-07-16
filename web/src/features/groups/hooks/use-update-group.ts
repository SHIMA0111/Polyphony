"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { updateGroup, type UpdateGroupInput } from "../api/update-group"

/**
 * Renames/re-describes a group and invalidates both the `["groups"]` list
 * and the `["groups", groupId]` detail query on success.
 */
export function useUpdateGroup(groupId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: UpdateGroupInput) => updateGroup(groupId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["groups"] })
      queryClient.invalidateQueries({ queryKey: ["groups", groupId] })
    },
  })
}
