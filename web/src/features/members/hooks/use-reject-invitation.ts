"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { rejectInvitation } from "../api/reject-invitation"

/** Rejects a pending, username-targeted invitation and invalidates the caller's own pending-invitations list. */
export function useRejectInvitation() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: rejectInvitation,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["invitations", "mine"] })
    },
  })
}
