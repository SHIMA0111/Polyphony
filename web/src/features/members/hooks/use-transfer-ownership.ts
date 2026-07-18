"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { transferOwnership } from "../api/transfer-ownership"

/**
 * Transfers room ownership to another existing member. Invalidates
 * `["rooms", roomId, "members"]` (both the old and new owner's role rows
 * change), `["rooms", roomId]` (the caller's own `Room.role` flips from
 * `master` to `admin`), and `["rooms"]` (the room list's per-room role also
 * reflects this room).
 */
export function useTransferOwnership(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (newOwnerId: string) => transferOwnership(roomId, newOwnerId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rooms", roomId, "members"] })
      queryClient.invalidateQueries({ queryKey: ["rooms", roomId] })
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
    },
  })
}
