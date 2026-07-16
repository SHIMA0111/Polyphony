"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import {
  batchInviteByGroup,
  type BatchInviteByGroupInput,
} from "../api/batch-invite-by-group"

/**
 * Batch-invites every member of a group into `roomId` in one call, and
 * invalidates the room's own invitations list (the Step 37 query key,
 * `["rooms", roomId, "invitations"]`) on success so any freshly-invited
 * members appear in the room's pending-invitations view without a manual
 * refetch.
 */
export function useBatchInviteByGroup(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: BatchInviteByGroupInput) =>
      batchInviteByGroup(roomId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rooms", roomId, "invitations"] })
    },
  })
}
