"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { acceptInvitation } from "../api/accept-invitation"

/**
 * Accepts a pending invitation. On success, invalidates:
 * - `["invitations", "mine"]` (the accepted invitation is no longer pending),
 * - `["rooms"]` (the caller now belongs to one more room),
 * - `["rooms", roomId, "members"]` (the accepting user is a new row in that
 *   room's member list) — `roomId` is read off the mutation's own result
 *   (`RoomMembershipResponse.room_id`), not a hook argument, since a single
 *   `InvitationsInbox` accepts invitations across many different rooms.
 */
export function useAcceptInvitation() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: acceptInvitation,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["invitations", "mine"] })
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
      queryClient.invalidateQueries({
        queryKey: ["rooms", data.room_id, "members"],
      })
    },
  })
}
