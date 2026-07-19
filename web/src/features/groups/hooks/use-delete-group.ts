"use client"

import { useRouter } from "next/navigation"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { deleteGroup } from "../api/delete-group"

/**
 * Deletes a group and, on success, invalidates the `["groups"]` list (the
 * deleted group no longer belongs to the caller) and navigates back to
 * `/groups`, since the detail view the caller was just on is no longer
 * accessible.
 *
 * Uses `router.replace`, not `push`: a `push` would leave the now-destroyed
 * `/groups/:groupId` detail route in browser history, so tapping Back from
 * `/groups` would return to a page for a group that no longer exists.
 */
export function useDeleteGroup() {
  const queryClient = useQueryClient()
  const router = useRouter()

  return useMutation({
    mutationFn: (groupId: string) => deleteGroup(groupId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["groups"] })
      router.replace("/groups")
    },
  })
}
