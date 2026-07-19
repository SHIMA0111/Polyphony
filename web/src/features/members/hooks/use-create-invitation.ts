"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { createInvitation, type CreateInvitationInput } from "../api/create-invitation"

/**
 * Creates a room invitation (by exact username, or a reusable link when
 * `invitee_username` is omitted) and invalidates the room's own invitations
 * list on success.
 */
export function useCreateInvitation(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: CreateInvitationInput) => createInvitation(roomId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rooms", roomId, "invitations"] })
    },
  })
}
