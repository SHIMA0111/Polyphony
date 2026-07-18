"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { changeMemberRole } from "../api/change-member-role"
import type { RoomRole } from "../types"

interface ChangeMemberRoleInput {
  userId: string
  role: RoomRole
}

/**
 * Changes a member's role (never `master` — the server rejects that; use
 * `useTransferOwnership` instead) and invalidates the room's member-list
 * query on success. The response itself is not written into the cache
 * directly (see `changeMemberRole`'s docstring: it carries no `username`),
 * relying on the invalidated refetch to bring back the full, JOINed row.
 */
export function useChangeMemberRole(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ userId, role }: ChangeMemberRoleInput) =>
      changeMemberRole(roomId, userId, role),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rooms", roomId, "members"] })
    },
  })
}
