"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { updateRoom, type UpdateRoomInput } from "../api/update-room"

/**
 * Renames/re-describes a room. Reconciles the `["rooms", roomId]` cache
 * directly with the server's response (so `ChatRoom`/`RoomSettingsDrawer`
 * reflect the new name without a full page reload) and invalidates the
 * `["rooms"]` list query, whose room-rail entries also show the name.
 */
export function useUpdateRoom(roomId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: UpdateRoomInput) => updateRoom(roomId, input),
    onSuccess: (data) => {
      queryClient.setQueryData(["rooms", roomId], data)
      queryClient.invalidateQueries({ queryKey: ["rooms"] })
    },
  })
}
